package config

import (
	"strings"
	"testing"
)

func TestFromEnv(t *testing.T) {
	t.Setenv("GRAFANA_URL", "http://grafana.grafana.svc/")
	t.Setenv("GRAFANA_NAMESPACE", "")
	t.Setenv("GRAFANA_TOKEN", "token")
	t.Setenv("GITHUB_APP_ID", "5743264")
	t.Setenv("GITHUB_APP_INSTALLATION_ID", "181563593")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "pem")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GrafanaURL != "http://grafana.grafana.svc" || cfg.GrafanaNamespace != "default" || cfg.GitHubInstallationID != 181563593 {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestFromEnvReportsAllMissingValues(t *testing.T) {
	for _, key := range []string{"GRAFANA_URL", "GRAFANA_TOKEN", "GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY"} {
		t.Setenv(key, "")
	}
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, key := range []string{"GRAFANA_URL", "GRAFANA_TOKEN", "GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error should mention %s: %v", key, err)
		}
	}
}
