package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg := Load()

	if cfg.ListenAddr != ":8585" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8585")
	}
	if cfg.DBPath != "./tracker.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "./tracker.db")
	}
	if cfg.GitHubToken != "" {
		t.Errorf("GitHubToken = %q, want empty", cfg.GitHubToken)
	}
	if cfg.WebhookURL != "" {
		t.Errorf("WebhookURL = %q, want empty", cfg.WebhookURL)
	}
	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 5*time.Minute)
	}
	if len(cfg.Branches) != 1 || cfg.Branches[0] != "nixos-unstable" {
		t.Errorf("Branches = %v, want [nixos-unstable]", cfg.Branches)
	}
}

func TestLoadAllOverrides(t *testing.T) {
	t.Setenv("NPT_LISTEN_ADDR", ":9090")
	t.Setenv("NPT_DB_PATH", "/tmp/test.db")
	t.Setenv("NPT_GITHUB_TOKEN", "ghp_test123")
	t.Setenv("NPT_WEBHOOK_URL", "https://example.com/hook")
	t.Setenv("NPT_POLL_INTERVAL", "30s")
	t.Setenv("NPT_BRANCHES", "nixos-unstable,nixos-24.11")

	cfg := Load()

	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":9090")
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/tmp/test.db")
	}
	if cfg.GitHubToken != "ghp_test123" {
		t.Errorf("GitHubToken = %q, want %q", cfg.GitHubToken, "ghp_test123")
	}
	if cfg.WebhookURL != "https://example.com/hook" {
		t.Errorf("WebhookURL = %q, want %q", cfg.WebhookURL, "https://example.com/hook")
	}
	if cfg.PollInterval != 30*time.Second {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 30*time.Second)
	}
	if len(cfg.Branches) != 2 || cfg.Branches[0] != "nixos-unstable" || cfg.Branches[1] != "nixos-24.11" {
		t.Errorf("Branches = %v, want [nixos-unstable nixos-24.11]", cfg.Branches)
	}
}

func TestLoadInvalidPollInterval(t *testing.T) {
	t.Setenv("NPT_POLL_INTERVAL", "notaduration")

	cfg := Load()

	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("PollInterval = %v, want default %v for invalid input", cfg.PollInterval, 5*time.Minute)
	}
}

func TestLoadBranchSplitting(t *testing.T) {
	t.Setenv("NPT_BRANCHES", "a,b,c")

	cfg := Load()

	want := []string{"a", "b", "c"}
	if len(cfg.Branches) != len(want) {
		t.Fatalf("Branches length = %d, want %d", len(cfg.Branches), len(want))
	}
	for i, b := range want {
		if cfg.Branches[i] != b {
			t.Errorf("Branches[%d] = %q, want %q", i, cfg.Branches[i], b)
		}
	}
}
