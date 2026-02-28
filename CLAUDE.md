# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o nixpkgs-pr-tracker .    # build
go run .                             # run directly
go test ./...                        # run all tests
go test ./internal/db/               # run tests for a single package
```

Dev shell via Nix: `nix develop` (provides go, gopls, gotools).

## Configuration

All config is via environment variables (no flags/files):

| Variable            | Default          | Description                                     |
| ------------------- | ---------------- | ----------------------------------------------- |
| `NPT_LISTEN_ADDR`   | `:8585`          | HTTP server address                             |
| `NPT_DB_PATH`       | `./tracker.db`   | SQLite database path                            |
| `NPT_GITHUB_TOKEN`  | (empty)          | GitHub API token (optional, raises rate limits) |
| `NPT_WEBHOOK_URL`   | (empty)          | Webhook URL for notifications                   |
| `NPT_POLL_INTERVAL` | `5m`             | How often to poll GitHub                        |
| `NPT_BRANCHES`      | `nixos-unstable` | Comma-separated list of branches to track       |

## Architecture

A Go web service that tracks NixOS/nixpkgs pull requests and monitors whether their merge commits have landed in target branches (e.g. `nixos-unstable`).

**Flow:** User adds a PR number via the web UI or API → the app fetches PR info from GitHub → a background poller periodically checks if the PR has been merged and if its merge commit has reached each tracked branch → once landed in all branches, the PR is auto-removed.

### Key packages

- **`main.go`** — Wires everything together: config, DB, GitHub client, event bus, poller, and HTTP server. Embeds HTML templates via `//go:embed`.
- **`internal/config`** — Loads config from env vars with defaults.
- **`internal/db`** — SQLite persistence layer (uses `modernc.org/sqlite`, a pure-Go driver — no CGO). Two tables: `tracked_prs` and `branch_status`. Auto-migrates on startup.
- **`internal/github`** — GitHub API client. Fetches PR info and checks if a commit exists in a branch via the compare API. Hardcoded to `NixOS/nixpkgs` repo.
- **`internal/poller`** — Background goroutine that periodically polls all tracked PRs. Updates status (open→merged→closed), checks branch landing, and auto-removes PRs that have landed everywhere.
- **`internal/event`** — Simple in-process pub/sub event bus. Event types: `pr_added`, `pr_removed`, `pr_merged`, `pr_landed_branch`.
- **`internal/notifier`** — `Notifier` interface + webhook implementation. Subscribes to the event bus and POSTs JSON payloads.
- **`internal/server`** — HTTP handlers. Serves the HTML UI at `/` and a JSON API (`POST /api/prs`, `GET /api/prs`, `DELETE /api/prs/{number}`).
- **`web/templates/`** — Go HTML templates embedded at compile time.

### API endpoints

- `GET /` — HTML dashboard
- `POST /api/prs` — Add a PR to track (body: `{"pr_number": 123}`)
- `GET /api/prs` — List tracked PRs as JSON
- `DELETE /api/prs/{number}` — Remove a tracked PR

## Commit Convention

This repo uses [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/#summary). Format: `<type>[optional scope]: <description>`, e.g. `feat: add webhook support`, `fix(poller): handle nil pointer`. Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `build`, `ci`, `style`, `perf` and `test`.
