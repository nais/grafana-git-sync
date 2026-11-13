package sync

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/nais/grafana-git-sync/internal/github"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	SourceURL = "https://github.com/nais/grafana-git-sync"

	ManagedByLabel        = "app.kubernetes.io/managed-by"
	ManagedByValue        = "grafana-git-sync"
	InstallationLabel     = "grafana-git-sync.nais.io/installation-id"
	PrivateKeyAnnotation  = "grafana-git-sync.nais.io/private-key-sha256"
	apiVersion            = "provisioning.grafana.app/v0alpha1"
	syncPath              = "grafana/"
	syncIntervalSeconds   = int64(60)
	maxNameLength         = 40 // Synced folders are keyed by repository name; Grafana folder UIDs max out at 40.
	nameHashLength        = 8
	connectionDescription = "Managed by grafana-git-sync (" + SourceURL + "). Manual changes are overwritten; change the GitHub App configuration instead."
	repositoryDescription = "Managed by grafana-git-sync (" + SourceURL + "). Manual changes are overwritten; change the source repository instead."
)

var (
	ConnectionsGVR  = schema.GroupVersionResource{Group: "provisioning.grafana.app", Version: "v0alpha1", Resource: "connections"}
	RepositoriesGVR = schema.GroupVersionResource{Group: "provisioning.grafana.app", Version: "v0alpha1", Resource: "repositories"}

	invalidNameChars = regexp.MustCompile(`[^a-z0-9-]+`)
	repeatedDashes   = regexp.MustCompile(`-{2,}`)
)

// ConnectionName is stable per installation so several installations can share one Grafana.
func ConnectionName(installationID int64) string {
	return "github-" + strconv.FormatInt(installationID, 10)
}

// RepositoryName derives a valid, stable Grafana name from owner/repo.
func RepositoryName(owner, repo string) string {
	full := strings.ToLower(owner + "-" + repo)
	name := strings.Trim(repeatedDashes.ReplaceAllString(invalidNameChars.ReplaceAllString(full, "-"), "-"), "-")
	if len(name) <= maxNameLength && name == full {
		return name
	}
	// Truncated or rewritten names get a hash of the original to stay unique.
	sum := sha256.Sum256([]byte(owner + "/" + repo))
	suffix := hex.EncodeToString(sum[:])[:nameHashLength]
	if prefix := maxNameLength - nameHashLength - 1; len(name) > prefix {
		name = strings.TrimRight(name[:prefix], "-")
	}
	return name + "-" + suffix
}

func PrivateKeyHash(privateKey []byte) string {
	sum := sha256.Sum256(privateKey)
	return hex.EncodeToString(sum[:])
}

func managedLabels(installationID int64) map[string]any {
	return map[string]any{
		ManagedByLabel:    ManagedByValue,
		InstallationLabel: strconv.FormatInt(installationID, 10),
	}
}

// desiredConnection omits secure values; the reconciler adds the key only when needed.
func desiredConnection(appID, installationID int64, privateKey []byte) *unstructured.Unstructured {
	id := strconv.FormatInt(installationID, 10)
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       "Connection",
		"metadata": map[string]any{
			"name":        ConnectionName(installationID),
			"labels":      managedLabels(installationID),
			"annotations": map[string]any{PrivateKeyAnnotation: PrivateKeyHash(privateKey)},
		},
		"spec": map[string]any{
			"title":       "GitHub App installation " + id,
			"description": connectionDescription,
			"type":        "github",
			// Grafana rewrites the URL to the installation page; set it the same
			// way so unchanged connections are not updated on every run.
			"url": "https://github.com/settings/installations/" + id,
			"github": map[string]any{
				"appID":          strconv.FormatInt(appID, 10),
				"installationID": id,
			},
			// Grafana is not reachable from GitHub (IAP), so it polls instead.
			"webhook": map[string]any{"disabled": true},
		},
	}}
}

func connectionSecure(privateKey []byte) map[string]any {
	// Grafana expects a base64-encoded PEM.
	return map[string]any{"privateKey": map[string]any{"create": base64.StdEncoding.EncodeToString(privateKey)}}
}

func desiredRepository(repo github.Repository, installationID int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       "Repository",
		"metadata": map[string]any{
			"name":   RepositoryName(repo.Owner, repo.Name),
			"labels": managedLabels(installationID),
		},
		"spec": map[string]any{
			"title":       repo.FullName,
			"description": repositoryDescription,
			"type":        "github",
			"github": map[string]any{
				"url":    strings.TrimSuffix(strings.TrimRight(repo.URL, "/"), ".git"),
				"branch": repo.DefaultBranch,
				"path":   syncPath,
			},
			"connection": map[string]any{"name": ConnectionName(installationID)},
			"sync": map[string]any{
				"enabled":         true,
				"intervalSeconds": syncIntervalSeconds,
				"target":          "folder",
			},
			"webhook": map[string]any{"disabled": true},
			// Empty workflows make the repository read-only in Grafana.
			"workflows": []any{},
		},
	}}
}

func describe(obj *unstructured.Unstructured) string {
	return fmt.Sprintf("%s/%s", obj.GetKind(), obj.GetName())
}
