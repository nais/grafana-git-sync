package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"

	"github.com/nais/grafana-git-sync/internal/github"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type Syncer struct {
	Client         dynamic.Interface
	Namespace      string
	AppID          int64
	InstallationID int64
	PrivateKey     []byte
	Log            *slog.Logger
}

// Run reconciles the connection and one read-only repository per GitHub repository.
// The caller must pass the complete repository list: anything managed by this
// installation and missing from repos is deleted.
func (s *Syncer) Run(ctx context.Context, repos []github.Repository) error {
	if err := s.syncConnection(ctx); err != nil {
		// Repositories depend on the connection; don't touch them without it.
		return fmt.Errorf("connection: %w", err)
	}

	var errs []error
	wanted := make(map[string]bool, len(repos))
	for _, repo := range repos {
		desired := desiredRepository(repo, s.InstallationID)
		if wanted[desired.GetName()] {
			errs = append(errs, fmt.Errorf("%s: name %q collides with another repository", repo.FullName, desired.GetName()))
			continue
		}
		wanted[desired.GetName()] = true
		if err := s.apply(ctx, s.repositories(), desired, nil); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", repo.FullName, err))
		}
	}

	if err := s.prune(ctx, wanted); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Syncer) connections() dynamic.ResourceInterface {
	return s.Client.Resource(ConnectionsGVR).Namespace(s.Namespace)
}

func (s *Syncer) repositories() dynamic.ResourceInterface {
	return s.Client.Resource(RepositoriesGVR).Namespace(s.Namespace)
}

func (s *Syncer) syncConnection(ctx context.Context) error {
	desired := desiredConnection(s.AppID, s.InstallationID, s.PrivateKey)
	return s.apply(ctx, s.connections(), desired, func(existing *unstructured.Unstructured) map[string]any {
		// Only send the key on create or rotation. Resending it makes Grafana
		// regenerate tokens and rerun health checks on every run.
		if existing == nil || existing.GetAnnotations()[PrivateKeyAnnotation] != PrivateKeyHash(s.PrivateKey) {
			return connectionSecure(s.PrivateKey)
		}
		return nil
	})
}

// apply creates or updates a managed object. It never adopts unlabelled objects,
// so Git Sync setups made by hand are left alone. Creates are never dry runs:
// Grafana only enforces GitHub App write permissions on dry-run requests.
func (s *Syncer) apply(ctx context.Context, client dynamic.ResourceInterface, desired *unstructured.Unstructured, secure func(*unstructured.Unstructured) map[string]any) error {
	log := s.Log.With("resource", describe(desired))

	existing, err := client.Get(ctx, desired.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if secure != nil {
			if v := secure(nil); v != nil {
				desired.Object["secure"] = v
			}
		}
		if _, err := client.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create: %w", err)
		}
		log.Info("created")
		return nil
	}
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	if !s.ownedByUs(existing) {
		return fmt.Errorf("%s exists but is not managed by %s for installation %d; delete it or rename it", describe(desired), ManagedByValue, s.InstallationID)
	}

	var secureValue map[string]any
	if secure != nil {
		secureValue = secure(existing)
	}
	if secureValue == nil && contains(existing.Object, desired.Object) {
		log.Debug("up to date")
		return nil
	}

	updated := existing.DeepCopy()
	updated.Object["spec"] = desired.Object["spec"]
	updated.SetLabels(merge(existing.GetLabels(), desired.GetLabels()))
	updated.SetAnnotations(merge(existing.GetAnnotations(), desired.GetAnnotations()))
	if secureValue != nil {
		updated.Object["secure"] = secureValue
	}
	if _, err := client.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	log.Info("updated", "rotatedSecret", secureValue != nil)
	return nil
}

func (s *Syncer) prune(ctx context.Context, wanted map[string]bool) error {
	list, err := s.repositories().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list repositories: %w", err)
	}
	var errs []error
	for _, item := range list.Items {
		if !s.ownedByUs(&item) || wanted[item.GetName()] {
			continue
		}
		// Grafana's finalizers remove the dashboards this repository provisioned.
		if err := s.repositories().Delete(ctx, item.GetName(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete %s: %w", describe(&item), err))
			continue
		}
		s.Log.Info("deleted", "resource", describe(&item))
	}
	return errors.Join(errs...)
}

func (s *Syncer) ownedByUs(obj *unstructured.Unstructured) bool {
	labels := obj.GetLabels()
	return labels[ManagedByLabel] == ManagedByValue && labels[InstallationLabel] == strconv.FormatInt(s.InstallationID, 10)
}

// contains reports whether every field set in want has the same value in have.
// Fields Grafana adds (defaults, status, secure references) are ignored, but any
// manual change to a field we own is detected and overwritten.
func contains(have, want map[string]any) bool {
	for key, w := range want {
		h, ok := have[key]
		if !ok {
			return false
		}
		if wm, isMap := w.(map[string]any); isMap {
			hm, ok := h.(map[string]any)
			if !ok || !contains(hm, wm) {
				return false
			}
			continue
		}
		if !reflect.DeepEqual(normalize(h), normalize(w)) {
			return false
		}
	}
	return true
}

// normalize makes JSON-decoded numbers comparable with Go integers.
func normalize(v any) any {
	switch n := v.(type) {
	case int:
		return int64(n)
	case float64:
		if n == float64(int64(n)) {
			return int64(n)
		}
	}
	return v
}

func merge(base map[string]string, overlay map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}
