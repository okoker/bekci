package store

import (
	"database/sql"
	"fmt"
	"time"
)

type CheckResult struct {
	ID         int64     `json:"id"`
	CheckID    string    `json:"check_id"`
	Status     string    `json:"status"`
	ResponseMs int64     `json:"response_ms"`
	Message    string    `json:"message"`
	Metrics    string    `json:"metrics"`
	CheckedAt  time.Time `json:"checked_at"`
}

// CheckResultSlim contains only the fields needed for history bar rendering.
type CheckResultSlim struct {
	Status     string    `json:"status"`
	ResponseMs int64     `json:"response_ms"`
	CheckedAt  time.Time `json:"checked_at"`
}

type DailyUptime struct {
	Date        string  `json:"date"`
	UptimePct   float64 `json:"uptime_pct"`
	TotalChecks int     `json:"total_checks"`
}

func (s *Store) SaveResult(r *CheckResult) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Raw result
	if _, err := tx.Exec(`
		INSERT INTO check_results (check_id, status, response_ms, message, metrics, checked_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, r.CheckID, r.Status, r.ResponseMs, r.Message, r.Metrics, r.CheckedAt); err != nil {
		return fmt.Errorf("insert check_results: %w", err)
	}

	// 2. Upsert current state
	if _, err := tx.Exec(`
		INSERT INTO check_state (check_id, status, response_ms, message, metrics, checked_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(check_id) DO UPDATE SET
			status      = excluded.status,
			response_ms = excluded.response_ms,
			message     = excluded.message,
			metrics     = excluded.metrics,
			checked_at  = excluded.checked_at
	`, r.CheckID, r.Status, r.ResponseMs, r.Message, r.Metrics, r.CheckedAt); err != nil {
		return fmt.Errorf("upsert check_state: %w", err)
	}

	// 3. Upsert daily rollup
	upVal := 0
	downVal := 0
	if r.Status == "up" {
		upVal = 1
	} else {
		downVal = 1
	}
	day := r.CheckedAt.Format("2006-01-02")

	if _, err := tx.Exec(`
		INSERT INTO check_daily_rollups (check_id, day, total_count, up_count, down_count, avg_response_ms, max_response_ms)
		VALUES (?, ?, 1, ?, ?, ?, ?)
		ON CONFLICT(check_id, day) DO UPDATE SET
			total_count     = total_count + 1,
			up_count        = up_count + ?,
			down_count      = down_count + ?,
			avg_response_ms = (avg_response_ms * total_count + ?) / (total_count + 1),
			max_response_ms = MAX(max_response_ms, ?)
	`, r.CheckID, day, upVal, downVal, r.ResponseMs, r.ResponseMs,
		upVal, downVal, r.ResponseMs, r.ResponseMs); err != nil {
		return fmt.Errorf("upsert check_daily_rollups: %w", err)
	}

	return tx.Commit()
}

// GetRecentResults returns raw results for a check within the last N hours.
func (s *Store) GetRecentResults(checkID string, hours int) ([]CheckResult, error) {
	rows, err := s.db.Query(`
		SELECT id, check_id, status, response_ms, message, metrics, checked_at
		FROM check_results
		WHERE check_id = ? AND checked_at >= datetime('now', ?)
		ORDER BY checked_at ASC
	`, checkID, formatHoursOffset(hours))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CheckResult
	for rows.Next() {
		var r CheckResult
		if err := rows.Scan(&r.ID, &r.CheckID, &r.Status, &r.ResponseMs, &r.Message, &r.Metrics, &r.CheckedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	if results == nil {
		results = []CheckResult{}
	}
	return results, rows.Err()
}

// GetRecentResultsSlim returns only status, response_ms, checked_at for bar rendering.
func (s *Store) GetRecentResultsSlim(checkID string, hours int) ([]CheckResultSlim, error) {
	rows, err := s.db.Query(`
		SELECT status, response_ms, checked_at
		FROM check_results
		WHERE check_id = ? AND checked_at >= datetime('now', ?)
		ORDER BY checked_at ASC
	`, checkID, formatHoursOffset(hours))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CheckResultSlim
	for rows.Next() {
		var r CheckResultSlim
		if err := rows.Scan(&r.Status, &r.ResponseMs, &r.CheckedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	if results == nil {
		results = []CheckResultSlim{}
	}
	return results, rows.Err()
}

// GetDailyUptime returns per-day uptime percentages for the last N days.
func (s *Store) GetDailyUptime(checkID string, days int) ([]DailyUptime, error) {
	rows, err := s.db.Query(`
		SELECT date(checked_at) as day,
		       ROUND(100.0 * SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) / COUNT(*), 2) as uptime_pct,
		       COUNT(*) as total_checks
		FROM check_results
		WHERE check_id = ? AND checked_at >= datetime('now', ?)
		GROUP BY day
		ORDER BY day ASC
	`, checkID, formatDaysOffset(days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uptimes []DailyUptime
	for rows.Next() {
		var u DailyUptime
		if err := rows.Scan(&u.Date, &u.UptimePct, &u.TotalChecks); err != nil {
			return nil, err
		}
		uptimes = append(uptimes, u)
	}
	if uptimes == nil {
		uptimes = []DailyUptime{}
	}
	return uptimes, rows.Err()
}

// GetRecentResultsByWindow returns results for a check within the last N seconds.
func (s *Store) GetRecentResultsByWindow(checkID string, windowSeconds int) ([]CheckResult, error) {
	rows, err := s.db.Query(`
		SELECT id, check_id, status, response_ms, message, metrics, checked_at
		FROM check_results
		WHERE check_id = ? AND checked_at >= datetime('now', ?)
		ORDER BY checked_at DESC
	`, checkID, fmt.Sprintf("-%d seconds", windowSeconds))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CheckResult
	for rows.Next() {
		var r CheckResult
		if err := rows.Scan(&r.ID, &r.CheckID, &r.Status, &r.ResponseMs, &r.Message, &r.Metrics, &r.CheckedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	if results == nil {
		results = []CheckResult{}
	}
	return results, rows.Err()
}

// GetLastResult returns the most recent result for a check (from check_state table).
func (s *Store) GetLastResult(checkID string) (*CheckResult, error) {
	r := &CheckResult{}
	err := s.db.QueryRow(`
		SELECT check_id, status, response_ms, message, metrics, checked_at
		FROM check_state
		WHERE check_id = ?
	`, checkID).Scan(&r.CheckID, &r.Status, &r.ResponseMs, &r.Message, &r.Metrics, &r.CheckedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return r, nil
}

// GetUptimePercent returns the uptime percentage for a check over the last N days.
func (s *Store) GetUptimePercent(checkID string, days int) (float64, error) {
	var pct float64
	err := s.db.QueryRow(`
		SELECT COALESCE(
			ROUND(100.0 * SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 2),
			-1
		)
		FROM check_results
		WHERE check_id = ? AND checked_at >= datetime('now', ?)
	`, checkID, formatDaysOffset(days)).Scan(&pct)
	return pct, err
}

// BatchCheckSummary holds the last result + 90d uptime for a check.
type BatchCheckSummary struct {
	CheckID    string
	Status     string
	Message    string
	ResponseMs int64
	Uptime90d  float64
}

// GetBatchLastResultAndUptime returns last result + 90d uptime for all checks in one query.
func (s *Store) GetBatchLastResultAndUptime() (map[string]*BatchCheckSummary, error) {
	rows, err := s.db.Query(`
		WITH latest_at AS (
			SELECT check_id, MAX(checked_at) as max_at
			FROM check_results
			GROUP BY check_id
		),
		uptime AS (
			SELECT check_id,
				COALESCE(
					ROUND(100.0 * SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 2),
					-1
				) as uptime_pct
			FROM check_results
			WHERE checked_at >= datetime('now', '-90 days')
			GROUP BY check_id
		)
		SELECT cr.check_id, cr.status, cr.message, cr.response_ms,
			COALESCE(u.uptime_pct, -1)
		FROM latest_at la
		JOIN check_results cr ON cr.check_id = la.check_id AND cr.checked_at = la.max_at
		LEFT JOIN uptime u ON cr.check_id = u.check_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*BatchCheckSummary)
	for rows.Next() {
		cs := &BatchCheckSummary{}
		if err := rows.Scan(&cs.CheckID, &cs.Status, &cs.Message, &cs.ResponseMs, &cs.Uptime90d); err != nil {
			return nil, err
		}
		result[cs.CheckID] = cs
	}
	return result, rows.Err()
}

// PurgeOldResults deletes results older than the given number of days.
func (s *Store) PurgeOldResults(days int) (int64, error) {
	res, err := s.db.Exec(`
		DELETE FROM check_results
		WHERE checked_at < datetime('now', ?)
	`, formatDaysOffset(days))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func formatDaysOffset(days int) string {
	return fmt.Sprintf("-%d days", days)
}

func formatHoursOffset(hours int) string {
	return fmt.Sprintf("-%d hours", hours)
}
