package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	ListenAddr   string
	DBPath       string
	GitHubToken  string
	WebhookURL   string
	PollInterval time.Duration
	Branches     []string
}

func Load() Config {
	cfg := Config{
		ListenAddr:   ":8585",
		DBPath:       "./tracker.db",
		PollInterval: 5 * time.Minute,
		Branches:     []string{"nixos-unstable"},
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
	if v := os.Getenv("NPT_BRANCHES"); v != "" {
		cfg.Branches = strings.Split(v, ",")
	}

	return cfg
}
