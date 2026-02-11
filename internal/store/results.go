package store

import (
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

type DailyUptime struct {
	Date        string  `json:"date"`
	UptimePct   float64 `json:"uptime_pct"`
	TotalChecks int     `json:"total_checks"`
}

func (s *Store) SaveResult(r *CheckResult) error {
	_, err := s.db.Exec(`
		INSERT INTO check_results (check_id, status, response_ms, message, metrics, checked_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, r.CheckID, r.Status, r.ResponseMs, r.Message, r.Metrics, r.CheckedAt)
	return err
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

// GetLastResult returns the most recent result for a check.
func (s *Store) GetLastResult(checkID string) (*CheckResult, error) {
	r := &CheckResult{}
	err := s.db.QueryRow(`
		SELECT id, check_id, status, response_ms, message, metrics, checked_at
		FROM check_results
		WHERE check_id = ?
		ORDER BY checked_at DESC
		LIMIT 1
	`, checkID).Scan(&r.ID, &r.CheckID, &r.Status, &r.ResponseMs, &r.Message, &r.Metrics, &r.CheckedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
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
