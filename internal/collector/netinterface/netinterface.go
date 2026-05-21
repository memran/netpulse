package netinterface

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/net"

	"github.com/memran/netpulse/internal/logger"
	"github.com/memran/netpulse/internal/state"
)

type Collector struct {
	log      *logger.Logger
	st       *state.AppState
	interval time.Duration
	names    []string
	prevIO   map[string]net.IOCountersStat
}

func NewCollector(log *logger.Logger, st *state.AppState, interval time.Duration, names []string) *Collector {
	return &Collector{
		log:      log.WithComponent("collector/netinterface"),
		st:       st,
		interval: interval,
		names:    names,
		prevIO:   make(map[string]net.IOCountersStat),
	}
}

func (c *Collector) Start(ctx context.Context) {
	c.log.Infof("starting interface collector (interval=%s)", c.interval)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.collect()
	for {
		select {
		case <-ticker.C:
			c.collect()
		case <-ctx.Done():
			c.log.Info("interface collector stopped")
			return
		}
	}
}

func (c *Collector) collect() {
	counters, err := net.IOCounters(true)
	if err != nil {
		c.log.Warnf("failed to get interface counters: %v", err)
		return
	}

	for _, io := range counters {
		if len(c.names) > 0 && !contains(c.names, io.Name) {
			continue
		}

		stats := state.InterfaceStats{
			Name:       io.Name,
			RXBytes:    io.BytesRecv,
			TXBytes:    io.BytesSent,
			TotalRX:    io.BytesRecv,
			TotalTX:    io.BytesSent,
			PacketsIn:  io.PacketsRecv,
			PacketsOut: io.PacketsSent,
			Drops:      io.Dropout + io.Dropin,
			Errors:     io.Errin + io.Errout,
			Up:         true,
			Gateway:    getDefaultGateway(),
		}

		if prev, ok := c.prevIO[io.Name]; ok {
			elapsed := c.interval.Seconds()
			if elapsed > 0 {
				rxBps := float64(io.BytesRecv-prev.BytesRecv) / elapsed
				txBps := float64(io.BytesSent-prev.BytesSent) / elapsed
				stats.RXSpeed = rxBps * 8 / 1_000_000
				stats.TXSpeed = txBps * 8 / 1_000_000
			}
		}

		c.prevIO[io.Name] = io
		c.st.SetInterfaceStats(io.Name, stats)
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}

	for _, iface := range ifaces {
		if len(c.names) > 0 && !contains(c.names, iface.Name) {
			continue
		}
		name := iface.Name
		up := false
		for _, flag := range iface.Flags {
			if flag == "up" || flag == "UP" {
				up = true
				break
			}
		}
		
		ipAddr := ""
		for _, addr := range iface.Addrs {
			a := addr.Addr
			// Strip CIDR prefix (e.g. "192.168.0.100/24" -> "192.168.0.100")
			if idx := strings.Index(a, "/"); idx != -1 {
				a = a[:idx]
			}
			// Prefer IPv4 (contains dot, no colon)
			if strings.Contains(a, ".") && !strings.Contains(a, ":") {
				ipAddr = a
				break
			}
			// Keep first IPv6 as fallback only if no IPv4 found yet
			if ipAddr == "" {
				ipAddr = a
			}
		}
		
		c.st.Update(func(as *state.AppState) {
			if existing, ok := as.Interfaces[name]; ok {
				existing.Up = up
				existing.MACAddress = iface.HardwareAddr
				existing.MTU = iface.MTU
				if ipAddr != "" {
					existing.IPAddress = ipAddr
				}
				existing.UpdatedAt = time.Now()
				as.Interfaces[name] = existing
			}
		})
	}
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func getDefaultGateway() string {
	var out bytes.Buffer
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("netstat", "-rn")
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			for _, line := range strings.Split(out.String(), "\n") {
				if strings.Contains(line, "default") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						return fields[1]
					}
				}
			}
		}
	case "linux":
		cmd := exec.Command("ip", "route")
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			for _, line := range strings.Split(out.String(), "\n") {
				if strings.HasPrefix(line, "default via") {
					fields := strings.Fields(line)
					if len(fields) >= 3 {
						return fields[2]
					}
				}
			}
		}
	case "windows":
		cmd := exec.Command("route", "print", "0.0.0.0")
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			for _, line := range strings.Split(out.String(), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 3 && fields[0] == "0.0.0.0" {
					return fields[2]
				}
			}
		}
	}
	return "N/A"
}
