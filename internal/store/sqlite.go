package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

type CheckResult struct {
	ID          int64
	ServiceKey  string
	Status      string // "up", "down"
	StatusCode  int
	ResponseMs  int64
	Error       string
	CheckedAt   time.Time
}

type Alert struct {
	ID           int64
	ServiceKey   string
	AlertType    string // "down", "recovery"
	Message      string
	SentAt       time.Time
	Acknowledged bool
}

type DailyStatus struct {
	Date       time.Time
	TotalChecks int
	UpChecks    int
	DownChecks  int
	UptimePct   float64
}

// New creates a new store with the given database path
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	// Restrict DB file permissions
	os.Chmod(dbPath, 0600)

	return s, nil
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS check_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		service_key TEXT NOT NULL,
		status TEXT NOT NULL,
		status_code INTEGER,
		response_ms INTEGER,
		error TEXT,
		checked_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_check_results_service_key ON check_results(service_key);
	CREATE INDEX IF NOT EXISTS idx_check_results_checked_at ON check_results(checked_at);
	CREATE INDEX IF NOT EXISTS idx_check_results_service_checked ON check_results(service_key, checked_at);

	CREATE TABLE IF NOT EXISTS alerts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		service_key TEXT NOT NULL,
		alert_type TEXT NOT NULL,
		message TEXT,
		sent_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		acknowledged INTEGER DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_alerts_service_key ON alerts(service_key);
	CREATE INDEX IF NOT EXISTS idx_alerts_sent_at ON alerts(sent_at);

	CREATE TABLE IF NOT EXISTS service_state (
		service_key TEXT PRIMARY KEY,
		current_status TEXT NOT NULL,
		last_check DATETIME,
		last_status_change DATETIME,
		consecutive_failures INTEGER DEFAULT 0
	);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveCheckResult stores a check result
func (s *Store) SaveCheckResult(r *CheckResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO check_results (service_key, status, status_code, response_ms, error, checked_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, r.ServiceKey, r.Status, r.StatusCode, r.ResponseMs, r.Error, r.CheckedAt)
	return err
}

// UpdateServiceState updates the current state of a service
func (s *Store) UpdateServiceState(serviceKey, status string, failures int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Check if status changed
	var currentStatus sql.NullString
	err := s.db.QueryRow(`SELECT current_status FROM service_state WHERE service_key = ?`, serviceKey).Scan(&currentStatus)

	statusChanged := err == sql.ErrNoRows || !currentStatus.Valid || currentStatus.String != status

	if err == sql.ErrNoRows {
		_, err = s.db.Exec(`
			INSERT INTO service_state (service_key, current_status, last_check, last_status_change, consecutive_failures)
			VALUES (?, ?, ?, ?, ?)
		`, serviceKey, status, now, now, failures)
	} else if statusChanged {
		_, err = s.db.Exec(`
			UPDATE service_state
			SET current_status = ?, last_check = ?, last_status_change = ?, consecutive_failures = ?
			WHERE service_key = ?
		`, status, now, now, failures, serviceKey)
	} else {
		_, err = s.db.Exec(`
			UPDATE service_state
			SET last_check = ?, consecutive_failures = ?
			WHERE service_key = ?
		`, now, failures, serviceKey)
	}

	return err
}

// GetServiceState returns the current state of a service
func (s *Store) GetServiceState(serviceKey string) (status string, lastChange time.Time, failures int, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastChangeNull sql.NullTime
	err = s.db.QueryRow(`
		SELECT current_status, last_status_change, consecutive_failures
		FROM service_state WHERE service_key = ?
	`, serviceKey).Scan(&status, &lastChangeNull, &failures)

	if err == sql.ErrNoRows {
		return "unknown", time.Time{}, 0, nil
	}
	if lastChangeNull.Valid {
		lastChange = lastChangeNull.Time
	}
	return
}

// SaveAlert stores an alert
func (s *Store) SaveAlert(a *Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO alerts (service_key, alert_type, message, sent_at)
		VALUES (?, ?, ?, ?)
	`, a.ServiceKey, a.AlertType, a.Message, a.SentAt)
	return err
}

// GetLastAlert returns the most recent alert for a service
func (s *Store) GetLastAlert(serviceKey string) (*Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a := &Alert{}
	err := s.db.QueryRow(`
		SELECT id, service_key, alert_type, message, sent_at, acknowledged
		FROM alerts WHERE service_key = ? ORDER BY sent_at DESC LIMIT 1
	`, serviceKey).Scan(&a.ID, &a.ServiceKey, &a.AlertType, &a.Message, &a.SentAt, &a.Acknowledged)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

// GetDailyStats returns daily uptime stats for a service over the last N days
func (s *Store) GetDailyStats(serviceKey string, days int) ([]DailyStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().AddDate(0, 0, -days).Truncate(24 * time.Hour)

	rows, err := s.db.Query(`
		SELECT
			date(checked_at) as day,
			COUNT(*) as total,
			SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) as up_count,
			SUM(CASE WHEN status = 'down' THEN 1 ELSE 0 END) as down_count
		FROM check_results
		WHERE service_key = ? AND checked_at >= ?
		GROUP BY date(checked_at)
		ORDER BY day ASC
	`, serviceKey, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Build a map of existing data
	dataMap := make(map[string]DailyStatus)
	for rows.Next() {
		var dayStr string
		var ds DailyStatus
		if err := rows.Scan(&dayStr, &ds.TotalChecks, &ds.UpChecks, &ds.DownChecks); err != nil {
			return nil, err
		}
		ds.Date, _ = time.Parse("2006-01-02", dayStr)
		if ds.TotalChecks > 0 {
			ds.UptimePct = float64(ds.UpChecks) / float64(ds.TotalChecks) * 100
		}
		dataMap[dayStr] = ds
	}

	// Fill in all days
	result := make([]DailyStatus, days)
	for i := 0; i < days; i++ {
		day := time.Now().AddDate(0, 0, -(days-1-i)).Truncate(24 * time.Hour)
		dayStr := day.Format("2006-01-02")
		if ds, ok := dataMap[dayStr]; ok {
			result[i] = ds
		} else {
			result[i] = DailyStatus{Date: day, UptimePct: -1} // -1 = no data
		}
	}

	return result, nil
}

// GetOverallUptime calculates uptime percentage for a service over the last N days
func (s *Store) GetOverallUptime(serviceKey string, days int) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().AddDate(0, 0, -days)

	var total, up int
	err := s.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) as up_count
		FROM check_results
		WHERE service_key = ? AND checked_at >= ?
	`, serviceKey, cutoff).Scan(&total, &up)

	if err != nil || total == 0 {
		return 0, err
	}

	return float64(up) / float64(total) * 100, nil
}

// GetRecentChecks returns the most recent check results for a service
func (s *Store) GetRecentChecks(serviceKey string, limit int) ([]CheckResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, service_key, status, status_code, response_ms, error, checked_at
		FROM check_results
		WHERE service_key = ?
		ORDER BY checked_at DESC
		LIMIT ?
	`, serviceKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CheckResult
	for rows.Next() {
		var r CheckResult
		var errStr sql.NullString
		if err := rows.Scan(&r.ID, &r.ServiceKey, &r.Status, &r.StatusCode, &r.ResponseMs, &errStr, &r.CheckedAt); err != nil {
			return nil, err
		}
		if errStr.Valid {
			r.Error = errStr.String
		}
		results = append(results, r)
	}
	return results, nil
}

// PurgeOldData removes check results older than the specified number of days
func (s *Store) PurgeOldData(days int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -days)

	result, err := s.db.Exec(`DELETE FROM check_results WHERE checked_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}

	// Also purge old alerts
	s.db.Exec(`DELETE FROM alerts WHERE sent_at < ?`, cutoff)

	return result.RowsAffected()
}

// StartPurgeRoutine runs periodic cleanup of old data
func (s *Store) StartPurgeRoutine(ctx context.Context, days int) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Run once at startup
	if deleted, err := s.PurgeOldData(days); err != nil {
		slog.Error("Error purging old data", "error", err)
	} else if deleted > 0 {
		slog.Info("Purged old check results", "count", deleted)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if deleted, err := s.PurgeOldData(days); err != nil {
				slog.Error("Error purging old data", "error", err)
			} else if deleted > 0 {
				slog.Info("Purged old check results", "count", deleted)
			}
		}
	}
}

// GetAllServiceStates returns current state for all services
func (s *Store) GetAllServiceStates() (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT service_key, current_status FROM service_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := make(map[string]string)
	for rows.Next() {
		var key, status string
		if err := rows.Scan(&key, &status); err != nil {
			return nil, err
		}
		states[key] = status
	}
	return states, nil
}
