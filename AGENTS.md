# Repository Guidelines

## Project Structure & Module Organization

- `main.go` wires dependencies and starts polling plus the HTTP server.
- Core logic is under `internal/`:
  - `internal/github` (GitHub API access), `internal/poller` (sync loop), `internal/db` (SQLite persistence), `internal/server` (HTTP handlers), `internal/notifier` (webhook notifications), `internal/topology` (branch graph/status), `internal/config` (env config), and `internal/event` (event model).
- HTML templates are in `web/templates/`.
- Nix/dev tooling lives in `flake.nix` and `treefmt.nix`.

## Build, Test, and Development Commands

- `nix develop --command go build -o nixpkgs-pr-tracker .` — build the binary.
- `nix develop --command go run .` — run locally with current environment variables.
- `nix develop --command go test ./...` — run all unit tests.
- `nix develop --command go test ./internal/db/` — run tests for one package.
- `nix fmt` — format Nix files via `treefmt`/`nixfmt`.

## Coding Style & Naming Conventions

- Follow standard Go style (`gofmt` formatting, tabs for indentation).
- Keep package names short, lowercase, and descriptive (`poller`, `topology`).
- Exported identifiers use `CamelCase`; unexported helpers use `camelCase`.
- Prefer table-driven tests where logic has multiple scenarios.
- Keep modules focused: API access in `internal/github`, persistence in `internal/db`, transport in `internal/server`.

## Testing Guidelines

- Tests use Go’s built-in `testing` package.
- Place tests next to code in `*_test.go` files.
- Name tests clearly by behavior (for example: `TestValidateBranches`).
- Run `nix develop --command go test ./...` before opening a PR.

## Commit & Pull Request Guidelines

- Use short, imperative commit subjects (for example: `Add notification branch validation`).
- Keep commits focused; avoid mixing refactors with feature changes.
- PRs should include:
  - a clear summary of behavior changes,
  - linked issue(s) if applicable,
  - test evidence (`go test` output or equivalent),
  - screenshots only when UI/template output changes.

## Configuration & Security Tips

- Configure via environment variables (`NPT_*`), not hardcoded secrets.
- Do not commit tokens, webhook URLs, or local DB artifacts.
- Validate branch-related config with existing `internal/config` logic before release.
