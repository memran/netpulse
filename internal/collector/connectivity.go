package collector

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/memran/netpulse/internal/logger"
	"github.com/memran/netpulse/internal/state"
)

type ConnectivityDetector struct {
	log              *logger.Logger
	state            *state.AppState
	httpClient       *http.Client
	lastPublicIPSync time.Time
}

func NewConnectivityDetector(log *logger.Logger, st *state.AppState) *ConnectivityDetector {
	return &ConnectivityDetector{
		log:   log.WithComponent("collector/connectivity"),
		state: st,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (d *ConnectivityDetector) Start(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.evaluate()
		case <-ctx.Done():
			d.log.Info("connectivity detector stopped")
			return
		default:
			time.Sleep(500 * time.Millisecond)
			d.evaluate()
			ticker.Reset(10 * time.Second)
		}
	}
}

func (d *ConnectivityDetector) evaluate() {
	snapshot := d.state.Read()

	pingOK := false
	for _, ps := range snapshot.PingStats {
		if ps.LastSuccess && ps.PacketLoss < 100 {
			pingOK = true
			break
		}
	}

	dnsOK := false
	for _, ds := range snapshot.DNSStats {
		if ds.Success {
			dnsOK = true
			break
		}
	}

	httpOK := false
	for _, hs := range snapshot.HTTPStats {
		if hs.Success {
			httpOK = true
			break
		}
	}

	var status state.ConnectivityStatus
	reason := ""

	if pingOK && (dnsOK || httpOK) {
		status = state.StatusOnline
	} else if pingOK || dnsOK || httpOK {
		status = state.StatusDegraded
		reason = "partial connectivity"
	} else {
		status = state.StatusOffline
		reason = "no connectivity"
	}

	if status != snapshot.InternetStatus {
		d.log.Infof("connectivity changed: %s -> %s (reason: %s)", snapshot.InternetStatus, status, reason)
	}

	publicIP := snapshot.PublicIP
	shouldLookupPublicIP := status != state.StatusOffline &&
		(publicIP == "" || time.Since(d.lastPublicIPSync) > 10*time.Minute)
	if shouldLookupPublicIP {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		detectedIP, err := d.lookupPublicIP(ctx)
		if err == nil && detectedIP != "" {
			publicIP = detectedIP
			d.lastPublicIPSync = time.Now()
		} else if publicIP == "" && err != nil {
			d.log.Debugf("public ip lookup failed: %v", err)
		}
	}

	d.state.SetConnectivity(status, publicIP)
}

func (d *ConnectivityDetector) lookupPublicIP(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org/", nil)
	if err != nil {
		return "", err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}

	ipText := strings.TrimSpace(string(body))
	ip := net.ParseIP(ipText)
	if ip == nil {
		return "", &net.ParseError{Type: "IP address", Text: ipText}
	}
	if !isPublicIP(ip) {
		return "", &net.ParseError{Type: "public IP address", Text: ipText}
	}
	return ipText, nil
}

func isPublicIP(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsMulticast()
}
