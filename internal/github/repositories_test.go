package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	gogithub "github.com/google/go-github/v93/github"
)

func TestListRepositoriesPaginatesAndSkipsArchived(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/installation/repositories" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_, _ = fmt.Fprint(w, `{"total_count":3,"repositories":[{"name":"old","full_name":"nais/old","archived":true,"owner":{"login":"nais"}}]}`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/installation/repositories?page=2&per_page=100>; rel="next"`, server.URL))
		_, _ = fmt.Fprint(w, `{"total_count":3,"repositories":[
			{"name":"cli","full_name":"nais/cli","html_url":"https://github.com/nais/cli","default_branch":"main","owner":{"login":"nais"}},
			{"name":"api","full_name":"nais/api","html_url":"https://github.com/nais/api","default_branch":"master","owner":{"login":"nais"}}]}`)
	}))
	defer server.Close()

	base := server.URL + "/"
	client, err := gogithub.NewClient(gogithub.WithURLs(&base, &base))
	if err != nil {
		t.Fatal(err)
	}
	repos, err := ListRepositories(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 non-archived repositories, got %+v", repos)
	}
	if repos[1].DefaultBranch != "master" || repos[0].Owner != "nais" || repos[0].FullName != "nais/cli" {
		t.Errorf("unexpected repositories: %+v", repos)
	}
}

func TestListRepositoriesFailsInsteadOfReturningPartialList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	base := server.URL + "/"
	client, err := gogithub.NewClient(gogithub.WithURLs(&base, &base))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ListRepositories(context.Background(), client); err == nil {
		t.Fatal("expected error")
	}
}
