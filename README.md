# NetPulse Community

**Free Version** — A polished, terminal-first internet diagnostic dashboard for the machine you are sitting on.

NetPulse Community gives you a fast, full-screen view of internet quality, latency, packet loss, jitter, DNS health, HTTP reachability, interface throughput, alerts, and manual speed tests. It is designed for personal users, technicians, gamers, Linux admins, VPS users, office staff, and ISP support engineers who need an immediate local-machine check.

## Product Scope

NetPulse Community is built to monitor this machine only:

- PC, laptop, desktop, Linux server, VPS, or office workstation
- Local interfaces such as `eth0`, `wlan0`, `en0`, Wi-Fi, or Ethernet

It does not monitor external infrastructure. There is no support for router monitoring, switch monitoring, MikroTik, SNMP, NetFlow, or NOC-style remote polling.

## Features

| Feature | Description |
|---------|-------------|
| Internet Quality | Online/degraded/offline status with latency, loss, jitter |
| Ping Monitor | ICMP ping to configurable targets such as gateway, Cloudflare, Google, or custom hosts |
| DNS Health | A and AAAA lookup checks with response time and failure detection |
| HTTP Reachability | HEAD/GET checks with status codes and TLS info |
| Interface Monitor | Per-interface RX/TX speeds, drops, errors |
| Packet Loss | Percentage-based loss tracking per target |
| Jitter Calculation | Latency variance between consecutive pings |
| Speed Test | Manual download/upload with concurrent workers |
| Latency Graph | ASCII trend graph for recent latency samples |
| Local Alerts | TUI-based rolling alert panel with severity levels |
| SQLite History | Persistent storage of speed tests, alerts, ping summaries, and event history |
| History Summary | Recent uptime, latency, loss, and downtime summary in the dashboard |
| YAML Config | Fully configurable via `settings.yml` |
| Structured Logging | JSON/text logs with component-level tracing |
| Graceful Shutdown | Signal handling (SIGINT/SIGTERM) with context propagation |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Collectors (ping, dns, http, interface, speedtest)          │
│         ↓                                                   │
│ State Manager (thread-safe AppState + event channel)        │
│         ↓                                                   │
│ Renderer (Bubble Tea / Lip Gloss TUI dashboard)             │
├─────────────────────────────────────────────────────────────┤
│ Alert Engine  ──→  SQLite Storage  ──→  Structured Logger   │
└─────────────────────────────────────────────────────────────┘
```

Collectors never directly update the UI. All data flows through the shared `AppState`.

## Quick Start

```bash
# Build from source
git clone https://github.com/memran/netpulse.git
cd netpulse
go build -o netpulse .

# Run (root privileges required for ICMP ping)
sudo ./netpulse
```

Or with a custom config path:

```bash
sudo ./netpulse /path/to/settings.yml
```

## TUI Controls

| Key | Action |
|-----|--------|
| `q` | Quit |
| `r` | Refresh |
| `s` | Run speed test |
| `h` | View history |
| `c` | Clear |
| `?` | Help |
| `↑` / `↓` | Scroll alerts |

## Dashboard Layout

Panels are organized for SSH-friendly, resize-safe terminal use:

- Header
- Top summary cards for status, latency, loss, jitter, DNS, download, and upload
- Connectivity Checks
- Local Network
- Real-Time Graphs
- History Summary
- Speed Test
- Alerts
- Footer Hotkeys

## TUI Layout

```
┌───────────────────────────────────────────────────────────────────────────────┐
│ NetPulse TUI v1.0.0 (FREE)     Internet: ONLINE     Uptime     Clock        │
├───────────────────────────────────────────────────────────────────────────────┤
│ Summary Cards: Status | Avg Latency | Packet Loss | Jitter | DNS | DL | UL  │
├───────────────────────────────┬──────────────────────────────┬───────────────┤
│ Connectivity Checks           │ Real-Time Graphs             │ Speed Test    │
│ Local Interface               │ History Summary              │ Alerts        │
├───────────────────────────────────────────────────────────────────────────────┤
│ [Q] Quit  [R] Refresh  [S] Speed Test  [H] History  [C] Clear   [?] Help    │
└───────────────────────────────────────────────────────────────────────────────┘
```

## Configuration

Edit `settings.yml` in the current directory, `~/.config/netpulse/`, or `~/.netpulse/`.

```yaml
app:
  refresh_interval: 2s
  timezone: Local
  debug: false

network:
  interfaces: []

ping:
  interval: 5s
  timeout: 10s
  packet_count: 5

dns:
  interval: 30s
  timeout: 5s
  servers:
    - 1.1.1.1:53
    - 8.8.8.8:53
  query_domain: google.com

targets:
  icmp:
    - 1.1.1.1
    - 8.8.8.8
  dns:
    - google.com
    - cloudflare.com
  http:
    - https://1.1.1.1
    - https://8.8.8.8

http:
  interval: 30s
  timeout: 10s
  method: HEAD

speedtest:
  enabled: true
  download_size_mb: 25
  upload_size_mb: 10
  workers: 4

storage:
  sqlite_path: netpulse.db

ui:
  theme: dark
  compact_mode: false
```

Supported configuration areas include refresh interval, interfaces, targets, DNS servers, speed test settings, UI theme, and SQLite path.

## Project Structure

```
├── main.go
├── settings.yml
├── go.mod / go.sum
├── internal/
│   ├── app/           # Application orchestrator
│   ├── config/        # YAML config loader (viper)
│   ├── state/         # Thread-safe AppState
│   ├── ui/            # Bubble Tea TUI dashboard
│   ├── collector/
│   │   ├── ping/      # ICMP ping collector
│   │   ├── dns/       # DNS resolver collector
│   │   ├── httpcheck/ # HTTP reachability collector
│   │   ├── netinterface/ # Network interface collector
│   │   ├── speedtest/ # Manual speed test
│   │   └── connectivity.go # Online/degraded/offline detector
│   ├── alert/         # Alert engine + evaluator
│   ├── storage/       # SQLite repository
│   └── logger/        # Structured logging (slog)
```

## Tech Stack

- **Language:** Go 1.25
- **TUI:** [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- **CLI:** [go-consolekit](https://github.com/memran/go-consolekit)
- **Config:** [Viper](https://github.com/spf13/viper)
- **Ping:** [go-ping](https://github.com/go-ping/ping)
- **DNS:** [miekg/dns](https://github.com/miekg/dns)
- **System:** [gopsutil](https://github.com/shirou/gopsutil)
- **SQLite:** [modernc.org/sqlite](https://modernc.org/sqlite)
- **Logging:** slog (stdlib)

## Requirements

- Go 1.25+
- Root/administrator privileges for ICMP ping on many systems
- A terminal with true color support (iTerm2, Kitty, modern terminals)

## Free Version Boundaries

NetPulse Community is the always-free, local-first edition of NetPulse. It gives you the core desktop dashboard, live diagnostics, and offline-safe storage for this machine only.

It does not include:

- Remote monitoring or distributed collectors
- HTTP API or WebSocket streaming
- Telegram or email alerts
- Daemon mode or startup-on-boot helpers
- SNMP, MikroTik API, NetFlow/IPFIX, PPPoE, or BGP monitoring
- Multi-user features, reports, or PDF/CSV export
- A central dashboard for multiple devices

## Paid Version Included

The paid version is designed for teams and operators who need to expand beyond one local machine. It adds the upgrade path for remote visibility, multi-device workflows, and additional automation features on top of the Community experience.

## License Model

No activation, API key, or remote verification is required for the Community edition.
