package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nais/grafana-git-sync/internal/config"
	"github.com/nais/grafana-git-sync/internal/github"
	"github.com/nais/grafana-git-sync/internal/sync"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("sync failed", "error", err)
		os.Exit(1)
	}
	log.Info("sync completed")
}

func run(log *slog.Logger) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	gh, err := github.NewInstallationClient(cfg.GitHubAppID, cfg.GitHubInstallationID, cfg.GitHubPrivateKey)
	if err != nil {
		return err
	}
	// Abort before touching Grafana if listing fails; a partial list would prune repositories.
	repos, err := github.ListRepositories(ctx, gh)
	if err != nil {
		return err
	}
	log.Info("listed GitHub repositories", "count", len(repos))

	// Grafana's /apis is Kubernetes-compatible; talk to it in-cluster, bypassing IAP.
	client, err := dynamic.NewForConfig(&rest.Config{
		Host:        cfg.GrafanaURL,
		BearerToken: cfg.GrafanaToken,
		UserAgent:   "grafana-git-sync",
		Timeout:     30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create Grafana client: %w", err)
	}

	return (&sync.Syncer{
		Client:         client,
		Namespace:      cfg.GrafanaNamespace,
		AppID:          cfg.GitHubAppID,
		InstallationID: cfg.GitHubInstallationID,
		PrivateKey:     cfg.GitHubPrivateKey,
		Log:            log,
	}).Run(ctx, repos)
}
