package state

import (
	"sync"
	"time"
)

type ConnectivityStatus int

const (
	StatusUnknown ConnectivityStatus = iota
	StatusOnline
	StatusDegraded
	StatusOffline
)

func (s ConnectivityStatus) String() string {
	switch s {
	case StatusOnline:
		return "ONLINE"
	case StatusDegraded:
		return "DEGRADED"
	case StatusOffline:
		return "OFFLINE"
	default:
		return "UNKNOWN"
	}
}

type InterfaceStats struct {
	Name      string  `json:"name"`
	RXBytes   uint64  `json:"rx_bytes"`
	TXBytes   uint64  `json:"tx_bytes"`
	RXSpeed     float64 `json:"rx_speed"` // Mbps
	TXSpeed     float64 `json:"tx_speed"` // Mbps
	TotalRX     uint64  `json:"total_rx"`
	TotalTX     uint64  `json:"total_tx"`
	PacketsIn   uint64  `json:"packets_in"`
	PacketsOut  uint64  `json:"packets_out"`
	Drops       uint64  `json:"drops"`
	Errors      uint64  `json:"errors"`
	Up          bool    `json:"up"`
	IPAddress   string  `json:"ip_address"`
	MACAddress  string  `json:"mac_address"`
	Gateway     string  `json:"gateway"`
	MTU         int     `json:"mtu"`
	UpdatedAt time.Time
}

type PingStats struct {
	Target      string  `json:"target"`
	AvgLatency  float64 `json:"avg_latency"` // ms
	MinLatency  float64 `json:"min_latency"`
	MaxLatency  float64 `json:"max_latency"`
	PacketLoss  float64 `json:"packet_loss"` // percentage
	Jitter      float64 `json:"jitter"`      // ms
	Sent        int     `json:"sent"`
	Received    int     `json:"received"`
	LastSuccess bool    `json:"last_success"`
	UpdatedAt   time.Time
}

type DNSStats struct {
	Server       string  `json:"server"`
	Domain       string  `json:"domain"`
	ResponseTime float64 `json:"response_time"` // ms
	AAddresses   []string `json:"a_addresses"`
	AAAAAddresses []string `json:"aaaa_addresses"`
	Success      bool    `json:"success"`
	Error        string  `json:"error,omitempty"`
	UpdatedAt    time.Time
}

type HTTPStats struct {
	URL          string  `json:"url"`
	StatusCode   int     `json:"status_code"`
	ResponseTime float64 `json:"response_time"` // ms
	TLSAvailable bool    `json:"tls_available"`
	Success      bool    `json:"success"`
	Error        string  `json:"error,omitempty"`
	UpdatedAt    time.Time
}

type SpeedTestResult struct {
	DownloadMbps  float64   `json:"download_mbps"`
	UploadMbps    float64   `json:"upload_mbps"`
	LatencyMs     float64   `json:"latency_ms"`
	Running       bool      `json:"running"`
	Error         string    `json:"error,omitempty"`
	CompletedAt   time.Time `json:"completed_at"`
}

type TracerouteHop struct {
	Hop  int
	IP   string
	RTTs []float64
}

type TracerouteResult struct {
	Target      string
	Hops        []TracerouteHop
	Running     bool
	Error       string
	CompletedAt time.Time
}

type AppState struct {
	mu sync.RWMutex

	InternetStatus ConnectivityStatus
	PublicIP       string

	Interfaces map[string]InterfaceStats
	PingStats  map[string]PingStats
	DNSStats   map[string]DNSStats
	HTTPStats  map[string]HTTPStats

	SpeedTest  SpeedTestResult
	Traceroute TracerouteResult

	LastUpdated time.Time

	subscribers []chan struct{}
}

func New() *AppState {
	return &AppState{
		Interfaces: make(map[string]InterfaceStats),
		PingStats:  make(map[string]PingStats),
		DNSStats:   make(map[string]DNSStats),
		HTTPStats:  make(map[string]HTTPStats),
	}
}

func (s *AppState) Subscribe() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan struct{}, 1)
	s.subscribers = append(s.subscribers, ch)
	return ch
}

func (s *AppState) notify() {
	for _, ch := range s.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *AppState) Update(fn func(*AppState)) {
	s.mu.Lock()
	fn(s)
	s.LastUpdated = time.Now()
	s.mu.Unlock()
	s.notify()
}

func (s *AppState) Read() AppStateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return AppStateSnapshot{
		InternetStatus: s.InternetStatus,
		PublicIP:       s.PublicIP,
		Interfaces:     copyMap(s.Interfaces),
		PingStats:      copyMap(s.PingStats),
		DNSStats:       copyMap(s.DNSStats),
		HTTPStats:      copyMap(s.HTTPStats),
		SpeedTest:      s.SpeedTest,
		Traceroute:     s.Traceroute,
		LastUpdated:    s.LastUpdated,
	}
}

type AppStateSnapshot struct {
	InternetStatus ConnectivityStatus
	PublicIP       string
	Interfaces     map[string]InterfaceStats
	PingStats      map[string]PingStats
	DNSStats       map[string]DNSStats
	HTTPStats      map[string]HTTPStats
	SpeedTest      SpeedTestResult
	Traceroute     TracerouteResult
	LastUpdated    time.Time
}

func (s *AppState) SetConnectivity(status ConnectivityStatus, publicIP string) {
	s.Update(func(as *AppState) {
		as.InternetStatus = status
		if publicIP != "" {
			as.PublicIP = publicIP
		}
	})
}

func (s *AppState) SetInterfaceStats(name string, stats InterfaceStats) {
	s.Update(func(as *AppState) {
		stats.UpdatedAt = time.Now()
		as.Interfaces[name] = stats
	})
}

func (s *AppState) SetPingStats(stats PingStats) {
	s.Update(func(as *AppState) {
		stats.UpdatedAt = time.Now()
		as.PingStats[stats.Target] = stats
	})
}

func (s *AppState) SetDNSStats(stats DNSStats) {
	s.Update(func(as *AppState) {
		stats.UpdatedAt = time.Now()
		as.DNSStats[stats.Server+"|"+stats.Domain] = stats
	})
}

func (s *AppState) SetHTTPStats(stats HTTPStats) {
	s.Update(func(as *AppState) {
		stats.UpdatedAt = time.Now()
		as.HTTPStats[stats.URL] = stats
	})
}

func (s *AppState) SetSpeedTest(result SpeedTestResult) {
	s.Update(func(as *AppState) {
		as.SpeedTest = result
	})
}

func (s *AppState) SetSpeedTestRunning(running bool) {
	s.Update(func(as *AppState) {
		as.SpeedTest.Running = running
	})
}

func (s *AppState) SetTraceroute(result TracerouteResult) {
	s.Update(func(as *AppState) {
		as.Traceroute = result
	})
}

func (s *AppState) SetTracerouteRunning(running bool) {
	s.Update(func(as *AppState) {
		as.Traceroute.Running = running
	})
}

func copyMap[K comparable, V any](src map[K]V) map[K]V {
	dst := make(map[K]V, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
