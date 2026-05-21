package speedtest

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/memran/netpulse/internal/logger"
	"github.com/memran/netpulse/internal/state"
)

type Tester struct {
	log          *logger.Logger
	st           *state.AppState
	downloadURL  string
	uploadURL    string
	downloadSize int64
	uploadSize   int64
	workers      int
	client       *http.Client
	onComplete   func(state.SpeedTestResult)
}

func NewTester(log *logger.Logger, st *state.AppState, downloadURL, uploadURL string, downloadSizeMB, uploadSizeMB int64, workers int) *Tester {
	return &Tester{
		log:          log.WithComponent("collector/speedtest"),
		st:           st,
		downloadURL:  downloadURL,
		uploadURL:    uploadURL,
		downloadSize: downloadSizeMB * 1_000_000,
		uploadSize:   uploadSizeMB * 1_000_000,
		workers:      workers,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *Tester) SetOnComplete(fn func(state.SpeedTestResult)) {
	t.onComplete = fn
}

func (t *Tester) Run(ctx context.Context) {
	t.st.SetSpeedTest(state.SpeedTestResult{Running: true})
	t.log.Info("speed test started")

	var (
		downloadMbps float64
		uploadMbps   float64
		mu           sync.Mutex
		wg           sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		dl, err := t.runDownload(ctx)
		mu.Lock()
		if err == nil {
			downloadMbps = dl
		} else {
			t.log.Warnf("download test: %v", err)
		}
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ul, err := t.runUpload(ctx)
		mu.Lock()
		if err == nil {
			uploadMbps = ul
		} else {
			t.log.Warnf("upload test: %v", err)
		}
		mu.Unlock()
	}()

	wg.Wait()

	mu.Lock()
	result := state.SpeedTestResult{
		DownloadMbps: round2(downloadMbps),
		UploadMbps:   round2(uploadMbps),
		Running:      false,
		CompletedAt:  time.Now(),
	}
	if result.DownloadMbps == 0 && result.UploadMbps == 0 {
		result.Error = "speed test failed"
	}
	mu.Unlock()

	t.st.SetSpeedTest(result)
	t.log.Infof("speed test: %.1f down / %.1f up Mbps", result.DownloadMbps, result.UploadMbps)
	
	if t.onComplete != nil {
		go t.onComplete(result)
	}
}

func (t *Tester) runDownload(ctx context.Context) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	start := time.Now()
	var total int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, t.workers)
	done := make(chan struct{})

	for w := 0; w < t.workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req, err := http.NewRequestWithContext(ctx, "GET", t.downloadURL, nil)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			req.Header.Set("Accept", "*/*")

			q := req.URL.Query()
			q.Set("bytes", fmt.Sprintf("%d", t.downloadSize/int64(t.workers)))
			req.URL.RawQuery = q.Encode()

			resp, err := t.client.Do(req)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				select {
				case errCh <- fmt.Errorf("bad status: %d", resp.StatusCode):
				default:
				}
				return
			}

			n, err := io.Copy(io.Discard, resp.Body)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}

			mu.Lock()
			total += n
			mu.Unlock()
		}()
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case err := <-errCh:
		return 0, err
	case <-done:
	}

	elapsed := time.Since(start).Seconds()
	if elapsed <= 0 {
		return 0, fmt.Errorf("no time elapsed")
	}

	bps := float64(total) * 8 / elapsed
	return bps / 1_000_000, nil
}

func (t *Tester) runUpload(ctx context.Context) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	buf := make([]byte, t.uploadSize)
	_, err := rand.Read(buf)
	if err != nil {
		return 0, fmt.Errorf("generate data: %w", err)
	}

	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "POST", t.uploadURL, &lazyReader{data: buf})
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.ContentLength = t.uploadSize

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("bad upload status: %d", resp.StatusCode)
	}

	io.Copy(io.Discard, resp.Body)

	elapsed := time.Since(start).Seconds()
	if elapsed <= 0 {
		return 0, fmt.Errorf("no time elapsed")
	}

	bps := float64(t.uploadSize) * 8 / elapsed
	return bps / 1_000_000, nil
}

type lazyReader struct {
	data   []byte
	offset int
}

func (r *lazyReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func round2(f float64) float64 {
	return float64(int(f*100)) / 100
}
