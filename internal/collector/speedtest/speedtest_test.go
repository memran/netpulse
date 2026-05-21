package speedtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/memran/netpulse/internal/logger"
	"github.com/memran/netpulse/internal/state"
)

func TestSpeedTester(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			bytesStr := r.URL.Query().Get("bytes")
			bytes, err := strconv.ParseInt(bytesStr, 10, 64)
			if err != nil {
				http.Error(w, "invalid bytes parameter", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Length", strconv.FormatInt(bytes, 10))
			w.WriteHeader(http.StatusOK)
			data := make([]byte, 1024)
			var written int64
			for written < bytes {
				toWrite := bytes - written
				if toWrite > 1024 {
					toWrite = 1024
				}
				n, _ := w.Write(data[:toWrite])
				written += int64(n)
			}
		} else if r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	// Initialize logs & state
	log, err := logger.New("", false)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	st := state.New()

	tester := NewTester(
		log,
		st,
		server.URL, // download URL
		server.URL, // upload URL
		1,          // 1 MB download
		1,          // 1 MB upload
		2,          // 2 workers
	)

	// Run speed test
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tester.Run(ctx)

	// Verify state results
	res := st.Read().SpeedTest
	if res.Running {
		t.Error("expected speedtest to not be running after completion")
	}
	if res.Error != "" {
		t.Errorf("expected no error, got: %s", res.Error)
	}
	if res.DownloadMbps <= 0 {
		t.Errorf("expected download mbps > 0, got %f", res.DownloadMbps)
	}
	if res.UploadMbps <= 0 {
		t.Errorf("expected upload mbps > 0, got %f", res.UploadMbps)
	}
}
