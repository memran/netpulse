package ping

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	gossping "github.com/go-ping/ping"

	"github.com/memran/netpulse/internal/logger"
	"github.com/memran/netpulse/internal/state"
)

type Collector struct {
	log      *logger.Logger
	st       *state.AppState
	interval time.Duration
	timeout  time.Duration
	count    int
	targets  []string
}

func NewCollector(log *logger.Logger, st *state.AppState, interval, timeout time.Duration, count int, targets []string) *Collector {
	return &Collector{
		log:      log.WithComponent("collector/ping"),
		st:       st,
		interval: interval,
		timeout:  timeout,
		count:    count,
		targets:  targets,
	}
}

func (c *Collector) Start(ctx context.Context) {
	c.log.Infof("starting ping collector (interval=%s, targets=%v)", c.interval, c.targets)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for _, target := range c.targets {
		c.pingTarget(ctx, target)
	}

	for {
		select {
		case <-ticker.C:
			for _, target := range c.targets {
				c.pingTarget(ctx, target)
			}
		case <-ctx.Done():
			c.log.Info("ping collector stopped")
			return
		}
	}
}

func (c *Collector) pingTarget(ctx context.Context, target string) {
	pinger, err := gossping.NewPinger(target)
	if err != nil {
		c.log.Warnf("failed to create pinger for %s: %v", target, err)
		c.st.SetPingStats(state.PingStats{
			Target:      target,
			LastSuccess: false,
			PacketLoss:  100,
			UpdatedAt:   time.Now(),
		})
		return
	}

	pinger.Count = c.count
	pinger.Timeout = c.timeout
	pinger.Interval = time.Second
	pinger.SetPrivileged(true)

	done := make(chan struct{})
	var stats *gossping.Statistics

	go func() {
		err = pinger.Run()
		if err != nil {
			c.log.Warnf("privileged ping for %s failed: %v; retrying in non-privileged mode...", target, err)
			p2, err2 := gossping.NewPinger(target)
			if err2 != nil {
				err = fmt.Errorf("fallback pinger creation: %w", err2)
				close(done)
				return
			}
			p2.Count = c.count
			p2.Timeout = c.timeout
			p2.Interval = time.Second
			p2.SetPrivileged(false)
			
			err = p2.Run()
			if err != nil {
				c.log.Warnf("non-privileged ping for %s failed: %v", target, err)
			} else {
				stats = p2.Statistics()
			}
		} else {
			stats = pinger.Statistics()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		pinger.Stop()
		return
	}

	if err != nil || stats == nil {
		c.log.Warnf("ping to %s failed completely: %v", target, err)
		c.st.SetPingStats(state.PingStats{
			Target:      target,
			LastSuccess: false,
			PacketLoss:  100,
			UpdatedAt:   time.Now(),
		})
		return
	}

	loss := float64(stats.PacketsSent-stats.PacketsRecv) / float64(stats.PacketsSent) * 100

	var rtts []float64
	for _, rtt := range stats.Rtts {
		rtts = append(rtts, float64(rtt.Milliseconds()))
	}

	jitter := calcJitter(rtts)

	ps := state.PingStats{
		Target:      target,
		AvgLatency:  float64(stats.AvgRtt.Milliseconds()),
		MinLatency:  float64(stats.MinRtt.Milliseconds()),
		MaxLatency:  float64(stats.MaxRtt.Milliseconds()),
		PacketLoss:  math.Round(loss*100) / 100,
		Jitter:      math.Round(jitter*100) / 100,
		Sent:        stats.PacketsSent,
		Received:    stats.PacketsRecv,
		LastSuccess: stats.PacketsRecv > 0,
		UpdatedAt:   time.Now(),
	}

	if ps.PacketLoss > 0 {
		c.log.Warnf("packet loss %.1f%% to %s", ps.PacketLoss, target)
	}

	c.st.SetPingStats(ps)
}

func calcJitter(rtts []float64) float64 {
	if len(rtts) < 2 {
		return 0
	}
	sort.Float64s(rtts)
	n := len(rtts)
	var sum float64
	for i := 1; i < n; i++ {
		diff := rtts[i] - rtts[i-1]
		if diff < 0 {
			diff = -diff
		}
		sum += diff
	}
	return sum / float64(n-1)
}

func (c *Collector) PingOnce(ctx context.Context, target string) (state.PingStats, error) {
	pinger, err := gossping.NewPinger(target)
	if err != nil {
		return state.PingStats{}, fmt.Errorf("create pinger: %w", err)
	}

	pinger.Count = c.count
	pinger.Timeout = c.timeout
	pinger.Interval = time.Second
	pinger.SetPrivileged(true)

	done := make(chan error)
	var stats *gossping.Statistics

	go func() {
		errRun := pinger.Run()
		if errRun != nil {
			c.log.Warnf("privileged PingOnce for %s failed: %v; trying non-privileged mode...", target, errRun)
			p2, err2 := gossping.NewPinger(target)
			if err2 != nil {
				done <- fmt.Errorf("fallback pinger creation: %w", err2)
				return
			}
			p2.Count = c.count
			p2.Timeout = c.timeout
			p2.Interval = time.Second
			p2.SetPrivileged(false)
			
			errRun2 := p2.Run()
			if errRun2 != nil {
				done <- fmt.Errorf("non-privileged ping failed: %w", errRun2)
				return
			}
			stats = p2.Statistics()
			done <- nil
		} else {
			stats = pinger.Statistics()
			done <- nil
		}
	}()

	select {
	case err = <-done:
	case <-ctx.Done():
		pinger.Stop()
		return state.PingStats{}, ctx.Err()
	}

	if err != nil {
		return state.PingStats{}, fmt.Errorf("ping %s: %w", target, err)
	}

	if stats == nil {
		return state.PingStats{}, fmt.Errorf("no stats returned")
	}

	loss := float64(stats.PacketsSent-stats.PacketsRecv) / float64(stats.PacketsSent) * 100

	var rtts []float64
	for _, rtt := range stats.Rtts {
		rtts = append(rtts, float64(rtt.Milliseconds()))
	}

	return state.PingStats{
		Target:      target,
		AvgLatency:  float64(stats.AvgRtt.Milliseconds()),
		MinLatency:  float64(stats.MinRtt.Milliseconds()),
		MaxLatency:  float64(stats.MaxRtt.Milliseconds()),
		PacketLoss:  math.Round(loss*100) / 100,
		Jitter:      math.Round(calcJitter(rtts)*100) / 100,
		Sent:        stats.PacketsSent,
		Received:    stats.PacketsRecv,
		LastSuccess: stats.PacketsRecv > 0,
	}, nil
}
