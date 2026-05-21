package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	appState    *state.AppState
	alertEng    *alert.Engine
	config      *UIConfig
	speedTester *speedtest.Tester
	store       *storage.Repository
	ctx         context.Context
	width       int
	height      int
	ready       bool
	quitting    bool
	startTime   time.Time
}

var (
	bg          = lipgloss.Color("#0B0F14")
	bd          = lipgloss.Color("#374151")
	hdr         = lipgloss.Color("#22D3EE")
	txt         = lipgloss.Color("#F9FAFB")
	sec         = lipgloss.Color("#9CA3AF")
	green       = lipgloss.Color("#32D74B")
	red         = lipgloss.Color("#FF453A")
	yellow      = lipgloss.Color("#FFD60A")
	baseStyle   = lipgloss.NewStyle().Background(bg).Foreground(txt)
	bdStyle     = lipgloss.NewStyle().Foreground(bd)
	lblStyle    = lipgloss.NewStyle().Foreground(sec)
	valStyle    = lipgloss.NewStyle().Foreground(txt)
	cyanStyle   = lipgloss.NewStyle().Foreground(hdr).Bold(true)
	greenStyle  = lipgloss.NewStyle().Foreground(green).Bold(true)
	redStyle    = lipgloss.NewStyle().Foreground(red).Bold(true)
	yellowStyle = lipgloss.NewStyle().Foreground(yellow).Bold(true)
)

func NewDashboard(st *state.AppState, eng *alert.Engine, cfg *UIConfig) *Dashboard {
	return &Dashboard{
		appState:  st,
		alertEng:  eng,
		config:    cfg,
		startTime: time.Now(),
	}
}

func (d *Dashboard) SetSpeedTester(t *speedtest.Tester, ctx context.Context) {
	d.speedTester = t
	d.ctx = ctx
}

func (d *Dashboard) SetStore(store *storage.Repository) {
	d.store = store
}

func (d *Dashboard) Init() tea.Cmd { return nil }

type StateUpdateMsg struct{}

func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		d.ready = true
		return d, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			d.quitting = true
			return d, tea.Quit
		case "s":
			if d.speedTester != nil && d.ctx != nil {
				go d.speedTester.Run(d.ctx)
			}
		case "c":
			d.alertEng.Clear()
		}
	case StateUpdateMsg:
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
	if d.width < 40 || d.height < 6 {
		return baseStyle.Render(fmt.Sprintf("Terminal too small: %dx%d (min 40x6)", d.width, d.height))
	}

	snap := d.appState.Read()
	marginX := maxInt(1, d.width/100)
	marginY := maxInt(0, d.height/100)
	frameW := d.width - marginX*2
	if frameW < 20 {
		frameW = 20
	}
	sectionW := frameW - 2
	if sectionW < 20 {
		sectionW = 20
	}

	header := frameBox(d.renderHeaderBar(snap, sectionW-2), sectionW)
	footer := frameBox(d.renderFooterBar(sectionW-2), sectionW)
	note := frameBox(d.renderFooterNote(sectionW-2), sectionW)

	usedLines := 3 + 3 + 3
	bodySectionHeight := d.height - (marginY * 2) - usedLines
	if bodySectionHeight < 3 {
		bodySectionHeight = 3
	}
	bodyContentHeight := bodySectionHeight - 2
	body := frameBox(strings.Join(forceHeight([]string{""}, bodyContentHeight), "\n"), sectionW)

	content := strings.Join([]string{header, body, footer, note}, "\n")
	return baseStyle.Render(applyOuterMargin(content, marginX, marginY))
}

func (d *Dashboard) renderHeaderBar(snap state.AppStateSnapshot, w int) string {
	left := cyanStyle.Render("NetPulse TUI") + valStyle.Render(" v1.0.0 ") + yellowStyle.Render("(FREE)")
	status := statusBullet(snap.InternetStatus) + lblStyle.Render("Internet: ") + d.statusStyle(snap.InternetStatus).Render(snap.InternetStatus.String())
	uptime := lblStyle.Render("Uptime: ") + valStyle.Render(formatUptimeLong(time.Since(d.startTime)))
	clock := lblStyle.Render("◷ ") + valStyle.Render(time.Now().Format("15:04:05"))
	right := uptime + "    " + clock

	leftW := lipgloss.Width(stripANSI(left))
	statusW := lipgloss.Width(stripANSI(status))
	rightW := lipgloss.Width(stripANSI(right))

	if statusW >= w {
		return truncateText(status, w)
	}
	if leftW+statusW+rightW+4 <= w {
		centerWidth := w - leftW - rightW
		center := lipgloss.NewStyle().Width(centerWidth).Align(lipgloss.Center).Render(status)
		return left + center + right
	}
	if leftW+statusW+2 <= w {
		return left + strings.Repeat(" ", w-leftW-statusW) + status
	}
	if statusW+rightW+2 <= w {
		return status + strings.Repeat(" ", w-statusW-rightW) + right
	}
	return centerText(status, w)
}

func (d *Dashboard) renderFooterBar(w int) string {
	leftSets := [][]string{
		{
			greenStyle.Render("[Q]") + valStyle.Render(" Quit"),
			greenStyle.Render("[R]") + valStyle.Render(" Refresh"),
			greenStyle.Render("[S]") + valStyle.Render(" Speed Test"),
			greenStyle.Render("[H]") + valStyle.Render(" History"),
			greenStyle.Render("[C]") + valStyle.Render(" Config"),
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
	if visibleWidth(right) > w {
		return " " + padVisual(truncateText(right, w), w) + " "
	}
	return " " + padVisual(splitLine("", right, w), w) + " "
}

func (d *Dashboard) renderFooterNote(w int) string {
	note := cyanStyle.Render("Note: ") +
		valStyle.Render("You are using the FREE version. Only ") +
		yellowStyle.Render("1 device (this machine)") +
		valStyle.Render(" can be monitored.")
	return " " + padVisual(truncateText(note, w), w) + " "
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

func statusBullet(status state.ConnectivityStatus) string {
	switch status {
	case state.StatusOnline:
		return greenStyle.Render("● ")
	case state.StatusDegraded:
		return yellowStyle.Render("● ")
	case state.StatusOffline:
		return redStyle.Render("● ")
	default:
		return lblStyle.Render("● ")
	}
}

func splitLine(left, right string, w int) string {
	gap := w - visibleWidth(left) - visibleWidth(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
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

func frameBox(content string, w int) string {
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = bdStyle.Render("│") + padVisual(lines[i], w) + bdStyle.Render("│")
	}
	top := bdStyle.Render("┌" + strings.Repeat("─", w) + "┐")
	bottom := bdStyle.Render("└" + strings.Repeat("─", w) + "┘")
	return top + "\n" + strings.Join(lines, "\n") + "\n" + bottom
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
		top[i] = blank
		bottom[i] = blank
	}
	out := append(top, lines...)
	out = append(out, bottom...)
	return strings.Join(out, "\n")
}

func padVisual(s string, w int) string {
	vis := visibleWidth(s)
	if vis >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vis)
}

func centerText(s string, width int) string {
	vis := visibleWidth(s)
	if vis >= width {
		return s
	}
	left := (width - vis) / 2
	return strings.Repeat(" ", left) + s
}

func truncateText(s string, w int) string {
	if visibleWidth(s) <= w {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && visibleWidth(string(runes)+"...") > w {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "..."
}

func visibleWidth(s string) int {
	return lipgloss.Width(stripANSI(s))
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
