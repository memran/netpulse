package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("expected default config, got nil")
	}
	if cfg.App.RefreshInterval == 0 {
		t.Error("expected non-zero refresh interval")
	}
	if len(cfg.Targets.ICMP) == 0 {
		t.Error("expected at least one ICMP target")
	}
	if len(cfg.DNS.Servers) == 0 {
		t.Error("expected at least one DNS server")
	}
	if cfg.Ping.PacketCount < 1 {
		t.Error("expected packet count >= 1")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				App: AppConfig{RefreshInterval: 2000000000},
				Ping: PingConfig{
					Interval:    5000000000,
					Timeout:     10000000000,
					PacketCount: 5,
				},
				Targets: TargetsConfig{
					ICMP: []string{"1.1.1.1"},
				},
				Speed: SpeedConfig{
					Enabled:        true,
					DownloadSizeMB: 25,
					UploadSizeMB:   10,
					Workers:        4,
				},
			},
			wantErr: false,
		},
		{
			name: "zero packet count",
			cfg: &Config{
				Ping: PingConfig{
					PacketCount: 0,
					Interval:    5000000000,
				},
				Targets: TargetsConfig{
					ICMP: []string{"1.1.1.1"},
				},
			},
			wantErr: true,
		},
		{
			name: "no ICMP targets",
			cfg: &Config{
				Ping: PingConfig{
					PacketCount: 3,
					Interval:    5000000000,
				},
				Targets: TargetsConfig{
					ICMP: []string{},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid speed test config",
			cfg: &Config{
				Ping: PingConfig{
					PacketCount: 3,
					Interval:    5000000000,
				},
				Targets: TargetsConfig{
					ICMP: []string{"1.1.1.1"},
				},
				Speed: SpeedConfig{
					Enabled:        true,
					DownloadSizeMB: 0,
					UploadSizeMB:   0,
					Workers:        0,
				},
			},
			wantErr: true,
		},
		{
			name: "interval too short",
			cfg: &Config{
				Ping: PingConfig{
					PacketCount: 3,
					Interval:    500000000,
				},
				Targets: TargetsConfig{
					ICMP: []string{"1.1.1.1"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgContent := `
app:
  refresh_interval: 3s
  debug: true
ping:
  interval: 10s
  packet_count: 10
targets:
  icmp:
    - 8.8.8.8
`
	cfgPath := filepath.Join(dir, "settings.yml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.App.Debug != true {
		t.Errorf("expected debug=true, got %v", cfg.App.Debug)
	}
	if cfg.Ping.PacketCount != 10 {
		t.Errorf("expected PacketCount=10, got %d", cfg.Ping.PacketCount)
	}
	if len(cfg.Targets.ICMP) != 1 || cfg.Targets.ICMP[0] != "8.8.8.8" {
		t.Errorf("unexpected ICMP targets: %v", cfg.Targets.ICMP)
	}
}

func TestLoadConfigInvalidFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "settings.yml")
	if err := os.WriteFile(cfgPath, []byte("invalid: yaml: [[["), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}
