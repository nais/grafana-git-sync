package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is read from the environment. Secrets are expected to come from a
// pre-existing Kubernetes Secret via secretKeyRef.
type Config struct {
	GrafanaURL       string
	GrafanaNamespace string
	GrafanaToken     string

	GitHubAppID          int64
	GitHubInstallationID int64
	GitHubPrivateKey     []byte
}

func FromEnv() (*Config, error) {
	cfg := &Config{
		GrafanaURL:       strings.TrimRight(os.Getenv("GRAFANA_URL"), "/"),
		GrafanaNamespace: envOr("GRAFANA_NAMESPACE", "default"),
		GrafanaToken:     os.Getenv("GRAFANA_TOKEN"),
		GitHubPrivateKey: []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY")),
	}

	var errs []error
	if cfg.GrafanaURL == "" {
		errs = append(errs, errors.New("GRAFANA_URL is required"))
	}
	if cfg.GrafanaToken == "" {
		errs = append(errs, errors.New("GRAFANA_TOKEN is required"))
	}
	if len(cfg.GitHubPrivateKey) == 0 {
		errs = append(errs, errors.New("GITHUB_APP_PRIVATE_KEY is required"))
	}

	var err error
	if cfg.GitHubAppID, err = requiredInt("GITHUB_APP_ID"); err != nil {
		errs = append(errs, err)
	}
	if cfg.GitHubInstallationID, err = requiredInt("GITHUB_APP_INSTALLATION_ID"); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requiredInt(key string) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", key, v)
	}
	return n, nil
}
