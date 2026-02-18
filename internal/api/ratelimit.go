package api

import (
	"sync"
	"time"
)

const (
	maxLoginAttempts = 5
	loginWindow      = 5 * time.Minute
	lockoutDuration  = 15 * time.Minute
	cleanupInterval  = 10 * time.Minute
)

type ipRecord struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*ipRecord
}

func newLoginLimiter() *loginLimiter {
	l := &loginLimiter{
		attempts: make(map[string]*ipRecord),
	}
	go l.cleanupLoop()
	return l
}

// allow returns true if the IP is permitted to attempt login.
func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	rec, ok := l.attempts[ip]
	if !ok {
		return true
	}

	now := time.Now()

	// Check lockout
	if !rec.lockedUntil.IsZero() && now.Before(rec.lockedUntil) {
		return false
	}

	// Reset if window expired
	if now.Sub(rec.windowStart) > loginWindow {
		delete(l.attempts, ip)
		return true
	}

	return rec.count < maxLoginAttempts
}

// recordFailure increments the failure count and locks if threshold hit.
func (l *loginLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	rec, ok := l.attempts[ip]
	if !ok {
		l.attempts[ip] = &ipRecord{count: 1, windowStart: now}
		return
	}

	// Reset if window expired
	if now.Sub(rec.windowStart) > loginWindow {
		rec.count = 1
		rec.windowStart = now
		rec.lockedUntil = time.Time{}
		return
	}

	rec.count++
	if rec.count >= maxLoginAttempts {
		rec.lockedUntil = now.Add(lockoutDuration)
	}
}

// resetSuccess clears the record for an IP on successful login.
func (l *loginLimiter) resetSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// cleanupLoop prunes stale entries every cleanupInterval.
func (l *loginLimiter) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for ip, rec := range l.attempts {
			windowExpired := now.Sub(rec.windowStart) > loginWindow
			lockExpired := rec.lockedUntil.IsZero() || now.After(rec.lockedUntil)
			if windowExpired && lockExpired {
				delete(l.attempts, ip)
			}
		}
		l.mu.Unlock()
	}
}
