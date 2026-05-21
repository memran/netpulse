package alert

import (
	"fmt"
	"sync"
	"time"
)

type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarn:
		return "WARN"
	case SeverityCritical:
		return "CRIT"
	default:
		return "UNKN"
	}
}

type Alert struct {
	ID        int       `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Severity  Severity  `json:"severity"`
	Message   string    `json:"message"`
	Source    string    `json:"source"`
}

type AlertRule struct {
	Name        string
	Description string
	Severity    Severity
	Evaluate    func() (bool, string)
}

type Engine struct {
	mu     sync.RWMutex
	alerts []Alert
	nextID int
	maxAlerts int
	rules     []AlertRule
	onAlert   func(Alert)
}

func NewEngine(maxAlerts int) *Engine {
	return &Engine{
		alerts:    make([]Alert, 0, maxAlerts),
		maxAlerts: maxAlerts,
		rules:     make([]AlertRule, 0),
	}
}

func (e *Engine) AddRule(rule AlertRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

func (e *Engine) SetRules(rules []AlertRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = rules
}

func (e *Engine) SetOnAlert(fn func(Alert)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onAlert = fn
}

func (e *Engine) Add(severity Severity, source, message string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.nextID++
	a := Alert{
		ID:        e.nextID,
		Timestamp: time.Now(),
		Severity:  severity,
		Message:   message,
		Source:    source,
	}

	e.alerts = append([]Alert{a}, e.alerts...)
	if len(e.alerts) > e.maxAlerts {
		e.alerts = e.alerts[:e.maxAlerts]
	}
	
	if e.onAlert != nil {
		go e.onAlert(a)
	}
}

func (e *Engine) AddAlert(a Alert) {
	e.Add(a.Severity, a.Source, a.Message)
}

func (e *Engine) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.alerts = make([]Alert, 0, e.maxAlerts)
}

func (e *Engine) Recent() []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]Alert, len(e.alerts))
	copy(result, e.alerts)
	return result
}

func (e *Engine) RecentBySeverity(severity Severity) []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var result []Alert
	for _, a := range e.alerts {
		if a.Severity == severity {
			result = append(result, a)
		}
	}
	return result
}

func PacketLossRule(threshold float64) AlertRule {
	return AlertRule{
		Name:        "packet_loss",
		Description: fmt.Sprintf("Packet loss > %.0f%%", threshold),
		Severity:    SeverityWarn,
		Evaluate: func() (bool, string) {
			return false, ""
		},
	}
}

func LatencyRule(thresholdMs float64) AlertRule {
	return AlertRule{
		Name:        "high_latency",
		Description: fmt.Sprintf("Latency > %.0fms", thresholdMs),
		Severity:    SeverityWarn,
		Evaluate: func() (bool, string) {
			return false, ""
		},
	}
}

func JitterRule(thresholdMs float64) AlertRule {
	return AlertRule{
		Name:        "high_jitter",
		Description: fmt.Sprintf("Jitter > %.0fms", thresholdMs),
		Severity:    SeverityWarn,
		Evaluate: func() (bool, string) {
			return false, ""
		},
	}
}
