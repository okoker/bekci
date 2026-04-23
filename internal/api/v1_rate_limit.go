package api

import (
	"strconv"
	"sync"
	"time"
)

// v1RateLimiter enforces a flat N-requests-per-minute-per-token cap on the
// machine API (/api/v1/*). Per-token fixed 60-second rolling windows are
// stored in-process; no Redis or external store required for a single-
// binary deployment. Windows age out automatically after ~5 minutes of
// inactivity so memory stays bounded.
//
// Not a token bucket — no burst allowance. If an admin sets the limit to
// 60 and a caller fires 61 requests inside any 60s window, the 61st is
// rejected with 429. Simpler to reason about; admins can tune the limit
// from Settings > General without restarting the binary.
type v1RateLimiter struct {
	mu      sync.Mutex
	windows map[string]*v1Window
	stopCh  chan struct{}
}

type v1Window struct {
	start time.Time
	count int
}

const v1WindowDuration = time.Minute
const v1WindowStale = 5 * time.Minute

func newV1RateLimiter() *v1RateLimiter {
	l := &v1RateLimiter{
		windows: make(map[string]*v1Window),
		stopCh:  make(chan struct{}),
	}
	go l.cleanupLoop()
	return l
}

func (l *v1RateLimiter) Stop() { close(l.stopCh) }

// Allow reports whether this tokenID may serve another request under the
// given per-minute limit. When denied, retryAfter is the whole seconds
// until the window resets.
func (l *v1RateLimiter) Allow(tokenID string, limit int) (ok bool, retryAfter int) {
	if limit <= 0 {
		// Misconfiguration — fail open rather than lock everyone out.
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	w, exists := l.windows[tokenID]
	if !exists || now.Sub(w.start) >= v1WindowDuration {
		l.windows[tokenID] = &v1Window{start: now, count: 1}
		return true, 0
	}
	if w.count >= limit {
		remaining := v1WindowDuration - now.Sub(w.start)
		secs := int(remaining.Seconds())
		if secs < 1 {
			secs = 1
		}
		return false, secs
	}
	w.count++
	return true, 0
}

func (l *v1RateLimiter) cleanupLoop() {
	t := time.NewTicker(v1WindowStale)
	defer t.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case <-t.C:
			l.purgeStale()
		}
	}
}

func (l *v1RateLimiter) purgeStale() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-v1WindowStale)
	for id, w := range l.windows {
		if w.start.Before(cutoff) {
			delete(l.windows, id)
		}
	}
}

// cachedAPIRateLimit reads the admin-tunable setting with a 30s cache so
// the hot auth path doesn't hit the DB on every request. Invalidated from
// handleUpdateSettings when the admin changes the value.
func (s *Server) cachedAPIRateLimit() int {
	s.apiRateLimitCache.mu.Lock()
	defer s.apiRateLimitCache.mu.Unlock()
	if time.Now().Before(s.apiRateLimitCache.until) {
		n, _ := strconv.Atoi(s.apiRateLimitCache.value)
		return n
	}
	val, _ := s.store.GetSetting("api_rate_limit_per_min")
	s.apiRateLimitCache.value = val
	s.apiRateLimitCache.until = time.Now().Add(30 * time.Second)
	n, _ := strconv.Atoi(val)
	return n
}

func (s *Server) invalidateAPIRateLimitCache() {
	s.apiRateLimitCache.mu.Lock()
	s.apiRateLimitCache.until = time.Time{}
	s.apiRateLimitCache.mu.Unlock()
}
