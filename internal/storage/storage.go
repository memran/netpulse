package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/memran/netpulse/internal/alert"
	"github.com/memran/netpulse/internal/state"

	_ "modernc.org/sqlite"
)

type Repository struct {
	db *sql.DB
}

type Schema struct {
	Version int
}

func New(path string) (*Repository, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	r := &Repository{db: db}
	if err := r.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := r.PruneOldData(); err != nil {
		// Log or ignore non-fatal pruning errors during startup
	}

	return r, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS speed_tests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		download_mbps REAL NOT NULL,
		upload_mbps REAL NOT NULL,
		latency_ms REAL NOT NULL DEFAULT 0,
		error TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS alerts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		severity TEXT NOT NULL,
		source TEXT NOT NULL,
		message TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS ping_summaries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		target TEXT NOT NULL,
		avg_latency_ms REAL NOT NULL,
		min_latency_ms REAL NOT NULL,
		max_latency_ms REAL NOT NULL,
		packet_loss REAL NOT NULL,
		jitter_ms REAL NOT NULL,
		sent INTEGER NOT NULL,
		received INTEGER NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS dns_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		server TEXT NOT NULL,
		domain TEXT NOT NULL,
		response_time_ms REAL NOT NULL,
		success INTEGER NOT NULL,
		error_text TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS connectivity_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		status TEXT NOT NULL,
		public_ip TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	`

	if _, err := r.db.Exec(schema); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}

	_, err := r.db.Exec("INSERT OR IGNORE INTO schema_version (version) VALUES (1)")
	if err != nil {
		return fmt.Errorf("insert version: %w", err)
	}

	return nil
}

func (r *Repository) SaveSpeedTest(result state.SpeedTestResult) error {
	_, err := r.db.Exec(
		`INSERT INTO speed_tests (download_mbps, upload_mbps, latency_ms, error, created_at) VALUES (?, ?, ?, ?, ?)`,
		result.DownloadMbps, result.UploadMbps, result.LatencyMs, nullIfEmpty(result.Error),
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (r *Repository) SaveAlert(a alert.Alert) error {
	_, err := r.db.Exec(
		`INSERT INTO alerts (severity, source, message, created_at) VALUES (?, ?, ?, ?)`,
		a.Severity.String(), a.Source, a.Message,
		a.Timestamp.UTC().Format(time.RFC3339),
	)
	return err
}

func (r *Repository) SavePingSummary(ps state.PingStats) error {
	_, err := r.db.Exec(
		`INSERT INTO ping_summaries (target, avg_latency_ms, min_latency_ms, max_latency_ms, packet_loss, jitter_ms, sent, received, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ps.Target, ps.AvgLatency, ps.MinLatency, ps.MaxLatency, ps.PacketLoss, ps.Jitter, ps.Sent, ps.Received,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (r *Repository) SaveDNSResult(server, domain string, responseTimeMs float64, success bool, errText string) error {
	_, err := r.db.Exec(
		`INSERT INTO dns_results (server, domain, response_time_ms, success, error_text, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		server, domain, responseTimeMs, boolToInt(success), nullIfEmpty(errText),
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (r *Repository) SaveConnectivityEvent(status state.ConnectivityStatus, publicIP string) error {
	_, err := r.db.Exec(
		`INSERT INTO connectivity_events (status, public_ip, created_at) VALUES (?, ?, ?)`,
		status.String(), nullIfEmpty(publicIP),
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (r *Repository) RecentSpeedTests(limit int) ([]state.SpeedTestResult, error) {
	rows, err := r.db.Query(
		`SELECT download_mbps, upload_mbps, latency_ms, COALESCE(error,''), created_at FROM speed_tests ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []state.SpeedTestResult
	for rows.Next() {
		var r state.SpeedTestResult
		var errStr, createdAt string
		if err := rows.Scan(&r.DownloadMbps, &r.UploadMbps, &r.LatencyMs, &errStr, &createdAt); err != nil {
			return nil, err
		}
		if errStr != "" {
			r.Error = errStr
		}
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			r.CompletedAt = t
		}
		results = append(results, r)
	}
	return results, nil
}

func (r *Repository) RecentAlerts(limit int) ([]alert.Alert, error) {
	rows, err := r.db.Query(
		`SELECT severity, source, message, created_at FROM alerts ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []alert.Alert
	for rows.Next() {
		var a alert.Alert
		var sevStr, createdAt string
		if err := rows.Scan(&sevStr, &a.Source, &a.Message, &createdAt); err != nil {
			return nil, err
		}
		switch sevStr {
		case "WARN":
			a.Severity = alert.SeverityWarn
		case "CRIT":
			a.Severity = alert.SeverityCritical
		default:
			a.Severity = alert.SeverityInfo
		}
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			a.Timestamp = t
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

type HistoryDaySummary struct {
	Date       string
	UptimeStr  string
	AvgLatency float64
	PacketLoss float64
	DowntimeStr string
}

func (r *Repository) WeeklyHistorySummary() ([]HistoryDaySummary, error) {
	rows, err := r.db.Query(`
		SELECT 
			date(created_at) as dt,
			AVG(avg_latency_ms),
			AVG(packet_loss),
			SUM(sent),
			SUM(received)
		FROM ping_summaries 
		WHERE created_at >= date('now', '-7 days')
		GROUP BY dt
		ORDER BY dt DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []HistoryDaySummary
	for rows.Next() {
		var dt string
		var avgLat, pktLoss float64
		var sent, recv int
		if err := rows.Scan(&dt, &avgLat, &pktLoss, &sent, &recv); err != nil {
			continue
		}
		
		uptimePct := 0.0
		if sent > 0 {
			uptimePct = (float64(recv) / float64(sent)) * 100
		}
		
		// Approximate downtime assuming 5 seconds per ping
		downtimeSecs := (sent - recv) * 5
		var downtimeStr string
		if downtimeSecs < 60 {
			downtimeStr = fmt.Sprintf("%dm", downtimeSecs/60)
			if downtimeSecs == 0 {
				downtimeStr = "0m"
			}
		} else {
			downtimeStr = fmt.Sprintf("%dm", downtimeSecs/60)
		}
		
		// Approximate uptime hours for the day (max 24h)
		uptimeHours := 24.0
		if downtimeSecs > 0 {
			uptimeHours = 24.0 - (float64(downtimeSecs) / 3600.0)
		}
		
		h := int(uptimeHours)
		m := int((uptimeHours - float64(h)) * 60)
		uptimeStr := fmt.Sprintf("%dh %02dm (%.1f%%)", h, m, uptimePct)
		
		summaries = append(summaries, HistoryDaySummary{
			Date: dt,
			UptimeStr: uptimeStr,
			AvgLatency: avgLat,
			PacketLoss: pktLoss,
			DowntimeStr: downtimeStr,
		})
	}
	return summaries, nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *Repository) PruneOldData() error {
	cutoff := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	queries := []string{
		"DELETE FROM speed_tests WHERE created_at < ?",
		"DELETE FROM alerts WHERE created_at < ?",
		"DELETE FROM ping_summaries WHERE created_at < ?",
		"DELETE FROM dns_results WHERE created_at < ?",
		"DELETE FROM connectivity_events WHERE created_at < ?",
	}
	for _, q := range queries {
		if _, err := r.db.Exec(q, cutoff); err != nil {
			return fmt.Errorf("execute prune '%s': %w", q, err)
		}
	}
	return nil
}
