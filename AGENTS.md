# Repository Guidelines

## Project Structure & Module Organization
`main.go` is the CLI entrypoint. Core code lives under `internal/`: `app/` wires the runtime, `config/` loads `settings.yml`, `state/` holds shared state, `ui/` renders the fullscreen dashboard, `collector/` contains network probes, and `alert/`, `storage/`, and `logger/` handle alerting, SQLite persistence, and logging. Tests sit beside the code as `*_test.go`. Runtime artifacts such as `netpulse.db` and `netpulse.log` are generated in the project root.

## Build, Test, and Development Commands
Use standard Go commands from the repository root:

- `go build -o netpulse .` builds the local binary.
- `go run .` starts the app with the default config.
- `sudo go run . settings.yml` runs with ICMP privileges and an explicit config.
- `go test ./...` runs the full test suite.
- `go test ./internal/state ./internal/config ./internal/alert ./internal/collector/speedtest` runs the packages that currently have passing tests in this checkout.
- `gofmt -w main.go internal/**/*.go` formats changed Go files before review.

## Coding Style & Naming Conventions
Follow idiomatic Go: tabs for indentation, mixedCaps for exported identifiers, lowercase package names, and short receiver names. Keep packages focused by feature, matching the existing `internal/<domain>` layout. Prefer table-driven tests for validation logic and keep config keys snake_case in YAML, for example `refresh_interval` and `packet_count`.

## Testing Guidelines
Write tests in the same package as the code under test and name them `TestXxx`. Favor deterministic unit tests with `httptest`, `t.TempDir()`, and small fixtures over live network dependencies. Cover new collector or state behavior with direct assertions on `AppState` snapshots. As of May 21, 2026, `go test ./...` does not pass because `internal/ui/dashboard.go` has compile errors; fix that before claiming a fully green run.

## Commit & Pull Request Guidelines
Git history is not available in this workspace, so follow the prevailing Go convention of short, imperative commit subjects with a clear scope, for example `ui: fix alert panel rendering`. Keep PRs narrow, describe behavior changes, list config or privilege impacts, and include a terminal screenshot when UI output changes to panels such as summary cards, connectivity checks, graphs, history, speed test, or alerts. Link the relevant issue when one exists and note the exact test command you ran.

## Security & Configuration Tips
Do not commit real network logs, generated databases, or environment-specific `settings.yml` secrets. This free Community build is local-machine-only; avoid adding remote-monitoring assumptions to docs or code paths without explicit product changes. The app may require `sudo` for ICMP; review privilege assumptions carefully and prefer safe defaults in config changes.
