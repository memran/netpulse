package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go-consolekit/console"
	"gopkg.in/yaml.v3"
)

const defaultYAML = `app:
  refresh_interval: 2s
  timezone: Local
  debug: false

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
  http:
    - https://google.com
    - https://youtube.com
    - https://cloudflare.com

http:
  interval: 30s
  timeout: 10s
  method: HEAD

speedtest:
  enabled: true
  download_size_mb: 25
  upload_size_mb: 10
  workers: 4
  download_url: "https://speed.cloudflare.com/__down"
  upload_url: "https://speed.cloudflare.com/__up"

traceroute:
  target: 1.1.1.1
  max_hops: 30
  probes: 3

storage:
  sqlite_path: netpulse.db

ui:
  theme: dark
  compact_mode: false
`

type Config struct {
	App        AppConfig
	Network    NetworkConfig
	Ping       PingConfig
	DNS        DNSConfig
	Targets    TargetsConfig
	HTTP       HTTPConfig
	Speed      SpeedConfig
	Traceroute TracerouteConfig
	Storage    StorageConfig
	UI         UIConfig
}

type AppConfig struct {
	RefreshInterval time.Duration
	Timezone        string
	Debug           bool
}

type NetworkConfig struct {
	Interfaces []string
}

type PingConfig struct {
	Interval    time.Duration
	Timeout     time.Duration
	PacketCount int
}

type DNSConfig struct {
	Interval    time.Duration
	Timeout     time.Duration
	Servers     []string
	QueryDomain string
}

type TargetsConfig struct {
	ICMP []string
	DNS  []string
	HTTP []string
}

type HTTPConfig struct {
	Interval time.Duration
	Timeout  time.Duration
	Method   string
}

type SpeedConfig struct {
	Enabled        bool
	DownloadSizeMB int64
	UploadSizeMB   int64
	Workers        int
	DownloadURL    string
	UploadURL      string
}

type TracerouteConfig struct {
	Target  string
	MaxHops int
	Probes  int
}

type StorageConfig struct {
	SQLitePath string
}

type UIConfig struct {
	Theme       string
	CompactMode bool
}

func Default() *Config {
	return &Config{
		App: AppConfig{
			RefreshInterval: 2 * time.Second,
			Timezone:        "Local",
			Debug:           false,
		},
		Network: NetworkConfig{
			Interfaces: []string{},
		},
		Ping: PingConfig{
			Interval:    5 * time.Second,
			Timeout:     10 * time.Second,
			PacketCount: 5,
		},
		DNS: DNSConfig{
			Interval:    30 * time.Second,
			Timeout:     5 * time.Second,
			Servers:     []string{"1.1.1.1:53", "8.8.8.8:53"},
			QueryDomain: "google.com",
		},
		Targets: TargetsConfig{
			ICMP: []string{"1.1.1.1", "8.8.8.8"},
			DNS:  []string{"google.com", "cloudflare.com"},
			HTTP: []string{"https://1.1.1.1", "https://8.8.8.8"},
		},
		HTTP: HTTPConfig{
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
			Method:   "HEAD",
		},
		Speed: SpeedConfig{
			Enabled:        true,
			DownloadSizeMB: 25,
			UploadSizeMB:   10,
			Workers:        4,
			DownloadURL:    "https://speed.cloudflare.com/__down",
			UploadURL:      "https://speed.cloudflare.com/__up",
		},
		Traceroute: TracerouteConfig{
			Target:  "1.1.1.1",
			MaxHops: 30,
			Probes:  3,
		},
		Storage: StorageConfig{
			SQLitePath: "netpulse.db",
		},
		UI: UIConfig{
			Theme:       "dark",
			CompactMode: false,
		},
	}
}

func Load(paths ...string) (*Config, error) {
	path, found, err := findConfigFile(paths...)
	if err != nil {
		return nil, err
	}
	if !found {
		_ = os.WriteFile("settings.yml", []byte(defaultYAML), 0o644)
		return Default(), nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Keep parse errors visible; console.Config swallows them internally.
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfgStore := console.NewConfig()
	cfgStore.LoadYAML(path)

	cfg := Default()
	if err := applyConfigStore(cfg, cfgStore); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	cfg.enforceFreeLimits()
	return cfg, nil
}

func WriteDefaultFile(path string) error {
	target := path
	if target == "" {
		target = "settings.yml"
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil && filepath.Dir(target) != "." {
		return err
	}
	return os.WriteFile(target, []byte(defaultYAML), 0o644)
}

func findConfigFile(paths ...string) (string, bool, error) {
	searchPaths := []string{"."}
	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(home, ".config", "netpulse"))
		searchPaths = append(searchPaths, filepath.Join(home, ".netpulse"))
	}
	searchPaths = append(searchPaths, "/etc/netpulse")
	if len(paths) > 0 && paths[0] != "" {
		searchPaths = append([]string{paths[0]}, searchPaths...)
	}

	for _, p := range searchPaths {
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", false, fmt.Errorf("read config: %w", err)
		}

		candidate := p
		if info.IsDir() {
			candidate = filepath.Join(p, "settings.yml")
		}

		if _, err := os.Stat(candidate); err == nil {
			return candidate, true, nil
		} else if !os.IsNotExist(err) {
			return "", false, fmt.Errorf("read config: %w", err)
		}
	}

	return "", false, nil
}

func applyConfigStore(cfg *Config, c *console.Config) error {
	cfg.App.RefreshInterval = durationValue(c, "app.refresh_interval", cfg.App.RefreshInterval)
	cfg.App.Timezone = c.GetString("app.timezone", cfg.App.Timezone)
	cfg.App.Debug = c.GetBool("app.debug", cfg.App.Debug)

	cfg.Network.Interfaces = stringSliceValue(c, "network.interfaces", cfg.Network.Interfaces)

	cfg.Ping.Interval = durationValue(c, "ping.interval", cfg.Ping.Interval)
	cfg.Ping.Timeout = durationValue(c, "ping.timeout", cfg.Ping.Timeout)
	cfg.Ping.PacketCount = c.GetInt("ping.packet_count", cfg.Ping.PacketCount)

	cfg.DNS.Interval = durationValue(c, "dns.interval", cfg.DNS.Interval)
	cfg.DNS.Timeout = durationValue(c, "dns.timeout", cfg.DNS.Timeout)
	cfg.DNS.Servers = stringSliceValue(c, "dns.servers", cfg.DNS.Servers)
	cfg.DNS.QueryDomain = c.GetString("dns.query_domain", cfg.DNS.QueryDomain)

	cfg.Targets.ICMP = stringSliceValue(c, "targets.icmp", cfg.Targets.ICMP)
	cfg.Targets.DNS = stringSliceValue(c, "targets.dns", cfg.Targets.DNS)
	cfg.Targets.HTTP = stringSliceValue(c, "targets.http", cfg.Targets.HTTP)

	cfg.HTTP.Interval = durationValue(c, "http.interval", cfg.HTTP.Interval)
	cfg.HTTP.Timeout = durationValue(c, "http.timeout", cfg.HTTP.Timeout)
	cfg.HTTP.Method = c.GetString("http.method", cfg.HTTP.Method)

	cfg.Speed.Enabled = c.GetBool("speedtest.enabled", cfg.Speed.Enabled)
	cfg.Speed.DownloadSizeMB = int64(c.GetInt("speedtest.download_size_mb", int(cfg.Speed.DownloadSizeMB)))
	cfg.Speed.UploadSizeMB = int64(c.GetInt("speedtest.upload_size_mb", int(cfg.Speed.UploadSizeMB)))
	cfg.Speed.Workers = c.GetInt("speedtest.workers", cfg.Speed.Workers)
	cfg.Speed.DownloadURL = c.GetString("speedtest.download_url", cfg.Speed.DownloadURL)
	cfg.Speed.UploadURL = c.GetString("speedtest.upload_url", cfg.Speed.UploadURL)

	cfg.Traceroute.Target = c.GetString("traceroute.target", cfg.Traceroute.Target)
	cfg.Traceroute.MaxHops = c.GetInt("traceroute.max_hops", cfg.Traceroute.MaxHops)
	cfg.Traceroute.Probes = c.GetInt("traceroute.probes", cfg.Traceroute.Probes)

	cfg.Storage.SQLitePath = c.GetString("storage.sqlite_path", cfg.Storage.SQLitePath)

	cfg.UI.Theme = c.GetString("ui.theme", cfg.UI.Theme)
	cfg.UI.CompactMode = c.GetBool("ui.compact_mode", cfg.UI.CompactMode)
	return nil
}

func durationValue(c *console.Config, key string, fallback time.Duration) time.Duration {
	val := c.Get(key)
	switch v := val.(type) {
	case string:
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	case int:
		return time.Duration(v)
	case int64:
		return time.Duration(v)
	case float64:
		return time.Duration(v)
	}
	return fallback
}

func stringSliceValue(c *console.Config, key string, fallback []string) []string {
	val := c.Get(key)
	switch v := val.(type) {
	case []string:
		return append([]string(nil), v...)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return fallback
	}
}

func (c *Config) enforceFreeLimits() {
	total := 0
	limit := 5

	icmp := []string{}
	for _, t := range c.Targets.ICMP {
		if total < limit {
			icmp = append(icmp, t)
			total++
		}
	}
	c.Targets.ICMP = icmp

	dns := []string{}
	for _, t := range c.Targets.DNS {
		if total < limit {
			dns = append(dns, t)
			total++
		}
	}
	c.Targets.DNS = dns

	http := []string{}
	for _, t := range c.Targets.HTTP {
		if total < limit {
			http = append(http, t)
			total++
		}
	}
	c.Targets.HTTP = http
}

func (c *Config) validate() error {
	if c.Ping.PacketCount < 1 {
		return fmt.Errorf("ping.packet_count must be >= 1")
	}
	if c.Ping.Interval < time.Second {
		return fmt.Errorf("ping.interval must be >= 1s")
	}
	if c.Speed.Enabled {
		if c.Speed.DownloadSizeMB < 1 {
			return fmt.Errorf("speedtest.download_size_mb must be >= 1")
		}
		if c.Speed.UploadSizeMB < 1 {
			return fmt.Errorf("speedtest.upload_size_mb must be >= 1")
		}
		if c.Speed.Workers < 1 {
			return fmt.Errorf("speedtest.workers must be >= 1")
		}
	}
	return nil
}
