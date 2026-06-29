package auth

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string]int
	locked   map[string]time.Time
	max      int
	lockout  time.Duration
}

func NewRateLimiter(maxAttempts int, lockoutDuration time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts: make(map[string]int),
		locked:   make(map[string]time.Time),
		max:      maxAttempts,
		lockout:  lockoutDuration,
	}
}

func (r *RateLimiter) IsLocked(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if until, ok := r.locked[ip]; ok {
		if time.Now().Before(until) {
			return true
		}
		delete(r.locked, ip)
		r.attempts[ip] = 0
	}

	return false
}

func (r *RateLimiter) RecordFailure(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.attempts[ip]++
	if r.attempts[ip] >= r.max {
		r.locked[ip] = time.Now().Add(r.lockout)
	}
}

func (r *RateLimiter) Reset(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.attempts, ip)
	delete(r.locked, ip)
}
