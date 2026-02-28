# nixpkgs-pr-tracker

A self-hosted web service that tracks [NixOS/nixpkgs](https://github.com/NixOS/nixpkgs) pull requests and tells you when their merge commits land in target branches like `nixos-unstable`.

## Why

After a nixpkgs PR is merged, it can take days or weeks for the commit to propagate to channel branches. This tool lets you add PRs you care about and get notified when they arrive — so you know exactly when a package update or fix is available in your channel.

## How it works

1. You add a PR number through the web UI or API.
2. The app fetches PR info from GitHub and starts tracking it.
3. A background poller periodically checks whether the PR has been merged and if its merge commit has reached each tracked branch.
4. Once the commit lands in all tracked branches, the PR is automatically removed from tracking.
5. Optionally, webhook notifications are sent at each stage (added, merged, landed, removed).

## Quick start

```bash
# Build
go build -o nixpkgs-pr-tracker .

# Run with defaults (listens on :8585, tracks nixos-unstable)
./nixpkgs-pr-tracker

# Or with Nix
nix develop
go run .
```

Open http://localhost:8585 in your browser to use the dashboard.

## Configuration

All configuration is via environment variables:

| Variable            | Default          | Description                                     |
| ------------------- | ---------------- | ----------------------------------------------- |
| `NPT_LISTEN_ADDR`   | `:8585`          | HTTP listen address                             |
| `NPT_DB_PATH`       | `./tracker.db`   | SQLite database file path                       |
| `NPT_GITHUB_TOKEN`  | _(empty)_        | GitHub API token (optional, raises rate limits) |
| `NPT_WEBHOOK_URL`   | _(empty)_        | Webhook URL for notifications                   |
| `NPT_POLL_INTERVAL` | `5m`             | How often to poll GitHub                        |
| `NPT_BRANCHES`      | `nixos-unstable` | Comma-separated branches to track               |

### Example

```bash
export NPT_GITHUB_TOKEN="ghp_..."
export NPT_BRANCHES="nixos-unstable,nixos-24.11"
export NPT_POLL_INTERVAL="2m"
export NPT_WEBHOOK_URL="https://telepush.example.com/api/inlets/nixpkgs-pr-tracker/your-token"
./nixpkgs-pr-tracker
```

## API

### Add a PR

```bash
curl -XPOST -H 'Content-Type: application/json' \
  -d '{"pr_number": 488091}' \
  http://localhost:8585/api/prs
```

### List tracked PRs

```bash
curl http://localhost:8585/api/prs
```

### Remove a PR

```bash
curl -XDELETE http://localhost:8585/api/prs/488091
```

## Notifications

Set `NPT_WEBHOOK_URL` to receive JSON webhook notifications for these events:

| Event              | Meaning                                                                   |
| ------------------ | ------------------------------------------------------------------------- |
| `pr_added`         | A PR was added to tracking                                                |
| `pr_merged`        | A tracked PR was merged                                                   |
| `pr_landed_branch` | A merge commit landed in a tracked branch                                 |
| `pr_removed`       | A PR was removed (manually or auto-removed after landing in all branches) |

Webhook payload:

```json
{
  "event": "pr_landed_branch",
  "pr_number": 488091,
  "title": "navidrome: 0.60.0 -> 0.60.3",
  "author": "tebriel",
  "branch": "nixos-unstable",
  "timestamp": "2026-02-25T12:00:00Z"
}
```

### Telegram notifications via Telepush

A [Telepush](https://github.com/muety/telepush) custom inlet is included at [`nixpkgs-pr-tracker.yaml`](nixpkgs-pr-tracker.yaml). To use it:

1. Copy `nixpkgs-pr-tracker.yaml` into your Telepush instance's `inlets.d/` directory.
2. Set `NPT_WEBHOOK_URL` to `https://<telepush-host>/api/inlets/nixpkgs-pr-tracker/<recipient-token>`.

## Development

```bash
nix develop              # enter dev shell (provides go, gopls, gotools)
go test ./...            # run all tests
go test -race ./...      # run tests with race detector
```
