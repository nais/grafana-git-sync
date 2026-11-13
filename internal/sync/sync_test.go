package sync

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/nais/grafana-git-sync/internal/github"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	namespace      = "default"
	appID          = int64(5743264)
	installationID = int64(181563593)
)

var privateKey = []byte("-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----\n")

func newSyncer(t *testing.T, objects ...runtime.Object) (*Syncer, *fake.FakeDynamicClient) {
	t.Helper()
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		ConnectionsGVR:  "ConnectionList",
		RepositoriesGVR: "RepositoryList",
	}, objects...)
	return &Syncer{
		Client:         client,
		Namespace:      namespace,
		AppID:          appID,
		InstallationID: installationID,
		PrivateKey:     privateKey,
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, client
}

func cliRepo() github.Repository {
	return github.Repository{Owner: "nais", Name: "cli", FullName: "nais/cli", URL: "https://github.com/nais/cli", DefaultBranch: "main"}
}

func get(t *testing.T, s *Syncer, gvr schema.GroupVersionResource, name string) *unstructured.Unstructured {
	t.Helper()
	obj, err := s.Client.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get %s %s: %v", gvr.Resource, name, err)
	}
	return obj
}

func writes(client *fake.FakeDynamicClient) []k8stesting.Action {
	var out []k8stesting.Action
	for _, a := range client.Actions() {
		switch a.GetVerb() {
		case "create", "update", "delete":
			out = append(out, a)
		}
	}
	return out
}

func TestRunCreatesReadOnlyRepositoryAndConnection(t *testing.T) {
	s, _ := newSyncer(t)
	if err := s.Run(context.Background(), []github.Repository{cliRepo()}); err != nil {
		t.Fatal(err)
	}

	conn := get(t, s, ConnectionsGVR, "github-181563593")
	if v, _, _ := unstructured.NestedString(conn.Object, "secure", "privateKey", "create"); v == "" {
		t.Error("private key not sent on create")
	}
	if v, _, _ := unstructured.NestedBool(conn.Object, "spec", "webhook", "disabled"); !v {
		t.Error("connection webhooks must be disabled")
	}
	if conn.GetLabels()[ManagedByLabel] != ManagedByValue {
		t.Error("connection is not labelled as managed")
	}

	repo := get(t, s, RepositoriesGVR, "nais-cli")
	workflows, found, _ := unstructured.NestedSlice(repo.Object, "spec", "workflows")
	if !found || len(workflows) != 0 {
		t.Errorf("repository must be read-only, workflows = %v", workflows)
	}
	for path, want := range map[string]string{
		"spec.title":           "nais/cli",
		"spec.github.branch":   "main",
		"spec.github.path":     "grafana/",
		"spec.connection.name": "github-181563593",
	} {
		if got, _, _ := unstructured.NestedString(repo.Object, strings.Split(path, ".")...); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	if desc, _, _ := unstructured.NestedString(repo.Object, "spec", "description"); !strings.Contains(desc, SourceURL) {
		t.Errorf("description must link to the job: %q", desc)
	}
	if v, _, _ := unstructured.NestedBool(repo.Object, "spec", "webhook", "disabled"); !v {
		t.Error("repository webhooks must be disabled to match the connection")
	}
}

func TestRunIsIdempotentAndDoesNotResendKey(t *testing.T) {
	s, client := newSyncer(t)
	ctx := context.Background()
	if err := s.Run(ctx, []github.Repository{cliRepo()}); err != nil {
		t.Fatal(err)
	}
	client.ClearActions()
	if err := s.Run(ctx, []github.Repository{cliRepo()}); err != nil {
		t.Fatal(err)
	}
	if w := writes(client); len(w) != 0 {
		t.Fatalf("expected no writes on unchanged second run, got %v", w)
	}
}

func TestRunRotatesKeyOnlyWhenChanged(t *testing.T) {
	s, client := newSyncer(t)
	ctx := context.Background()
	if err := s.Run(ctx, nil); err != nil {
		t.Fatal(err)
	}
	// Simulate Grafana replacing the inline secret with a reference.
	conn := get(t, s, ConnectionsGVR, "github-181563593")
	conn.Object["secure"] = map[string]any{"privateKey": map[string]any{"name": "inline-abc"}}
	if _, err := s.connections().Update(ctx, conn, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	s.PrivateKey = []byte("rotated")
	client.ClearActions()
	if err := s.Run(ctx, nil); err != nil {
		t.Fatal(err)
	}
	conn = get(t, s, ConnectionsGVR, "github-181563593")
	if v, _, _ := unstructured.NestedString(conn.Object, "secure", "privateKey", "create"); v == "" {
		t.Error("rotated key was not sent")
	}
	if conn.GetAnnotations()[PrivateKeyAnnotation] != PrivateKeyHash([]byte("rotated")) {
		t.Error("key hash annotation not updated")
	}
	if len(writes(client)) != 1 {
		t.Errorf("expected one update, got %v", writes(client))
	}
}

func TestRunOverwritesManualChanges(t *testing.T) {
	s, _ := newSyncer(t)
	ctx := context.Background()
	if err := s.Run(ctx, []github.Repository{cliRepo()}); err != nil {
		t.Fatal(err)
	}
	repo := get(t, s, RepositoriesGVR, "nais-cli")
	_ = unstructured.SetNestedSlice(repo.Object, []any{"write"}, "spec", "workflows")
	repo.SetLabels(map[string]string{ManagedByLabel: ManagedByValue, InstallationLabel: "181563593", "team": "kept"})
	if _, err := s.repositories().Update(ctx, repo, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := s.Run(ctx, []github.Repository{cliRepo()}); err != nil {
		t.Fatal(err)
	}
	repo = get(t, s, RepositoriesGVR, "nais-cli")
	if workflows, _, _ := unstructured.NestedSlice(repo.Object, "spec", "workflows"); len(workflows) != 0 {
		t.Errorf("manual write workflow not reverted: %v", workflows)
	}
	if repo.GetLabels()["team"] != "kept" {
		t.Error("unrelated labels should be preserved")
	}
}

func TestRunRefusesUnmanagedObjects(t *testing.T) {
	manual := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion, "kind": "Repository",
		"metadata": map[string]any{"name": "nais-cli", "namespace": namespace},
		"spec":     map[string]any{"title": "hand-made"},
	}}
	s, _ := newSyncer(t, manual)
	other := github.Repository{Owner: "nais", Name: "api", FullName: "nais/api", URL: "https://github.com/nais/api", DefaultBranch: "main"}

	err := s.Run(context.Background(), []github.Repository{cliRepo(), other})
	if err == nil || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("expected refusal error, got %v", err)
	}
	if title, _, _ := unstructured.NestedString(get(t, s, RepositoriesGVR, "nais-cli").Object, "spec", "title"); title != "hand-made" {
		t.Error("unmanaged repository was modified")
	}
	get(t, s, RepositoriesGVR, "nais-api") // Other repositories still sync.
}

func TestRunPrunesOnlyOwnRepositories(t *testing.T) {
	repo := func(name string, labels map[string]any) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": apiVersion, "kind": "Repository",
			"metadata": map[string]any{"name": name, "namespace": namespace, "labels": labels},
		}}
	}
	s, _ := newSyncer(t,
		repo("nais-gone", map[string]any{ManagedByLabel: ManagedByValue, InstallationLabel: "181563593"}),
		repo("other-installation", map[string]any{ManagedByLabel: ManagedByValue, InstallationLabel: "1"}),
		repo("hand-made", nil),
	)
	if err := s.Run(context.Background(), []github.Repository{cliRepo()}); err != nil {
		t.Fatal(err)
	}

	list, err := s.repositories().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	got := strings.Join(names, ",")
	if strings.Contains(got, "nais-gone") {
		t.Errorf("stale managed repository not pruned: %s", got)
	}
	for _, keep := range []string{"nais-cli", "other-installation", "hand-made"} {
		if !strings.Contains(got, keep) {
			t.Errorf("%s should be kept: %s", keep, got)
		}
	}
}

func TestRepositoryName(t *testing.T) {
	for _, tc := range []struct{ owner, repo, want string }{
		{"nais", "cli", "nais-cli"},
		{"NAIS", "cli", "nais-cli"}, // GitHub names are case-insensitive.
		{"nais", "my_repo.js", "nais-my-repo-js-"},
		{"navikt", strings.Repeat("a", 60), "navikt-aaaaaaaaaaaaaaaaaaaaaaaa-"},
	} {
		got := RepositoryName(tc.owner, tc.repo)
		if !strings.HasPrefix(got, tc.want) {
			t.Errorf("RepositoryName(%q, %q) = %q, want prefix %q", tc.owner, tc.repo, got, tc.want)
		}
		if len(got) > maxNameLength {
			t.Errorf("RepositoryName(%q, %q) = %q exceeds %d characters", tc.owner, tc.repo, got, maxNameLength)
		}
	}
	if RepositoryName("nais", "my_repo") == RepositoryName("nais", "my-repo") {
		t.Error("rewritten names must not collide with literal names")
	}
}
