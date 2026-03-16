package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ningw42/nixpkgs-pr-tracker/internal/topology"
)

type Config struct {
	ListenAddr           string
	DBPath               string
	GitHubToken          string
	WebhookURL           string
	PollInterval         time.Duration
	TargetBranches       []string
	NotificationBranches []string
}

// parseBranches splits a comma-separated string into branch names,
// trimming whitespace and filtering out empty entries.
func parseBranches(s string) []string {
	parts := strings.Split(s, ",")
	branches := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			branches = append(branches, p)
		}
	}
	return branches
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:   ":8585",
		DBPath:       "./tracker.db",
		PollInterval: 5 * time.Minute,
	}

	if v := os.Getenv("NPT_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("NPT_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("NPT_GITHUB_TOKEN"); v != "" {
		cfg.GitHubToken = v
	}
	if v := os.Getenv("NPT_WEBHOOK_URL"); v != "" {
		cfg.WebhookURL = v
	}
	if v := os.Getenv("NPT_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.PollInterval = d
		}
	}

	if v := os.Getenv("NPT_TARGET_BRANCHES"); v != "" {
		cfg.TargetBranches = parseBranches(v)
	}
	if len(cfg.TargetBranches) == 0 {
		return cfg, fmt.Errorf("NPT_TARGET_BRANCHES is required (set to a comma-separated list of branch names)")
	}

	if v := os.Getenv("NPT_NOTIFICATION_BRANCHES"); v != "" {
		cfg.NotificationBranches = parseBranches(v)
		if len(cfg.NotificationBranches) == 0 {
			return cfg, fmt.Errorf("NPT_NOTIFICATION_BRANCHES is set but contains no valid branch names")
		}
	} else {
		// Default to target branches (copy to avoid shared-slice aliasing)
		cfg.NotificationBranches = make([]string, len(cfg.TargetBranches))
		copy(cfg.NotificationBranches, cfg.TargetBranches)
	}

	// Ensure all target branches are included in notification branches,
	// otherwise target branches would never be checked and auto-removal
	// could never trigger.
	notifSet := make(map[string]bool, len(cfg.NotificationBranches))
	for _, b := range cfg.NotificationBranches {
		notifSet[b] = true
	}
	var missing []string
	for _, b := range cfg.TargetBranches {
		if !notifSet[b] {
			missing = append(missing, b)
		}
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("target branches %v are not in NPT_NOTIFICATION_BRANCHES; they would never be checked", missing)
	}

	return cfg, nil
}

// ValidateBranches checks that all branches are in topology.KnownBranches.
func ValidateBranches(branches []string) error {
	known := make(map[string]bool, len(topology.KnownBranches))
	for _, b := range topology.KnownBranches {
		known[b] = true
	}
	var unknown []string
	for _, b := range branches {
		if !known[b] {
			unknown = append(unknown, b)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown branches: %v", unknown)
	}
	return nil
}
