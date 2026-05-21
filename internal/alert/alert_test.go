package alert

import (
	"testing"
	"time"
)

func TestNewEngine(t *testing.T) {
	e := NewEngine(10)
	if e == nil {
		t.Fatal("expected non-nil engine")
	}
	alerts := e.Recent()
	if len(alerts) != 0 {
		t.Errorf("expected empty alerts, got %d", len(alerts))
	}
}

func TestAddAlert(t *testing.T) {
	e := NewEngine(10)
	e.Add(SeverityWarn, "ping", "Packet loss 10%")

	alerts := e.Recent()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Severity != SeverityWarn {
		t.Errorf("expected WARN, got %v", alerts[0].Severity)
	}
	if alerts[0].Source != "ping" {
		t.Errorf("expected ping, got %s", alerts[0].Source)
	}
	if alerts[0].Message != "Packet loss 10%" {
		t.Errorf("unexpected message: %s", alerts[0].Message)
	}
}

func TestMaxAlerts(t *testing.T) {
	e := NewEngine(3)
	for i := 0; i < 5; i++ {
		e.Add(SeverityInfo, "test", "test message")
	}

	alerts := e.Recent()
	if len(alerts) > 3 {
		t.Errorf("expected at most 3 alerts, got %d", len(alerts))
	}
}

func TestClearAlerts(t *testing.T) {
	e := NewEngine(10)
	e.Add(SeverityInfo, "test", "test")
	e.Clear()

	alerts := e.Recent()
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts after clear, got %d", len(alerts))
	}
}

func TestRecentBySeverity(t *testing.T) {
	e := NewEngine(10)
	e.Add(SeverityInfo, "test", "info message")
	e.Add(SeverityWarn, "test", "warn message")
	e.Add(SeverityCritical, "test", "critical message")

	critAlerts := e.RecentBySeverity(SeverityCritical)
	if len(critAlerts) != 1 {
		t.Errorf("expected 1 critical alert, got %d", len(critAlerts))
	}

	warnAlerts := e.RecentBySeverity(SeverityWarn)
	if len(warnAlerts) != 1 {
		t.Errorf("expected 1 warn alert, got %d", len(warnAlerts))
	}
}

func TestAlertOrder(t *testing.T) {
	e := NewEngine(10)
	e.Add(SeverityInfo, "test", "first")
	e.Add(SeverityInfo, "test", "second")

	alerts := e.Recent()
	if alerts[0].Message != "second" {
		t.Errorf("expected newest first, got %s", alerts[0].Message)
	}
}

func TestAlertID(t *testing.T) {
	e := NewEngine(10)
	e.Add(SeverityInfo, "test", "msg1")
	e.Add(SeverityInfo, "test", "msg2")

	alerts := e.Recent()
	if alerts[0].ID <= alerts[1].ID {
		t.Errorf("expected newer alert to have higher ID")
	}
}

func TestAlertTimestamp(t *testing.T) {
	e := NewEngine(10)
	before := time.Now()
	e.Add(SeverityInfo, "test", "msg")
	after := time.Now()

	alerts := e.Recent()
	if alerts[0].Timestamp.Before(before) || alerts[0].Timestamp.After(after) {
		t.Errorf("timestamp out of range")
	}
}

func TestSeverityString(t *testing.T) {
	tests := []struct {
		severity Severity
		want     string
	}{
		{SeverityInfo, "INFO"},
		{SeverityWarn, "WARN"},
		{SeverityCritical, "CRIT"},
	}
	for _, tt := range tests {
		if got := tt.severity.String(); got != tt.want {
			t.Errorf("got %s, want %s", got, tt.want)
		}
	}
}

func TestAlertRules(t *testing.T) {
	rule := PacketLossRule(5)
	if rule.Name != "packet_loss" {
		t.Errorf("expected packet_loss, got %s", rule.Name)
	}

	rule = LatencyRule(100)
	if rule.Name != "high_latency" {
		t.Errorf("expected high_latency, got %s", rule.Name)
	}

	rule = JitterRule(20)
	if rule.Name != "high_jitter" {
		t.Errorf("expected high_jitter, got %s", rule.Name)
	}
}
