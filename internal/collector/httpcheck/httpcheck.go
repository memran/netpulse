package httpcheck

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/memran/netpulse/internal/logger"
	"github.com/memran/netpulse/internal/state"
)

type Collector struct {
	log      *logger.Logger
	state    *state.AppState
	interval time.Duration
	timeout  time.Duration
	method   string
	targets  []string
	client   *http.Client
}

func NewCollector(log *logger.Logger, st *state.AppState, interval, timeout time.Duration, method string, targets []string) *Collector {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 0,
		}).DialContext,
		DisableKeepAlives: true,
	}

	return &Collector{
		log:      log.WithComponent("collector/http"),
		state:    st,
		interval: interval,
		timeout:  timeout,
		method:   method,
		targets:  targets,
		client: &http.Client{
			Transport: tr,
			Timeout:   timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Collector) Start(ctx context.Context) {
	c.log.Infof("starting HTTP collector (interval=%s)", c.interval)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for _, target := range c.targets {
		c.checkTarget(ctx, target)
	}

	for {
		select {
		case <-ticker.C:
			for _, target := range c.targets {
				c.checkTarget(ctx, target)
			}
		case <-ctx.Done():
			c.log.Info("HTTP collector stopped")
			return
		}
	}
}

func (c *Collector) checkTarget(ctx context.Context, target string) {
	req, err := http.NewRequestWithContext(ctx, c.method, target, nil)
	if err != nil {
		c.log.Warnf("failed to create request for %s: %v", target, err)
		c.state.SetHTTPStats(state.HTTPStats{
			URL:     target,
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	elapsed := time.Since(start)

	stats := state.HTTPStats{
		URL:          target,
		ResponseTime: float64(elapsed.Milliseconds()),
		UpdatedAt:    time.Now(),
	}

	if err != nil {
		stats.Success = false
		stats.Error = err.Error()
	} else {
		defer resp.Body.Close()
		stats.StatusCode = resp.StatusCode
		stats.Success = resp.StatusCode >= 200 && resp.StatusCode < 500
		if resp.TLS != nil {
			stats.TLSAvailable = true
		}
	}

	c.state.SetHTTPStats(stats)
}

func (c *Collector) CheckOnce(ctx context.Context, target string) (state.HTTPStats, error) {
	req, err := http.NewRequestWithContext(ctx, c.method, target, nil)
	if err != nil {
		return state.HTTPStats{}, err
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	elapsed := time.Since(start)

	stats := state.HTTPStats{
		URL:          target,
		ResponseTime: float64(elapsed.Milliseconds()),
	}

	if err != nil {
		stats.Success = false
		stats.Error = err.Error()
		return stats, err
	}
	defer resp.Body.Close()

	stats.StatusCode = resp.StatusCode
	stats.Success = resp.StatusCode >= 200 && resp.StatusCode < 500
	if resp.TLS != nil {
		stats.TLSAvailable = true
	}

	return stats, nil
}
