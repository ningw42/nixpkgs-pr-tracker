package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRequiresTargetBranches(t *testing.T) {
	// NPT_TARGET_BRANCHES not set → error
	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error when NPT_TARGET_BRANCHES is not set")
	}
	if !strings.Contains(err.Error(), "NPT_TARGET_BRANCHES") {
		t.Errorf("error %q should mention NPT_TARGET_BRANCHES", err)
	}
}

func TestLoadTargetBranchesOnlyWhitespace(t *testing.T) {
	t.Setenv("NPT_TARGET_BRANCHES", " , , ")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error for whitespace-only NPT_TARGET_BRANCHES")
	}
	if !strings.Contains(err.Error(), "NPT_TARGET_BRANCHES") {
		t.Errorf("error %q should mention NPT_TARGET_BRANCHES", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("NPT_TARGET_BRANCHES", "nixos-unstable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

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
	if len(cfg.TargetBranches) != 1 || cfg.TargetBranches[0] != "nixos-unstable" {
		t.Errorf("TargetBranches = %v, want [nixos-unstable]", cfg.TargetBranches)
	}
	// NotificationBranches defaults to TargetBranches when not set
	if len(cfg.NotificationBranches) != 1 || cfg.NotificationBranches[0] != "nixos-unstable" {
		t.Errorf("NotificationBranches = %v, want [nixos-unstable]", cfg.NotificationBranches)
	}
}

func TestLoadAllOverrides(t *testing.T) {
	t.Setenv("NPT_LISTEN_ADDR", ":9090")
	t.Setenv("NPT_DB_PATH", "/tmp/test.db")
	t.Setenv("NPT_GITHUB_TOKEN", "ghp_test123")
	t.Setenv("NPT_WEBHOOK_URL", "https://example.com/hook")
	t.Setenv("NPT_POLL_INTERVAL", "30s")
	t.Setenv("NPT_TARGET_BRANCHES", "nixos-unstable")
	t.Setenv("NPT_NOTIFICATION_BRANCHES", "staging,nixos-unstable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

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
	if len(cfg.TargetBranches) != 1 || cfg.TargetBranches[0] != "nixos-unstable" {
		t.Errorf("TargetBranches = %v, want [nixos-unstable]", cfg.TargetBranches)
	}
	if len(cfg.NotificationBranches) != 2 || cfg.NotificationBranches[0] != "staging" || cfg.NotificationBranches[1] != "nixos-unstable" {
		t.Errorf("NotificationBranches = %v, want [staging nixos-unstable]", cfg.NotificationBranches)
	}
}

func TestLoadInvalidPollInterval(t *testing.T) {
	t.Setenv("NPT_TARGET_BRANCHES", "nixos-unstable")
	t.Setenv("NPT_POLL_INTERVAL", "notaduration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("PollInterval = %v, want default %v for invalid input", cfg.PollInterval, 5*time.Minute)
	}
}

func TestLoadTargetBranchSplitting(t *testing.T) {
	t.Setenv("NPT_TARGET_BRANCHES", "a,b,c")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	want := []string{"a", "b", "c"}
	if len(cfg.TargetBranches) != len(want) {
		t.Fatalf("TargetBranches length = %d, want %d", len(cfg.TargetBranches), len(want))
	}
	for i, b := range want {
		if cfg.TargetBranches[i] != b {
			t.Errorf("TargetBranches[%d] = %q, want %q", i, cfg.TargetBranches[i], b)
		}
	}
}

func TestLoadTrimsWhitespaceAndFiltersEmpty(t *testing.T) {
	t.Setenv("NPT_TARGET_BRANCHES", " nixos-unstable , staging ,, ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.TargetBranches) != 2 {
		t.Fatalf("TargetBranches = %v, want 2 entries", cfg.TargetBranches)
	}
	if cfg.TargetBranches[0] != "nixos-unstable" || cfg.TargetBranches[1] != "staging" {
		t.Errorf("TargetBranches = %v, want [nixos-unstable staging]", cfg.TargetBranches)
	}
}

func TestValidateBranchesAllValid(t *testing.T) {
	if err := ValidateBranches([]string{"nixos-unstable", "master", "staging"}); err != nil {
		t.Errorf("ValidateBranches returned error for known branches: %v", err)
	}
}

func TestValidateBranchesUnknown(t *testing.T) {
	err := ValidateBranches([]string{"nixos-unstable", "foobar"})
	if err == nil {
		t.Fatal("ValidateBranches returned nil, want error for unknown branch")
	}
	if !strings.Contains(err.Error(), "foobar") {
		t.Errorf("error %q does not mention unknown branch 'foobar'", err)
	}
}

func TestValidateBranchesEmpty(t *testing.T) {
	if err := ValidateBranches([]string{}); err != nil {
		t.Errorf("ValidateBranches returned error for empty list: %v", err)
	}
}

func TestNotificationBranchesDefaultsToTarget(t *testing.T) {
	t.Setenv("NPT_TARGET_BRANCHES", "nixos-unstable,nixos-24.11")
	// NPT_NOTIFICATION_BRANCHES not set

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.NotificationBranches) != 2 || cfg.NotificationBranches[0] != "nixos-unstable" || cfg.NotificationBranches[1] != "nixos-24.11" {
		t.Errorf("NotificationBranches = %v, want [nixos-unstable nixos-24.11] (should default to target)", cfg.NotificationBranches)
	}
}

func TestNotificationBranchesDefaultIsIndependent(t *testing.T) {
	// When notification branches default to target branches, they should be
	// independent slices (not aliased) to prevent future mutation bugs.
	t.Setenv("NPT_TARGET_BRANCHES", "nixos-unstable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Mutate one and verify the other is unaffected
	cfg.NotificationBranches[0] = "mutated"
	if cfg.TargetBranches[0] == "mutated" {
		t.Error("NotificationBranches and TargetBranches share the same underlying slice")
	}
}

func TestNotificationBranchesExplicit(t *testing.T) {
	t.Setenv("NPT_TARGET_BRANCHES", "nixos-unstable")
	t.Setenv("NPT_NOTIFICATION_BRANCHES", "staging,nixos-unstable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.TargetBranches) != 1 || cfg.TargetBranches[0] != "nixos-unstable" {
		t.Errorf("TargetBranches = %v, want [nixos-unstable]", cfg.TargetBranches)
	}
	if len(cfg.NotificationBranches) != 2 || cfg.NotificationBranches[0] != "staging" || cfg.NotificationBranches[1] != "nixos-unstable" {
		t.Errorf("NotificationBranches = %v, want [staging nixos-unstable]", cfg.NotificationBranches)
	}
}

func TestTargetNotInNotificationBranchesErrors(t *testing.T) {
	// Target branch "nixos-24.11" is not in notification branches → error
	t.Setenv("NPT_TARGET_BRANCHES", "nixos-unstable,nixos-24.11")
	t.Setenv("NPT_NOTIFICATION_BRANCHES", "nixos-unstable")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should error when target branches are not in notification branches")
	}
	if !strings.Contains(err.Error(), "nixos-24.11") {
		t.Errorf("error %q should mention the missing branch 'nixos-24.11'", err)
	}
}

func TestNotificationBranchesWhitespaceOnly(t *testing.T) {
	t.Setenv("NPT_TARGET_BRANCHES", "nixos-unstable")
	t.Setenv("NPT_NOTIFICATION_BRANCHES", " , , ")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should error for whitespace-only NPT_NOTIFICATION_BRANCHES")
	}
}
