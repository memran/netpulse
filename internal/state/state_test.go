package state

import (
	"testing"
)

func TestNewState(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("expected non-nil state")
	}

	snapshot := s.Read()
	if snapshot.InternetStatus != StatusUnknown {
		t.Errorf("expected StatusUnknown, got %v", snapshot.InternetStatus)
	}
	if len(snapshot.Interfaces) != 0 {
		t.Errorf("expected empty interfaces, got %d", len(snapshot.Interfaces))
	}
}

func TestSetConnectivity(t *testing.T) {
	s := New()
	s.SetConnectivity(StatusOnline, "1.2.3.4")

	snapshot := s.Read()
	if snapshot.InternetStatus != StatusOnline {
		t.Errorf("expected StatusOnline, got %v", snapshot.InternetStatus)
	}
	if snapshot.PublicIP != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %s", snapshot.PublicIP)
	}
}

func TestSetInterfaceStats(t *testing.T) {
	s := New()
	stats := InterfaceStats{
		Name:    "en0",
		RXBytes: 1000,
		TXBytes: 2000,
		Drops:   5,
		Errors:  1,
		Up:      true,
	}
	s.SetInterfaceStats("en0", stats)

	snapshot := s.Read()
	iface, ok := snapshot.Interfaces["en0"]
	if !ok {
		t.Fatal("expected interface en0")
	}
	if iface.RXBytes != 1000 {
		t.Errorf("expected 1000, got %d", iface.RXBytes)
	}
	if iface.TXBytes != 2000 {
		t.Errorf("expected 2000, got %d", iface.TXBytes)
	}
	if iface.Drops != 5 {
		t.Errorf("expected 5, got %d", iface.Drops)
	}
	if iface.Errors != 1 {
		t.Errorf("expected 1, got %d", iface.Errors)
	}
	if !iface.Up {
		t.Error("expected up=true")
	}
}

func TestSetPingStats(t *testing.T) {
	s := New()
	stats := PingStats{
		Target:      "1.1.1.1",
		AvgLatency:  5.0,
		PacketLoss:  0,
		Jitter:      1.5,
		LastSuccess: true,
		Sent:        5,
		Received:    5,
	}
	s.SetPingStats(stats)

	snapshot := s.Read()
	ps, ok := snapshot.PingStats["1.1.1.1"]
	if !ok {
		t.Fatal("expected ping stats for 1.1.1.1")
	}
	if ps.AvgLatency != 5.0 {
		t.Errorf("expected 5.0, got %f", ps.AvgLatency)
	}
	if ps.PacketLoss != 0 {
		t.Errorf("expected 0, got %f", ps.PacketLoss)
	}
	if ps.Received != 5 {
		t.Errorf("expected 5, got %d", ps.Received)
	}
}

func TestSetDNSStats(t *testing.T) {
	s := New()
	stats := DNSStats{
		Server:       "1.1.1.1:53",
		Domain:       "google.com",
		ResponseTime: 12.5,
		AAddresses:   []string{"142.250.80.46"},
		Success:      true,
	}
	s.SetDNSStats(stats)

	snapshot := s.Read()
	key := "1.1.1.1:53|google.com"
	ds, ok := snapshot.DNSStats[key]
	if !ok {
		t.Fatal("expected DNS stats")
	}
	if ds.ResponseTime != 12.5 {
		t.Errorf("expected 12.5, got %f", ds.ResponseTime)
	}
	if !ds.Success {
		t.Error("expected success")
	}
}

func TestSetHTTPStats(t *testing.T) {
	s := New()
	stats := HTTPStats{
		URL:          "https://1.1.1.1",
		StatusCode:   200,
		ResponseTime: 45.0,
		TLSAvailable: true,
		Success:      true,
	}
	s.SetHTTPStats(stats)

	snapshot := s.Read()
	hs, ok := snapshot.HTTPStats["https://1.1.1.1"]
	if !ok {
		t.Fatal("expected HTTP stats for https://1.1.1.1")
	}
	if hs.StatusCode != 200 {
		t.Errorf("expected 200, got %d", hs.StatusCode)
	}
	if !hs.TLSAvailable {
		t.Error("expected TLS available")
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New()
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			s.SetPingStats(PingStats{
				Target:     "1.1.1.1",
				AvgLatency: float64(i),
			})
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = s.Read()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			s.SetInterfaceStats("en0", InterfaceStats{RXBytes: uint64(i)})
		}
		done <- true
	}()

	for i := 0; i < 3; i++ {
		<-done
	}

	snapshot := s.Read()
	if ps, ok := snapshot.PingStats["1.1.1.1"]; ok {
		t.Logf("final ping latency: %f", ps.AvgLatency)
	}
}

func TestSubscribe(t *testing.T) {
	s := New()
	ch := s.Subscribe()

	s.SetConnectivity(StatusOnline, "1.2.3.4")
	<-ch

	s.SetPingStats(PingStats{Target: "8.8.8.8"})
	<-ch
}

func TestConnectivityStatusString(t *testing.T) {
	tests := []struct {
		status ConnectivityStatus
		want   string
	}{
		{StatusUnknown, "UNKNOWN"},
		{StatusOnline, "ONLINE"},
		{StatusDegraded, "DEGRADED"},
		{StatusOffline, "OFFLINE"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("got %s, want %s", got, tt.want)
		}
	}
}
