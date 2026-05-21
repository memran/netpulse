package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
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

storage:
  sqlite_path: netpulse.db

ui:
  theme: dark
  compact_mode: false
`

type Config struct {
	App     AppConfig     `mapstructure:"app"`
	Network NetworkConfig `mapstructure:"network"`
	Ping    PingConfig    `mapstructure:"ping"`
	DNS     DNSConfig     `mapstructure:"dns"`
	Targets TargetsConfig `mapstructure:"targets"`
	HTTP    HTTPConfig    `mapstructure:"http"`
	Speed   SpeedConfig   `mapstructure:"speedtest"`
	Storage StorageConfig `mapstructure:"storage"`
	UI      UIConfig      `mapstructure:"ui"`
}

type AppConfig struct {
	RefreshInterval time.Duration `mapstructure:"refresh_interval"`
	Timezone        string        `mapstructure:"timezone"`
	Debug           bool          `mapstructure:"debug"`
}

type NetworkConfig struct {
	Interfaces []string `mapstructure:"interfaces"`
}

type PingConfig struct {
	Interval    time.Duration `mapstructure:"interval"`
	Timeout     time.Duration `mapstructure:"timeout"`
	PacketCount int           `mapstructure:"packet_count"`
}

type DNSConfig struct {
	Interval    time.Duration `mapstructure:"interval"`
	Timeout     time.Duration `mapstructure:"timeout"`
	Servers     []string      `mapstructure:"servers"`
	QueryDomain string        `mapstructure:"query_domain"`
}

type TargetsConfig struct {
	ICMP []string `mapstructure:"icmp"`
	DNS  []string `mapstructure:"dns"`
	HTTP []string `mapstructure:"http"`
}

type HTTPConfig struct {
	Interval time.Duration `mapstructure:"interval"`
	Timeout  time.Duration `mapstructure:"timeout"`
	Method   string        `mapstructure:"method"`
}

type SpeedConfig struct {
	Enabled         bool  `mapstructure:"enabled"`
	DownloadSizeMB  int64 `mapstructure:"download_size_mb"`
	UploadSizeMB    int64 `mapstructure:"upload_size_mb"`
	Workers         int   `mapstructure:"workers"`
	DownloadURL     string `mapstructure:"download_url"`
	UploadURL       string `mapstructure:"upload_url"`
}

type StorageConfig struct {
	SQLitePath string `mapstructure:"sqlite_path"`
}

type UIConfig struct {
	Theme       string `mapstructure:"theme"`
	CompactMode bool   `mapstructure:"compact_mode"`
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
			Enabled:         true,
			DownloadSizeMB:  25,
			UploadSizeMB:    10,
			Workers:         4,
			DownloadURL:     "https://speed.cloudflare.com/__down",
			UploadURL:       "https://speed.cloudflare.com/__up",
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
	v := viper.New()
	v.SetConfigName("settings")
	v.SetConfigType("yaml")

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
		v.AddConfigPath(p)
	}

	v.SetDefault("app.refresh_interval", "2s")
	v.SetDefault("app.timezone", "Local")
	v.SetDefault("app.debug", false)

	v.SetDefault("ping.interval", "5s")
	v.SetDefault("ping.timeout", "10s")
	v.SetDefault("ping.packet_count", 5)

	v.SetDefault("dns.interval", "30s")
	v.SetDefault("dns.timeout", "5s")
	v.SetDefault("dns.servers", []string{"1.1.1.1:53", "8.8.8.8:53"})
	v.SetDefault("dns.query_domain", "google.com")

	v.SetDefault("targets.icmp", []string{"1.1.1.1", "8.8.8.8"})
	v.SetDefault("targets.dns", []string{"google.com", "cloudflare.com"})
	v.SetDefault("targets.http", []string{"https://1.1.1.1", "https://8.8.8.8"})

	v.SetDefault("http.interval", "30s")
	v.SetDefault("http.timeout", "10s")
	v.SetDefault("http.method", "HEAD")

	v.SetDefault("speedtest.enabled", true)
	v.SetDefault("speedtest.download_size_mb", 25)
	v.SetDefault("speedtest.upload_size_mb", 10)
	v.SetDefault("speedtest.workers", 4)
	v.SetDefault("speedtest.download_url", "https://speed.cloudflare.com/__down")
	v.SetDefault("speedtest.upload_url", "https://speed.cloudflare.com/__up")

	v.SetDefault("storage.sqlite_path", "netpulse.db")

	v.SetDefault("ui.theme", "dark")
	v.SetDefault("ui.compact_mode", false)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			_ = os.WriteFile("settings.yml", []byte(defaultYAML), 0644)
			return Default(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	
	cfg.enforceFreeLimits()

	return &cfg, nil
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
