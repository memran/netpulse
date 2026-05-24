package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/memran/netpulse/internal/alert"
	"github.com/memran/netpulse/internal/collector"
	dnsColl "github.com/memran/netpulse/internal/collector/dns"
	httpcoll "github.com/memran/netpulse/internal/collector/httpcheck"
	ifacecoll "github.com/memran/netpulse/internal/collector/netinterface"
	pingcoll "github.com/memran/netpulse/internal/collector/ping"
	speedcoll "github.com/memran/netpulse/internal/collector/speedtest"
	tracert "github.com/memran/netpulse/internal/collector/traceroute"
	"github.com/memran/netpulse/internal/config"
	"github.com/memran/netpulse/internal/logger"
	"github.com/memran/netpulse/internal/state"
	"github.com/memran/netpulse/internal/storage"
	"github.com/memran/netpulse/internal/ui"
)

type App struct {
	cfg      *config.Config
	log      *logger.Logger
	stateMgr *state.Manager
	alert    *alert.Engine
	store    *storage.Repository
	dash     *ui.Dashboard
	prog     *tea.Program

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func Run(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log, err := logger.New("", cfg.App.Debug)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer log.Close()

	stateMgr := state.NewManager()
	alerts := alert.NewEngine(50)
	alerts.Add(alert.SeverityInfo, "system", "NetPulse Community started")

	var repo *storage.Repository
	repo, err = storage.New(cfg.Storage.SQLitePath)
	if err != nil {
		log.Warnf("storage init failed (non-fatal): %v", err)
		repo = nil
	}

	app := &App{
		cfg:      cfg,
		log:      log,
		stateMgr: stateMgr,
		alert:    alerts,
		store:    repo,
	}

	ctx, cancel := context.WithCancel(context.Background())
	app.cancel = cancel

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	dashCfg := &ui.UIConfig{
		Theme:       cfg.UI.Theme,
		CompactMode: cfg.UI.CompactMode,
	}

	app.dash = ui.NewDashboard(stateMgr.Snapshot(), alerts, dashCfg)
	if repo != nil {
		app.dash.SetStore(repo)
	}

	app.prog = tea.NewProgram(
		app.dash,
		tea.WithAltScreen(),
		tea.WithFPS(10),
		tea.WithANSICompressor(),
		tea.WithoutSignalHandler(),
	)

	app.startStateManager(ctx)
	app.startCollectors(ctx)
	app.startAlertEvaluator(ctx)
	app.startStateFlusher(ctx)

	go func() {
		select {
		case <-sigCh:
			cancel()
			if app.prog != nil {
				app.prog.Quit()
			}
		case <-ctx.Done():
		}
	}()

	if _, err := app.prog.Run(); err != nil {
		log.Errorf("dashboard error: %v", err)
	}

	cancel()
	app.wg.Wait()

	if repo != nil {
		repo.Close()
	}

	return nil
}

func (a *App) startStateManager(ctx context.Context) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.stateMgr.Start(ctx)
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		for {
			select {
			case snap := <-a.stateMgr.Events():
				if a.prog != nil {
					a.prog.Send(ui.SnapshotMsg{Snapshot: snap})
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (a *App) startCollectors(ctx context.Context) {
	st := a.stateMgr.State()

	if len(a.cfg.Targets.ICMP) > 0 {
		pingColl := pingcoll.NewCollector(a.log, st,
			a.cfg.Ping.Interval,
			a.cfg.Ping.Timeout,
			a.cfg.Ping.PacketCount,
			a.cfg.Targets.ICMP,
		)
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			pingColl.Start(ctx)
		}()
	}

	ifaceCfg := a.cfg.Network.Interfaces
	if len(ifaceCfg) == 0 {
		ifaceCfg = nil
	}
	ifaceColl := ifacecoll.NewCollector(a.log, st,
		a.cfg.App.RefreshInterval*2,
		ifaceCfg,
	)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ifaceColl.Start(ctx)
	}()

	if len(a.cfg.Targets.DNS) > 0 {
		dnsColl := dnsColl.NewCollector(a.log, st,
			a.cfg.DNS.Interval,
			a.cfg.DNS.Timeout,
			a.cfg.DNS.Servers,
			a.cfg.Targets.DNS,
		)
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			dnsColl.Start(ctx)
		}()
	}

	if len(a.cfg.Targets.HTTP) > 0 {
		httpColl := httpcoll.NewCollector(a.log, st,
			a.cfg.HTTP.Interval,
			a.cfg.HTTP.Timeout,
			a.cfg.HTTP.Method,
			a.cfg.Targets.HTTP,
		)
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			httpColl.Start(ctx)
		}()
	}

	connDet := collector.NewConnectivityDetector(a.log, st)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		connDet.Start(ctx)
	}()

	speedTester := speedcoll.NewTester(a.log, st,
		a.cfg.Speed.DownloadURL,
		a.cfg.Speed.UploadURL,
		a.cfg.Speed.DownloadSizeMB,
		a.cfg.Speed.UploadSizeMB,
		a.cfg.Speed.Workers,
	)
	if a.store != nil {
		if tests, err := a.store.RecentSpeedTests(1); err == nil && len(tests) > 0 {
			st.SetSpeedTest(tests[0])
		}
		speedTester.SetOnComplete(func(res state.SpeedTestResult) {
			if err := a.store.SaveSpeedTest(res); err != nil {
				a.log.Warnf("save speed test: %v", err)
			}
		})
	}
	a.dash.SetSpeedTester(speedTester, ctx)

	tracerRunner := tracert.NewRunner(a.log, st,
		a.cfg.Traceroute.MaxHops,
		a.cfg.Traceroute.Probes,
	)
	a.dash.SetTracerouteRunner(tracerRunner, ctx)
}

func (a *App) startAlertEvaluator(ctx context.Context) {
	if a.store != nil {
		a.alert.SetOnAlert(func(al alert.Alert) {
			if err := a.store.SaveAlert(al); err != nil {
				a.log.Warnf("save alert: %v", err)
			}
		})
	}

	eval := alert.NewEvaluator(a.log, a.stateMgr.State(), a.alert,
		5.0,   // loss threshold: 5%
		200.0, // latency threshold: 200ms
		30.0,  // jitter threshold: 30ms
	)

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		eval.Start(ctx)
	}()
}

func (a *App) startStateFlusher(ctx context.Context) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if a.store == nil {
					continue
				}
				snapshot := a.stateMgr.Snapshot()

				for _, ps := range snapshot.PingStats {
					if err := a.store.SavePingSummary(ps); err != nil {
						a.log.Warnf("save ping summary: %v", err)
					}
				}

				for _, ds := range snapshot.DNSStats {
					if !ds.Success {
						if err := a.store.SaveDNSResult(ds.Server, ds.Domain, ds.ResponseTime, ds.Success, ds.Error); err != nil {
							a.log.Warnf("save dns result: %v", err)
						}
					}
				}

				if err := a.store.SaveConnectivityEvent(snapshot.InternetStatus, snapshot.PublicIP); err != nil {
					a.log.Warnf("save connectivity event: %v", err)
				}

				if err := a.store.PruneOldData(); err != nil {
					a.log.Warnf("failed to prune old database entries: %v", err)
				}

			case <-ctx.Done():
				return
			}
		}
	}()
}
