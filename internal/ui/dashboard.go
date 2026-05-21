package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/memran/netpulse/internal/alert"
	"github.com/memran/netpulse/internal/collector/speedtest"
	"github.com/memran/netpulse/internal/state"
	"github.com/memran/netpulse/internal/storage"
)

type UIConfig struct {
	Theme       string
	CompactMode bool
}

type Dashboard struct {
	snapshot          state.AppStateSnapshot
	alertEng          *alert.Engine
	config            *UIConfig
	speedTester       *speedtest.Tester
	store             *storage.Repository
	ctx               context.Context
	width             int
	height            int
	ready             bool
	quitting          bool
	startTime         time.Time
	alertScrollOffset int
	history           []storage.HistoryDaySummary
	historyLastUpdate time.Time
	latencyRing       []float64
	lossRing          []float64
	jitterRing        []float64
	rxRing            []float64
	txRing            []float64
	speedDownRing     []float64
	speedUpRing       []float64
	lastIfaceName     string
	lastIfaceUpdate   time.Time
	lastSpeedTestAt   time.Time
	localPanelCache   string
	localPanelKey     string
}

var (
	bg          = lipgloss.Color("#0B0F14")
	bd          = lipgloss.Color("#374151")
	hdr         = lipgloss.Color("#00D4FF")
	txt         = lipgloss.Color("#F8FAFC")
	sec         = lipgloss.Color("#9CA3AF")
	green       = lipgloss.Color("#32D74B")
	red         = lipgloss.Color("#FF453A")
	yellow      = lipgloss.Color("#FFD60A")
	cyan        = lipgloss.Color("#22D3EE")
	blue        = lipgloss.Color("#38BDF8")
	purple      = lipgloss.Color("#A855F7")
	baseStyle   = lipgloss.NewStyle().Background(bg).Foreground(txt)
	bdStyle     = lipgloss.NewStyle().Foreground(bd)
	hdrStyle    = lipgloss.NewStyle().Foreground(hdr).Bold(true)
	lblStyle    = lipgloss.NewStyle().Foreground(sec)
	valStyle    = lipgloss.NewStyle().Foreground(txt)
	greenStyle  = lipgloss.NewStyle().Foreground(green).Bold(true)
	redStyle    = lipgloss.NewStyle().Foreground(red).Bold(true)
	yellowStyle = lipgloss.NewStyle().Foreground(yellow).Bold(true)
	cyanStyle   = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	blueStyle   = lipgloss.NewStyle().Foreground(blue).Bold(true)
	purpleStyle = lipgloss.NewStyle().Foreground(purple).Bold(true)
	mutedStyle  = lipgloss.NewStyle().Foreground(sec)
)

func NewDashboard(snapshot state.AppStateSnapshot, eng *alert.Engine, cfg *UIConfig) *Dashboard {
	return &Dashboard{
		snapshot:      snapshot,
		alertEng:      eng,
		config:        cfg,
		startTime:     time.Now(),
		latencyRing:   make([]float64, 0, 60),
		lossRing:      make([]float64, 0, 60),
		jitterRing:    make([]float64, 0, 60),
		rxRing:        make([]float64, 0, 24),
		txRing:        make([]float64, 0, 24),
		speedDownRing: make([]float64, 0, 24),
		speedUpRing:   make([]float64, 0, 24),
	}
}

func (d *Dashboard) SetSpeedTester(t *speedtest.Tester, ctx context.Context) {
	d.speedTester = t
	d.ctx = ctx
}

func (d *Dashboard) SetStore(store *storage.Repository) {
	d.store = store
}

func (d *Dashboard) Init() tea.Cmd { return tickClock() }

type SnapshotMsg struct {
	Snapshot state.AppStateSnapshot
}

type ClockTickMsg time.Time

func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		d.ready = true
		return d, nil
	case ClockTickMsg:
		return d, tickClock()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			d.quitting = true
			return d, tea.Quit
		case "s", "S":
			if d.speedTester != nil && d.ctx != nil {
				go d.speedTester.Run(d.ctx)
			}
		case "c":
			d.alertEng.Clear()
			d.alertScrollOffset = 0
		case "up":
			if d.alertScrollOffset > 0 {
				d.alertScrollOffset--
			}
		case "down":
			d.alertScrollOffset++
		case "h", "r":
			d.loadHistory()
		}
	case SnapshotMsg:
		d.snapshot = msg.Snapshot
		snap := msg.Snapshot
		avgL, avgLoss, avgJit := aggPing(snap)
		d.latencyRing = appendRing(d.latencyRing, avgL, 60)
		d.lossRing = appendRing(d.lossRing, avgLoss, 60)
		d.jitterRing = appendRing(d.jitterRing, avgJit, 60)
		if iface, ok := d.getActiveInterface(snap); ok {
			if iface.Name != d.lastIfaceName || iface.UpdatedAt.After(d.lastIfaceUpdate) {
				d.rxRing = appendRing(d.rxRing, iface.RXSpeed, 24)
				d.txRing = appendRing(d.txRing, iface.TXSpeed, 24)
				d.lastIfaceName = iface.Name
				d.lastIfaceUpdate = iface.UpdatedAt
			}
		}
		if !snap.SpeedTest.CompletedAt.IsZero() && snap.SpeedTest.CompletedAt.After(d.lastSpeedTestAt) {
			d.speedDownRing = appendRing(d.speedDownRing, snap.SpeedTest.DownloadMbps, 24)
			d.speedUpRing = appendRing(d.speedUpRing, snap.SpeedTest.UploadMbps, 24)
			d.lastSpeedTestAt = snap.SpeedTest.CompletedAt
		}
		if d.store != nil && time.Since(d.historyLastUpdate) > 5*time.Minute {
			d.loadHistory()
		}
		return d, nil
	}
	return d, nil
}

func (d *Dashboard) View() string {
	if !d.ready {
		return baseStyle.Render("Initializing NetPulse UI...")
	}
	if d.quitting {
		return ""
	}
	if d.width < 120 || d.height < 30 {
		return baseStyle.Render(fmt.Sprintf("Terminal too small: %dx%d (min 120x30)", d.width, d.height))
	}

	snap := d.snapshot
	marginX := maxInt(1, d.width/100)
	marginY := maxInt(0, d.height/100)
	contentW := d.width - marginX*2
	if contentW < 110 {
		contentW = 110
	}

	header := frameBox(d.renderHeaderBar(snap, contentW-2), contentW-2)
	cards := d.renderSummaryCards(snap, contentW)

	bodyAvailable := d.height - marginY*2 - lineCount(header) - lineCount(cards) - 6
	if bodyAvailable < 18 {
		bodyAvailable = 18
	}
	body := d.renderBody(snap, contentW, bodyAvailable)
	footer := frameBox(d.renderFooterBar(contentW-4), contentW-2)
	note := frameBox(d.renderFooterNote(contentW-4), contentW-2)

	content := strings.Join([]string{header, cards, body, footer, note}, "\n")
	return baseStyle.Render(normalizeViewport(applyOuterMargin(content, marginX, marginY), d.width, d.height))
}

func tickClock() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return ClockTickMsg(t)
	})
}

func (d *Dashboard) renderHeaderBar(snap state.AppStateSnapshot, w int) string {
	left := cyanStyle.Render("NetPulse TUI") + valStyle.Render(" v1.0.0 ") + yellowStyle.Render("(FREE)")
	status := greenStyle.Render("* ") + lblStyle.Render("Internet: ") + d.statusStyle(snap.InternetStatus).Render(snap.InternetStatus.String())
	right := lblStyle.Render("Uptime: ") + valStyle.Render(formatUptimeLong(time.Since(d.startTime))) + "    " +
		lblStyle.Render("Time: ") + valStyle.Render(time.Now().Format("15:04:05"))
	return layoutThree(left, status, right, w)
}

func (d *Dashboard) renderSummaryCards(snap state.AppStateSnapshot, w int) string {
	avgL, avgLoss, avgJit := aggPing(snap)
	dnsPct, dnsLabel, dnsStyle := d.dnsHealth(snap)
	download := formatSpeed(snap.SpeedTest.DownloadMbps)
	upload := formatSpeed(snap.SpeedTest.UploadMbps)
	cardW := splitWidths(w, 7, 1)

	cards := []string{
		card("STATUS", "*", d.statusStyle(snap.InternetStatus), snap.InternetStatus.String(), d.statusStyle(snap.InternetStatus), "Since "+formatSinceShort(time.Since(d.startTime)), cardW[0]),
		card("LATENCY (AVG)", "~", cyanStyle, fmt.Sprintf("%.1f ms", avgL), greenStyle, "All Targets", cardW[1]),
		card("PACKET LOSS (AVG)", "o", valStyle, fmt.Sprintf("%.1f%%", avgLoss), metricStyle(avgLoss, 1, 5), "All Targets", cardW[2]),
		card("JITTER (AVG)", "<>", cyanStyle, fmt.Sprintf("%.1f ms", avgJit), metricStyle(avgJit, 10, 30), "All Targets", cardW[3]),
		card("DNS HEALTH", "+", dnsStyle, fmt.Sprintf("%.0f%%", dnsPct), dnsStyle, dnsLabel, cardW[4]),
		card("DOWNLOAD", "v", blueStyle, download, blueStyle, "Last Test", cardW[5]),
		card("UPLOAD", "^", purpleStyle, upload, purpleStyle, "Last Test", cardW[6]),
	}

	return joinHorizontal(cards, cardW, 6)
}

func (d *Dashboard) renderBody(snap state.AppStateSnapshot, w, h int) string {
	colW := splitWidths(w, 3, 1)

	leftTop := maxInt(12, h*45/100)
	leftBottom := h - leftTop - 1
	centerTop := maxInt(16, h*68/100)
	centerBottom := h - centerTop - 1
	rightTop := maxInt(12, h*53/100)
	rightBottom := h - rightTop - 1

	left := joinVertical([]string{
		d.renderConnectivityChecks(snap, colW[0]-2, leftTop),
		d.renderLocalInterface(snap, colW[0]-2, leftBottom),
	}, []int{leftTop, leftBottom})
	center := joinVertical([]string{
		d.renderGraphs(colW[1]-2, centerTop),
		d.renderHistory(colW[1]-2, centerBottom),
	}, []int{centerTop, centerBottom})
	right := joinVertical([]string{
		d.renderSpeedTest(snap, colW[2]-2, rightTop),
		d.renderAlerts(colW[2]-2, rightBottom),
	}, []int{rightTop, rightBottom})

	return joinHorizontal([]string{left, center, right}, colW, h)
}

func (d *Dashboard) renderConnectivityChecks(snap state.AppStateSnapshot, w, h int) string {
	type row struct {
		target  string
		kind    string
		latency string
		loss    string
		ok      bool
	}
	var rows []row
	for _, ds := range snap.DNSStats {
		rows = append(rows, row{
			target:  ds.Domain,
			kind:    "DNS",
			latency: fmt.Sprintf("%.1f ms", ds.ResponseTime),
			loss:    "0%",
			ok:      ds.Success,
		})
	}
	for _, hs := range snap.HTTPStats {
		rows = append(rows, row{
			target:  simplifyURL(hs.URL),
			kind:    "HTTP",
			latency: fmt.Sprintf("%.1f ms", hs.ResponseTime),
			loss:    "0%",
			ok:      hs.Success,
		})
	}
	for _, ps := range snap.PingStats {
		rows = append(rows, row{
			target:  ps.Target,
			kind:    "PING",
			latency: fmt.Sprintf("%.1f ms", ps.AvgLatency),
			loss:    fmt.Sprintf("%.0f%%", ps.PacketLoss),
			ok:      ps.LastSuccess && ps.Sent > 0,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].target < rows[j].target })

	lines := []string{
		panelTitle("CONNECTIVITY CHECKS", "", w),
		bdStyle.Render(strings.Repeat("-", w)),
		lblStyle.Render(padRight("Target", maxInt(12, w-34))) + " " +
			lblStyle.Render(padRight("Type", 6)) + " " +
			lblStyle.Render(padRight("Status", 7)) + " " +
			lblStyle.Render(padRight("Latency", 10)) + " " +
			lblStyle.Render("Loss"),
	}
	healthy := 0
	maxRows := maxInt(1, h-6)
	for i, row := range rows {
		if i >= maxRows {
			break
		}
		if row.ok {
			healthy++
		}
		status := redStyle.Render("o")
		if row.ok {
			status = greenStyle.Render("*")
		}
		lines = append(lines,
			valStyle.Render(padRight(truncateText(row.target, maxInt(12, w-34)), maxInt(12, w-34)))+" "+
				lblStyle.Render(padRight(row.kind, 6))+" "+
				status+"      "+
				valStyle.Render(padRight(row.latency, 10))+" "+
				metricStyle(parsePercent(row.loss), 1, 5).Render(row.loss),
		)
	}
	if len(rows) == 0 {
		lines = append(lines, mutedStyle.Render("No checks yet"))
	}
	lines = append(lines, "")
	lines = append(lines, splitLine(
		lblStyle.Render(fmt.Sprintf("Summary: %d/%d up", healthy, len(rows))),
		greenStyle.Render("All targets are reachable"),
		w,
	))
	return frameBox(strings.Join(forceHeight(lines, h-2), "\n"), w)
}

func (d *Dashboard) renderLocalInterface(snap state.AppStateSnapshot, w, h int) string {
	iface, ok := d.getActiveInterface(snap)
	cacheKey := fmt.Sprintf("%d|%d|%t|%s|%s", w, h, ok, snap.PublicIP, d.interfaceCacheStamp(iface, ok))
	if d.localPanelCache != "" && d.localPanelKey == cacheKey {
		return d.localPanelCache
	}
	title := panelTitle("LOCAL INTERFACE", "(none)", w)
	if ok {
		title = panelTitle("LOCAL INTERFACE", "("+iface.Name+")", w)
	}
	lines := []string{
		title,
		bdStyle.Render(strings.Repeat("-", w)),
	}
	if !ok {
		lines = append(lines, mutedStyle.Render("No active interface"))
		panel := frameBox(strings.Join(forceHeight(lines, h-2), "\n"), w)
		d.localPanelKey = cacheKey
		d.localPanelCache = panel
		return panel
	}

	lines = append(lines,
		splitLine(lblStyle.Render("Status:"), d.statusStyleBool(iface.Up).Render(boolText(iface.Up)), w),
		"",
		infoRow("IP Address", fallback(iface.IPAddress, "---"), w),
		infoRow("Public IP", fallback(snap.PublicIP, "---"), w),
		infoRow("MAC Address", fallback(iface.MACAddress, "---"), w),
		infoRow("Gateway", fallback(iface.Gateway, "---"), w),
		infoRow("MTU", fmt.Sprintf("%d", iface.MTU), w),
	)
	rxW := (w - 1) / 2
	txW := w - rxW - 1
	lines = append(lines, joinHorizontal([]string{
		miniMetric("Download (RX)", formatSpeed(iface.RXSpeed), blueStyle, d.rxRing, rxW),
		miniMetric("Upload (TX)", formatSpeed(iface.TXSpeed), purpleStyle, d.txRing, txW),
	}, []int{rxW, txW}, 6))
	lines = append(lines,
		infoRow("Total RX", humanBytes(iface.TotalRX), w),
		infoRow("Total TX", humanBytes(iface.TotalTX), w),
		infoRow("Packets In", humanCount(iface.PacketsIn), w),
		infoRow("Packets Out", humanCount(iface.PacketsOut), w),
		infoRow("Errors", fmt.Sprintf("%d", iface.Errors), w),
		infoRow("Drops", fmt.Sprintf("%d", iface.Drops), w),
	)
	panel := frameBox(strings.Join(forceHeight(lines, h-2), "\n"), w)
	d.localPanelKey = cacheKey
	d.localPanelCache = panel
	return panel
}

func (d *Dashboard) renderGraphs(w, h int) string {
	graphHeights := []int{maxInt(5, (h-6)/3), maxInt(5, (h-6)/3), 0}
	graphHeights[2] = h - 4 - graphHeights[0] - graphHeights[1]
	lines := []string{
		panelTitle("REAL-TIME GRAPHS", "(Last 60 Seconds)", w),
		bdStyle.Render(strings.Repeat("-", w)),
	}
	lines = append(lines, d.renderSeries("Latency (ms)", d.latencyRing, greenStyle, fmt.Sprintf("Avg: %.1f ms", average(d.latencyRing)), w, graphHeights[0])...)
	lines = append(lines, d.renderSeries("Packet Loss (%)", d.lossRing, redStyle, fmt.Sprintf("Avg: %.1f %%", average(d.lossRing)), w, graphHeights[1])...)
	lines = append(lines, d.renderSeries("Jitter (ms)", d.jitterRing, yellowStyle, fmt.Sprintf("Avg: %.1f ms", average(d.jitterRing)), w, graphHeights[2])...)
	return frameBox(strings.Join(forceHeight(lines, h-2), "\n"), w)
}

func (d *Dashboard) renderHistory(w, h int) string {
	lines := []string{
		panelTitle("HISTORY SUMMARY", "(Last 7 Days)", w),
		bdStyle.Render(strings.Repeat("-", w)),
	}
	if len(d.history) == 0 {
		lines = append(lines, mutedStyle.Render("No history available yet"))
		return frameBox(strings.Join(forceHeight(lines, h-2), "\n"), w)
	}
	lines = append(lines,
		lblStyle.Render(padRight("Date", 11))+" "+
			lblStyle.Render(padRight("Uptime", 16))+" "+
			lblStyle.Render(padRight("Avg Latency", 12))+" "+
			lblStyle.Render(padRight("Packet Loss", 11))+" "+
			lblStyle.Render("Downtime"),
	)
	maxRows := maxInt(1, h-5)
	for i, row := range d.history {
		if i >= maxRows {
			break
		}
		lines = append(lines,
			valStyle.Render(padRight(row.Date, 11))+" "+
				valStyle.Render(padRight(truncateText(row.UptimeStr, 16), 16))+" "+
				valStyle.Render(padRight(fmt.Sprintf("%.1f ms", row.AvgLatency), 12))+" "+
				metricStyle(row.PacketLoss, 1, 5).Render(padRight(fmt.Sprintf("%.1f %%", row.PacketLoss), 11))+" "+
				valStyle.Render(row.DowntimeStr),
		)
	}
	return frameBox(strings.Join(forceHeight(lines, h-2), "\n"), w)
}

func (d *Dashboard) renderSpeedTest(snap state.AppStateSnapshot, w, h int) string {
	lines := []string{
		panelTitle("SPEED TEST", "(Manual)", w),
		bdStyle.Render(strings.Repeat("-", w)),
	}
	leftW := (w - 1) / 2
	rightW := w - leftW - 1
	lines = append(lines, joinHorizontal([]string{
		miniMetric("DOWNLOAD", speedPanelValue(snap.SpeedTest.Running, snap.SpeedTest.DownloadMbps), blueStyle, d.speedDownRing, leftW),
		miniMetric("UPLOAD", speedPanelValue(snap.SpeedTest.Running, snap.SpeedTest.UploadMbps), purpleStyle, d.speedUpRing, rightW),
	}, []int{leftW, rightW}, 8))

	timeText := "---"
	if !snap.SpeedTest.CompletedAt.IsZero() {
		timeText = snap.SpeedTest.CompletedAt.Local().Format("2006-01-02 15:04:05")
	}
	statusText := "Idle"
	if snap.SpeedTest.Running {
		statusText = "Running"
	} else if snap.SpeedTest.Error != "" {
		statusText = "Failed"
	} else if !snap.SpeedTest.CompletedAt.IsZero() {
		statusText = "Completed"
	}
	lines = append(lines,
		"",
		infoRow("Status", statusText, w),
		infoRow("Time", timeText, w),
		infoRow("Protocol", "Multi-Thread", w),
		infoRow("Ping", speedLatencyText(snap), w),
		infoRow("Jitter", fmt.Sprintf("%.1f ms", average(d.jitterRing)), w),
		infoRow("Packet Loss", fmt.Sprintf("%.1f %%", average(d.lossRing)), w),
		infoRow("Error", fallback(snap.SpeedTest.Error, "---"), w),
		"",
		centerText(greenStyle.Render("[S]")+" "+valStyle.Render(speedActionText(snap.SpeedTest.Running)), w),
	)
	return frameBox(strings.Join(forceHeight(lines, h-2), "\n"), w)
}

func (d *Dashboard) renderAlerts(w, h int) string {
	lines := []string{
		panelTitle("ALERTS", "", w),
		redStyle.Render(strings.Repeat("-", w)),
	}
	alerts := d.alertEng.Recent()
	if len(alerts) == 0 {
		lines = append(lines,
			"",
			"",
			centerText(greenStyle.Render("*"), w),
			"",
			centerText(valStyle.Render("No active alerts"), w),
			centerText(lblStyle.Render("Everything looks good!"), w),
			"",
			"",
			"",
			bdStyle.Render(strings.Repeat("-", w)),
			centerText(greenStyle.Render("[H]")+" "+cyanStyle.Render("View History"), w),
		)
		return frameBox(strings.Join(forceHeight(lines, h-2), "\n"), w)
	}
	maxRows := maxInt(1, h-5)
	start := minInt(d.alertScrollOffset, maxInt(0, len(alerts)-maxRows))
	end := minInt(len(alerts), start+maxRows)
	for _, a := range alerts[start:end] {
		prefix := blueStyle.Render("INFO")
		switch a.Severity {
		case alert.SeverityWarn:
			prefix = yellowStyle.Render("WARN")
		case alert.SeverityCritical:
			prefix = redStyle.Render("CRIT")
		}
		lines = append(lines, prefix+" "+valStyle.Render(truncateText(a.Message, w-5)))
	}
	return frameBox(strings.Join(forceHeight(lines, h-2), "\n"), w)
}

func (d *Dashboard) renderFooterBar(w int) string {
	leftSets := [][]string{
		{
			greenStyle.Render("[Q]") + valStyle.Render(" Quit"),
			greenStyle.Render("[R]") + valStyle.Render(" Refresh"),
			greenStyle.Render("[S]") + valStyle.Render(" Speed Test"),
			greenStyle.Render("[H]") + valStyle.Render(" History"),
			greenStyle.Render("[C]") + valStyle.Render(" Clear"),
			greenStyle.Render("[?]") + valStyle.Render(" Help"),
		},
		{
			greenStyle.Render("[Q]"),
			greenStyle.Render("[R]"),
			greenStyle.Render("[S]"),
			greenStyle.Render("[H]"),
			greenStyle.Render("[C]"),
			greenStyle.Render("[?]"),
		},
	}
	right := greenStyle.Render("FREE VERSION (1 DEVICE)")
	for _, set := range leftSets {
		for _, sep := range []string{"      ", "   ", " "} {
			left := strings.Join(set, sep)
			if visibleWidth(left)+1+visibleWidth(right) <= w {
				return " " + padVisual(splitLine(left, right, w), w) + " "
			}
		}
	}
	return " " + padVisual(right, w) + " "
}

func (d *Dashboard) renderFooterNote(w int) string {
	lines := []string{
		cyanStyle.Render("Note: ") +
			valStyle.Render("You are using the FREE version. Only ") +
			yellowStyle.Render("1 device (this machine)") +
			valStyle.Render(" can be monitored."),
		cyanStyle.Render("Help: ") +
			valStyle.Render("Press ") +
			greenStyle.Render("[?]") +
			valStyle.Render(" for shortcuts. ") +
			valStyle.Render("Developed By Mohammad Emran <memran.dhk@gmail.com>"),
	}
	inner := strings.Join(lines, "\n")
	return " " + padVisual(inner, w) + " "
}

func (d *Dashboard) renderSeries(title string, data []float64, style lipgloss.Style, stat string, w, h int) []string {
	if h < 5 {
		h = 5
	}
	lines := []string{splitLine(style.Render(title), style.Render(stat), w)}
	graphW := maxInt(10, w-6)
	plot := sample(data, graphW)
	maxVal := maxFloat(1, maxSlice(plot))
	if title != "Packet Loss (%)" && maxVal < 10 {
		maxVal = 10
	}
	if title == "Packet Loss (%)" && maxVal < 5 {
		maxVal = 5
	}
	gridH := h - 3
	grid := make([][]rune, gridH)
	for y := 0; y < gridH; y++ {
		grid[y] = make([]rune, graphW)
		for x := 0; x < graphW; x++ {
			grid[y][x] = ' '
		}
	}

	for _, guide := range []int{0, gridH / 2, gridH - 1} {
		if guide >= 0 && guide < gridH {
			for x := 0; x < graphW; x++ {
				grid[guide][x] = '-'
			}
		}
	}

	points := make([]int, len(plot))
	for i, v := range plot {
		filled := 0
		if maxVal > 0 {
			filled = int((v / maxVal) * float64(maxInt(1, gridH-1)))
		}
		if filled < 0 {
			filled = 0
		}
		if filled > gridH-1 {
			filled = gridH - 1
		}
		points[i] = gridH - 1 - filled
	}

	for x := 0; x < len(points)-1; x++ {
		drawLine(grid, x, points[x], x+1, points[x+1])
	}
	for x, y := range points {
		grid[y][x] = '*'
	}

	for y := gridH - 1; y >= 0; y-- {
		prefix := "    "
		if y == gridH-1 {
			prefix = fmt.Sprintf("%3.0f ", maxVal)
		} else if y == 0 {
			prefix = "  0 "
		}
		var row strings.Builder
		row.WriteString(lblStyle.Render(prefix))
		row.WriteString(bdStyle.Render("|"))
		for x := 0; x < graphW; x++ {
			switch grid[y][x] {
			case '*', '|', '-', '/', '\\', '+':
				row.WriteString(style.Render(string(grid[y][x])))
			default:
				row.WriteByte(' ')
			}
		}
		lines = append(lines, row.String())
	}
	lines = append(lines, splitLine(lblStyle.Render("-60s"), lblStyle.Render("Now"), w))
	return lines
}

func drawLine(grid [][]rune, x0, y0, x1, y1 int) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}
	if x0 == x1 {
		drawVertical(grid, x0, y0, y1)
		return
	}

	dx := x1 - x0
	dy := y1 - y0
	steps := absInt(dx)
	if steps == 0 {
		return
	}

	for step := 0; step <= steps; step++ {
		x := x0 + step*signInt(dx)
		t := float64(step) / float64(steps)
		y := y0 + int(t*float64(dy))
		ch := '-'
		switch {
		case dy < 0:
			ch = '/'
		case dy > 0:
			ch = '\\'
		default:
			ch = '-'
		}
		plotRune(grid, x, y, ch)
	}
}

func drawVertical(grid [][]rune, x, y0, y1 int) {
	start := minInt(y0, y1)
	end := maxInt(y0, y1)
	for y := start; y <= end; y++ {
		plotRune(grid, x, y, '|')
	}
}

func plotRune(grid [][]rune, x, y int, ch rune) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[0]) {
		return
	}
	current := grid[y][x]
	if current == '*' {
		return
	}
	if current == '-' || current == ' ' {
		grid[y][x] = ch
		return
	}
	if current != ch {
		grid[y][x] = '+'
	}
}

func card(title, icon string, iconStyle lipgloss.Style, value string, valueStyle lipgloss.Style, subtitle string, w int) string {
	lines := []string{
		centerText(lblStyle.Render(title), w-2),
		"",
		centerText(iconStyle.Render(icon)+" "+valueStyle.Render(value), w-2),
		"",
		centerText(lblStyle.Render(subtitle), w-2),
	}
	return frameBox(strings.Join(forceHeight(lines, 4), "\n"), w-2)
}

func miniMetric(title, value string, style lipgloss.Style, data []float64, w int) string {
	lines := []string{
		centerText(style.Render(title), w-2),
		"",
		centerText(style.Render(value), w-2),
		"",
		centerText(style.Render(sparkline(sample(data, maxInt(6, w-6)))), w-2),
	}
	return frameBox(strings.Join(forceHeight(lines, 6), "\n"), w-2)
}

func panelTitle(title, suffix string, w int) string {
	if suffix == "" {
		return hdrStyle.Render(title)
	}
	return splitLine(hdrStyle.Render(title), lblStyle.Render(suffix), w)
}

func infoRow(label, value string, w int) string {
	return splitLine(lblStyle.Render(label), valStyle.Render(value), w)
}

func (d *Dashboard) getActiveInterface(snap state.AppStateSnapshot) (state.InterfaceStats, bool) {
	var candidates []state.InterfaceStats
	for _, iface := range snap.Interfaces {
		if iface.Up {
			candidates = append(candidates, iface)
		}
	}
	if len(candidates) == 0 {
		return state.InterfaceStats{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		a := candidates[i].TotalRX + candidates[i].TotalTX
		b := candidates[j].TotalRX + candidates[j].TotalTX
		if a == b {
			return candidates[i].Name < candidates[j].Name
		}
		return a > b
	})
	return candidates[0], true
}

func (d *Dashboard) loadHistory() {
	if d.store == nil {
		return
	}
	h, err := d.store.WeeklyHistorySummary()
	if err == nil {
		d.history = h
		d.historyLastUpdate = time.Now()
	}
}

func (d *Dashboard) interfaceCacheStamp(iface state.InterfaceStats, ok bool) string {
	if !ok {
		return "none"
	}
	return fmt.Sprintf("%s|%t|%s|%s|%s|%d|%.2f|%.2f|%d|%d|%d|%d|%d|%d|%s",
		iface.Name,
		iface.Up,
		iface.IPAddress,
		iface.MACAddress,
		iface.Gateway,
		iface.MTU,
		iface.RXSpeed,
		iface.TXSpeed,
		iface.TotalRX,
		iface.TotalTX,
		iface.PacketsIn,
		iface.PacketsOut,
		iface.Errors,
		iface.Drops,
		iface.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
}

func (d *Dashboard) dnsHealth(snap state.AppStateSnapshot) (float64, string, lipgloss.Style) {
	total := 0
	success := 0
	for _, ds := range snap.DNSStats {
		total++
		if ds.Success {
			success++
		}
	}
	if total == 0 {
		return 0, "Unknown", lblStyle
	}
	pct := float64(success) * 100 / float64(total)
	if pct >= 100 {
		return pct, "Good", greenStyle
	}
	if pct >= 80 {
		return pct, "Warn", yellowStyle
	}
	return pct, "Fail", redStyle
}

func (d *Dashboard) statusStyle(status state.ConnectivityStatus) lipgloss.Style {
	switch status {
	case state.StatusOnline:
		return greenStyle
	case state.StatusDegraded:
		return yellowStyle
	case state.StatusOffline:
		return redStyle
	default:
		return lblStyle
	}
}

func (d *Dashboard) statusStyleBool(ok bool) lipgloss.Style {
	if ok {
		return greenStyle
	}
	return redStyle
}

func aggPing(snap state.AppStateSnapshot) (avg, loss, jitter float64) {
	var n int
	for _, ps := range snap.PingStats {
		avg += ps.AvgLatency
		loss += ps.PacketLoss
		jitter += ps.Jitter
		n++
	}
	if n > 0 {
		avg /= float64(n)
		loss /= float64(n)
		jitter /= float64(n)
	}
	return
}

func frameBox(content string, w int) string {
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = bdStyle.Render("|") + padVisual(lines[i], w) + bdStyle.Render("|")
	}
	top := bdStyle.Render("+" + strings.Repeat("-", w) + "+")
	bottom := bdStyle.Render("+" + strings.Repeat("-", w) + "+")
	return top + "\n" + strings.Join(lines, "\n") + "\n" + bottom
}

func joinHorizontal(parts []string, widths []int, h int) string {
	linesByPart := make([][]string, len(parts))
	for i, part := range parts {
		linesByPart[i] = forceHeight(strings.Split(part, "\n"), h)
	}
	var out []string
	for row := 0; row < h; row++ {
		var b strings.Builder
		for i := range parts {
			b.WriteString(padVisual(linesByPart[i][row], widths[i]))
			if i < len(parts)-1 {
				b.WriteByte(' ')
			}
		}
		out = append(out, b.String())
	}
	return strings.Join(out, "\n")
}

func joinVertical(parts []string, heights []int) string {
	var out []string
	for i, part := range parts {
		out = append(out, forceHeight(strings.Split(part, "\n"), heights[i])...)
		if i < len(parts)-1 {
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}

func forceHeight(lines []string, height int) []string {
	if len(lines) > height {
		return lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

func applyOuterMargin(content string, marginX, marginY int) string {
	lines := strings.Split(content, "\n")
	pad := strings.Repeat(" ", marginX)
	for i := range lines {
		lines[i] = pad + lines[i]
	}
	if marginY <= 0 {
		return strings.Join(lines, "\n")
	}
	blank := strings.Repeat(" ", visibleWidth(lines[0]))
	top := make([]string, marginY)
	bottom := make([]string, marginY)
	for i := 0; i < marginY; i++ {
		top[i], bottom[i] = blank, blank
	}
	out := append(top, lines...)
	out = append(out, bottom...)
	return strings.Join(out, "\n")
}

func normalizeViewport(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = padVisual(fitText(lines[i], width), width)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func lineCount(s string) int { return len(strings.Split(s, "\n")) }

func splitLine(left, right string, w int) string {
	left = fitText(left, w)
	right = fitText(right, w)
	if visibleWidth(left)+visibleWidth(right) > w {
		rightMax := minInt(visibleWidth(right), maxInt(8, w/3))
		leftMax := maxInt(1, w-rightMax-1)
		left = truncateText(left, leftMax)
		right = truncateText(right, rightMax)
	}
	gap := w - visibleWidth(left) - visibleWidth(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func layoutThree(left, middle, right string, w int) string {
	left = fitText(left, w)
	middle = fitText(middle, w)
	right = fitText(right, w)
	leftW := visibleWidth(left)
	middleW := visibleWidth(middle)
	rightW := visibleWidth(right)
	if leftW+middleW+rightW+4 <= w {
		centerWidth := w - leftW - rightW
		return left + lipgloss.NewStyle().Width(centerWidth).Align(lipgloss.Center).Render(middle) + right
	}
	if leftW+middleW+2 <= w {
		return left + strings.Repeat(" ", w-leftW-middleW) + middle
	}
	if middleW+rightW+2 <= w {
		return middle + strings.Repeat(" ", w-middleW-rightW) + right
	}
	return centerText(middle, w)
}

func centerText(s string, w int) string {
	s = fitText(s, w)
	vis := visibleWidth(s)
	if vis >= w {
		return s
	}
	left := (w - vis) / 2
	return strings.Repeat(" ", left) + s
}

func visibleWidth(s string) int {
	return lipgloss.Width(stripANSI(s))
}

func padVisual(s string, w int) string {
	s = fitText(s, w)
	if visibleWidth(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-visibleWidth(s))
}

func padRight(s string, w int) string {
	s = fitText(s, w)
	if visibleWidth(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-visibleWidth(s))
}

func fitText(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if visibleWidth(s) <= w {
		return s
	}
	return truncateText(s, w)
}

func truncateText(s string, w int) string {
	if visibleWidth(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	if w <= 3 {
		return strings.Repeat(".", w)
	}

	limit := w - 3
	var b strings.Builder
	visible := 0
	sawANSI := false
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			end := ansiSequenceEnd(s, i)
			if end <= i {
				i++
				continue
			}
			b.WriteString(s[i:end])
			sawANSI = true
			i = end
			continue
		}

		r, size := nextRune(s, i)
		if visible+lipgloss.Width(string(r)) > limit {
			break
		}
		b.WriteRune(r)
		visible += lipgloss.Width(string(r))
		i += size
	}

	if sawANSI {
		b.WriteString("\x1b[0m")
	}
	b.WriteString("...")
	return b.String()
}

func ansiSequenceEnd(s string, start int) int {
	if start+1 >= len(s) || s[start] != 0x1b {
		return start
	}
	i := start + 1
	if s[i] != '[' {
		return start + 1
	}
	i++
	for i < len(s) {
		ch := s[i]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			return i + 1
		}
		i++
	}
	return len(s)
}

func nextRune(s string, start int) (rune, int) {
	if start >= len(s) {
		return 0, 0
	}
	return utf8.DecodeRuneInString(s[start:])
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inEscape {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				inEscape = false
			}
			continue
		}
		if ch == 0x1b {
			inEscape = true
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func splitWidths(total, count, gap int) []int {
	widths := make([]int, count)
	usable := total - (count-1)*gap
	base := usable / count
	rem := usable % count
	for i := 0; i < count; i++ {
		widths[i] = base
		if i < rem {
			widths[i]++
		}
	}
	return widths
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func appendRing(values []float64, v float64, max int) []float64 {
	values = append(values, v)
	if len(values) > max {
		values = values[len(values)-max:]
	}
	return values
}

func sample(values []float64, n int) []float64 {
	if len(values) == 0 {
		return make([]float64, n)
	}
	if len(values) >= n {
		return append([]float64(nil), values[len(values)-n:]...)
	}
	out := make([]float64, n)
	copy(out[n-len(values):], values)
	return out
}

func sparkline(values []float64) string {
	if len(values) == 0 {
		return ""
	}
	bars := []byte{'.', ':', '-', '=', '+', '*', '#', '@'}
	maxVal := maxFloat(1, maxSlice(values))
	var b strings.Builder
	for _, v := range values {
		idx := int((v / maxVal) * 7)
		if idx < 0 {
			idx = 0
		}
		if idx > 7 {
			idx = 7
		}
		b.WriteByte(bars[idx])
	}
	return b.String()
}

func maxSlice(values []float64) float64 {
	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

func metricStyle(v, warn, crit float64) lipgloss.Style {
	if v >= crit {
		return redStyle
	}
	if v >= warn {
		return yellowStyle
	}
	return greenStyle
}

func parsePercent(s string) float64 {
	var out float64
	fmt.Sscanf(strings.TrimSuffix(s, "%"), "%f", &out)
	return out
}

func formatSpeed(v float64) string {
	if v <= 0 {
		return "---"
	}
	return fmt.Sprintf("%.2f Mbps", v)
}

func speedPanelValue(running bool, v float64) string {
	if running {
		return "Running..."
	}
	return formatSpeed(v)
}

func speedActionText(running bool) string {
	if running {
		return "Speed Test Running"
	}
	return "Start Speed Test"
}

func speedLatencyText(snap state.AppStateSnapshot) string {
	if snap.SpeedTest.LatencyMs > 0 {
		return fmt.Sprintf("%.1f ms", snap.SpeedTest.LatencyMs)
	}
	return "---"
}

func humanBytes(v uint64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%d B", v)
	}
	div, exp := uint64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffix := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.2f %s", float64(v)/float64(div), suffix[exp])
}

func humanCount(v uint64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.2f M", float64(v)/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.2f K", float64(v)/1_000)
	default:
		return fmt.Sprintf("%d", v)
	}
}

func fallback(s, f string) string {
	if strings.TrimSpace(s) == "" {
		return f
	}
	return s
}

func boolText(ok bool) string {
	if ok {
		return "UP"
	}
	return "DOWN"
}

func formatUptimeLong(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, mins, secs)
	}
	return fmt.Sprintf("%dh %dm %ds", hours, mins, secs)
}

func formatSinceShort(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if days > 0 {
		return fmt.Sprintf("%dd %dh ago", days, hours)
	}
	mins := int(d.Minutes())
	if mins > 0 {
		return fmt.Sprintf("%dm ago", mins)
	}
	return "just now"
}

func simplifyURL(s string) string {
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	return strings.TrimSuffix(s, "/")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func signInt(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	default:
		return 0
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
