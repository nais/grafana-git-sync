package github

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	gogithub "github.com/google/go-github/v93/github"
)

// Repository is the subset of a GitHub repository the sync needs.
type Repository struct {
	Owner         string
	Name          string
	FullName      string
	URL           string
	DefaultBranch string
}

// NewInstallationClient authenticates as the GitHub App installation.
func NewInstallationClient(appID, installationID int64, privateKey []byte) (*gogithub.Client, error) {
	transport, err := ghinstallation.New(http.DefaultTransport, appID, installationID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("create GitHub App installation transport: %w", err)
	}
	return gogithub.NewClient(
		gogithub.WithTransport(transport),
		gogithub.WithTimeout(30*time.Second),
		gogithub.WithUserAgent("grafana-git-sync"),
	)
}

// ListRepositories returns every non-archived repository the installation can
// access. It fails rather than returning a partial list, because callers prune
// based on the result.
func ListRepositories(ctx context.Context, client *gogithub.Client) ([]Repository, error) {
	var repos []Repository
	opts := &gogithub.ListOptions{PerPage: 100}
	for {
		page, resp, err := client.Apps.ListRepos(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list installation repositories: %w", err)
		}
		for _, r := range page.Repositories {
			if r.GetArchived() {
				continue
			}
			repos = append(repos, Repository{
				Owner:         r.GetOwner().GetLogin(),
				Name:          r.GetName(),
				FullName:      r.GetFullName(),
				URL:           r.GetHTMLURL(),
				DefaultBranch: r.GetDefaultBranch(),
			})
		}
		if resp.NextPage == 0 {
			return repos, nil
		}
		opts.Page = resp.NextPage
	}
}
