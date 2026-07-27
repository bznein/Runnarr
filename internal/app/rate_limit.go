package app

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{attempts: make(map[string][]time.Time)}
}

func (l *loginRateLimiter) allowAndRecord(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempts, cutoff := l.pruneLocked(key, now)
	if len(attempts) >= 10 {
		return false
	}
	l.attempts[key] = append(attempts, now)
	l.cleanupLocked(cutoff)
	return true
}

func (l *loginRateLimiter) blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempts, cutoff := l.pruneLocked(key, now)
	l.cleanupLocked(cutoff)
	return len(attempts) >= 10
}

func (l *loginRateLimiter) recordFailure(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempts, cutoff := l.pruneLocked(key, now)
	if len(attempts) < 10 {
		l.attempts[key] = append(attempts, now)
	}
	l.cleanupLocked(cutoff)
}

func (l *loginRateLimiter) pruneLocked(key string, now time.Time) ([]time.Time, time.Time) {
	cutoff := now.Add(-time.Minute)
	attempts := l.attempts[key][:0]
	for _, attempt := range l.attempts[key] {
		if attempt.After(cutoff) {
			attempts = append(attempts, attempt)
		}
	}
	l.attempts[key] = attempts
	return attempts, cutoff
}

func (l *loginRateLimiter) cleanupLocked(cutoff time.Time) {
	if len(l.attempts) <= 10_000 {
		return
	}
	for candidate, values := range l.attempts {
		if len(values) == 0 || values[len(values)-1].Before(cutoff) {
			delete(l.attempts, candidate)
		}
	}
}

func requestClientKey(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if value := strings.TrimSpace(r.Header.Get("X-Real-IP")); value != "" {
			return value
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}
