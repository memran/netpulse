# NetPulse Repository Guide

## Entrypoint & CLI Layer

`main.go` uses `go-consolekit` for sub-commands. Calling `netpulse [arg]` maps to `netpulse run [arg]` unless the arg is a known subcommand (`run`, `config`, `history`, `help`, `completion`) or starts with `-`.

Commands live in `internal/commands/`:
- `run` — boots the full TUI via `internal/app/app.go` → `app.Run()`
- `config:init`, `config:validate`, `history:show` — headless utility commands

## Core Architecture

`internal/app/app.go::Run()` is the sole runtime orchestrator. It:
1. Loads config, creates logger, state manager, alert engine, SQLite repo
2. Sets up the Bubble Tea program with flags: `tea.WithAltScreen()`, `tea.WithFPS(10)`, `tea.WithANSICompressor()`, `tea.WithoutSignalHandler()`
3. Starts background goroutines: state manager event loop, all collectors (goroutine per type), alert evaluator, 60-second state flusher to SQLite
4. Snapshot updates flow: collector → `state.SetXxx()` → `state.Subscribe()` channel → `Manager` → `ui.SnapshotMsg` → `tea.Program.Send()`

Data flows one way: collectors write to `AppState`, the UI reads snapshots. Collectors never touch the UI directly.

## State System

`internal/state/` has two parts:
- `AppState` — thread-safe struct with `Read()` returning `AppStateSnapshot` and `Subscribe()` returning a channel. Uses `sync.RWMutex`.
- `Manager` — wraps `AppState` with an event channel (`chan AppStateSnapshot, buf 1`). `Start(ctx)` runs a goroutine that emits snapshots on each state change.

`AppStateSnapshot` is a plain struct copy (no mutex, safe to read). Fields: `InternetStatus`, `PublicIP`, `Interfaces` (map), `PingStats`, `DNSStats`, `HTTPStats`, `SpeedTest`.

## Config System

`internal/config/config.go::Load()` finds `settings.yml` in argument path, then CWD, `~/.config/netpulse/`, `~/.netpulse/`, `/etc/netpulse`. Writes default YAML to CWD if none found and returns defaults.

Free version caps combined targets at 5 (`enforceFreeLimits()`). Config keys use snake_case in YAML (e.g. `refresh_interval`, `packet_count`).

## UI/Dashboard

`internal/ui/dashboard.go` is a single-file Bubble Tea model (~1300 lines). Uses `lipgloss` for styling with `#0B0F14` background, `#374151` borders, `#22D3EE` header text.

Key rendering pattern: manual ASCII box borders using `┌┐└┘├┤─│` with `bdStyle` foreground. Inner content lines are wrapped with `│` and padded to `innerW`. Sections separated by `├──┤` lines.

Height budget: `panelV = d.height - 7` (outer top + header + 3 separators + outer bottom + footer). Split `topH=38%`, `graphH=28%`, `botH=remaining`.

The `SnapshotMsg` struct is defined in the dashboard package and used by `app.go` to send state updates.

## Alert System

`internal/alert/`:
- `Severity` constants: `SeverityInfo`, `SeverityWarn`, `SeverityCritical`
- `Alert` struct: `ID`, `Timestamp`, `Severity`, `Source`, `Message`
- `Engine` with max alerts (configurable, defaults to 50). `Recent()` returns newest-first. `Clear()` empties.
- `Evaluator` monitors state and triggers alerts via `Engine.Add()`. Thresholds: loss >5%, latency >200ms, jitter >30ms.

## Tests

Four packages have passing tests today:
```
go test ./internal/state ./internal/config ./internal/alert ./internal/collector/speedtest
```

Tests use `t.TempDir()` for config fixtures and `t.TempDir()` for config file tests. The alert tests use `time.Now()` comparisons. The config validation test uses nanosecond durations directly (not parsed strings). The state tests use `t.Logf` for concurrent test output.

`go test ./...` does not pass — `internal/ui/dashboard.go` has compile errors when there's an active UI rewrite in progress.

## Build & Release

CI (`build.yml`) runs `go test ./...` then cross-builds with `CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"` for linux/amd64, windows/amd64, darwin/amd64. Artifacts include `README.md` and `settings.yml`.

## Privilege & Security

ICMP ping requires root/`sudo`. The app is local-machine-only — no remote monitoring, no API, no multi-device, no SNMP or NetFlow. Config supports `enabled: false` under `speedtest:` to disable the speed test collector.

## Key Go Details

- Go 1.25, module `github.com/memran/netpulse`
- `modernc.org/sqlite` (cgo-free, pure Go SQLite). `db.SetMaxOpenConns(1)`.
- `replace go-consolekit => github.com/memran/go-consolekit v0.1.1-...` in `go.mod`
- No `go generate`, no codegen, no protobuf, no migrations beyond `CREATE TABLE IF NOT EXISTS`
