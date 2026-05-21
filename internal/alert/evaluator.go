package alert

import (
	"context"
	"fmt"
	"time"

	"github.com/memran/netpulse/internal/logger"
	"github.com/memran/netpulse/internal/state"
)

type Evaluator struct {
	log              *logger.Logger
	st               *state.AppState
	alerts           *Engine
	lossThreshold    float64
	latencyThreshold float64
	jitterThreshold  float64
}

func NewEvaluator(log *logger.Logger, st *state.AppState, alerts *Engine, lossThreshold, latencyThreshold, jitterThreshold float64) *Evaluator {
	return &Evaluator{
		log:              log.WithComponent("alert/evaluator"),
		st:               st,
		alerts:           alerts,
		lossThreshold:    lossThreshold,
		latencyThreshold: latencyThreshold,
		jitterThreshold:  jitterThreshold,
	}
}

func (e *Evaluator) Start(ctx context.Context) {
	e.log.Info("starting alert evaluator")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.evaluate()
		case <-ctx.Done():
			e.log.Info("alert evaluator stopped")
			return
		}
	}
}

func (e *Evaluator) evaluate() {
	snapshot := e.st.Read()

	for _, ps := range snapshot.PingStats {
		if ps.PacketLoss > e.lossThreshold && ps.Sent > 0 {
			e.alerts.Add(SeverityWarn, "ping",
				formatAlert("Packet loss %.1f%% to %s", ps.PacketLoss, ps.Target))
		}
		if ps.AvgLatency > e.latencyThreshold && ps.Received > 0 {
			e.alerts.Add(SeverityWarn, "ping",
				formatAlert("High latency %.0fms to %s", ps.AvgLatency, ps.Target))
		}
		if ps.Jitter > e.jitterThreshold && ps.Received > 1 {
			e.alerts.Add(SeverityWarn, "ping",
				formatAlert("High jitter %.1fms to %s", ps.Jitter, ps.Target))
		}
	}

	for _, ds := range snapshot.DNSStats {
		if !ds.Success && ds.Error != "" {
			e.alerts.Add(SeverityCritical, "dns",
				formatAlert("DNS failure: %s via %s", ds.Domain, ds.Server))
		}
	}

	switch snapshot.InternetStatus {
	case state.StatusOffline:
		e.alerts.Add(SeverityCritical, "connectivity", "Internet OFFLINE")
	case state.StatusDegraded:
		e.alerts.Add(SeverityWarn, "connectivity", "Internet DEGRADED")
	}

	for _, iface := range snapshot.Interfaces {
		if !iface.Up {
			e.alerts.Add(SeverityCritical, "interface",
				formatAlert("Interface %s DOWN", iface.Name))
		}
	}
}

func formatAlert(msg string, args ...interface{}) string {
	return fmt.Sprintf(msg, args...)
}
