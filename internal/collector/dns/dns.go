package dns

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"

	"github.com/memran/netpulse/internal/logger"
	"github.com/memran/netpulse/internal/state"
)

type Collector struct {
	log      *logger.Logger
	state    *state.AppState
	interval time.Duration
	timeout  time.Duration
	servers  []string
	domains  []string
}

func NewCollector(log *logger.Logger, st *state.AppState, interval, timeout time.Duration, servers, domains []string) *Collector {
	return &Collector{
		log:      log.WithComponent("collector/dns"),
		state:    st,
		interval: interval,
		timeout:  timeout,
		servers:  servers,
		domains:  domains,
	}
}

func (c *Collector) Start(ctx context.Context) {
	c.log.Infof("starting DNS collector (interval=%s)", c.interval)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for _, server := range c.servers {
		for _, domain := range c.domains {
			c.resolveDomain(ctx, server, domain)
		}
	}

	for {
		select {
		case <-ticker.C:
			for _, server := range c.servers {
				for _, domain := range c.domains {
					c.resolveDomain(ctx, server, domain)
				}
			}
		case <-ctx.Done():
			c.log.Info("DNS collector stopped")
			return
		}
	}
}

func (c *Collector) resolveDomain(ctx context.Context, server, domain string) {
	client := &dns.Client{
		Timeout: c.timeout,
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	msg.RecursionDesired = true

	start := time.Now()
	resp, _, err := client.ExchangeContext(ctx, msg, server)
	elapsed := time.Since(start)

	if err != nil {
		c.log.Warnf("DNS lookup %s via %s failed: %v", domain, server, err)
		c.state.SetDNSStats(state.DNSStats{
			Server:       server,
			Domain:       domain,
			ResponseTime: float64(elapsed.Milliseconds()),
			Success:      false,
			Error:        err.Error(),
			UpdatedAt:    time.Now(),
		})
		return
	}

	var aAddrs []string
	for _, ans := range resp.Answer {
		if a, ok := ans.(*dns.A); ok {
			aAddrs = append(aAddrs, a.A.String())
		}
	}

	stats := state.DNSStats{
		Server:       server,
		Domain:       domain,
		ResponseTime: float64(elapsed.Milliseconds()),
		AAddresses:   aAddrs,
		Success:      resp.Rcode == dns.RcodeSuccess,
		UpdatedAt:    time.Now(),
	}

	if !stats.Success {
		stats.Error = dns.RcodeToString[resp.Rcode]
	}

	c.state.SetDNSStats(stats)
}

func (c *Collector) ResolveOnce(ctx context.Context, server, domain string) (state.DNSStats, error) {
	client := &dns.Client{
		Timeout: c.timeout,
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	msg.RecursionDesired = true

	start := time.Now()
	resp, _, err := client.ExchangeContext(ctx, msg, server)
	elapsed := time.Since(start)

	if err != nil {
		return state.DNSStats{}, err
	}

	var aAddrs []string
	for _, ans := range resp.Answer {
		if a, ok := ans.(*dns.A); ok {
			aAddrs = append(aAddrs, a.A.String())
		}
	}

	stats := state.DNSStats{
		Server:       server,
		Domain:       domain,
		ResponseTime: float64(elapsed.Milliseconds()),
		AAddresses:   aAddrs,
		Success:      resp.Rcode == dns.RcodeSuccess,
	}

	if !stats.Success {
		stats.Error = dns.RcodeToString[resp.Rcode]
	}

	return stats, nil
}

type ResolverPool struct {
	mu      sync.Mutex
	current int
	servers []string
}

func NewResolverPool(servers []string) *ResolverPool {
	return &ResolverPool{servers: servers}
}

func (p *ResolverPool) Next() string {
	if len(p.servers) == 0 {
		return "1.1.1.1:53"
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.servers[p.current]
	p.current = (p.current + 1) % len(p.servers)
	return s
}

func LookupPublicIP(ctx context.Context, servers []string) (string, error) {
	client := &dns.Client{Timeout: 5 * time.Second}

	msg := new(dns.Msg)
	msg.SetQuestion("myip.opendns.com.", dns.TypeA)
	msg.RecursionDesired = true

	candidates := []string{
		"resolver1.opendns.com:53",
		"resolver2.opendns.com:53",
	}
	candidates = append(candidates, servers...)

	for _, server := range candidates {
		resp, _, err := client.ExchangeContext(ctx, msg, server)
		if err != nil {
			continue
		}
		if resp.Rcode != dns.RcodeSuccess {
			continue
		}
		for _, ans := range resp.Answer {
			if a, ok := ans.(*dns.A); ok {
				if a.A != nil && isPublicIPv4(a.A) {
					return a.A.String(), nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not determine public IP")
}

func isPublicIPv4(ip net.IP) bool {
	if ip == nil {
		return false
	}
	ip = ip.To4()
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsMulticast()
}

var randomReader = rand.Reader

func generateRandomData(size int) ([]byte, error) {
	data := make([]byte, size)
	if _, err := randomReader.Read(data); err != nil {
		return nil, fmt.Errorf("generate random data: %w", err)
	}
	bigInt := new(big.Int)
	bigInt.SetUint64(1<<63 - 1)
	// Fill with random bytes for realistic-looking data
	// This is intentionally simple for speed test purposes
	return data, nil
}

func (c *Collector) validate() error {
	if len(c.servers) == 0 {
		return fmt.Errorf("no DNS servers configured")
	}
	if len(c.domains) == 0 {
		return fmt.Errorf("no DNS domains configured")
	}
	return nil
}
