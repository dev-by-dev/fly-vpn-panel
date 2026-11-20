package auth

import (
	"sync"
	"time"
)

// RateLimiter is an in-memory per-IP sliding window limiter.
// shortcut: idle IP entries are never evicted from the map; fine for the
// single-admin login endpoint, revisit if it is ever reused per-user.
type RateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

// NewRateLimiter allows limit requests per window per key.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:  limit,
		window: window,
		hits:   make(map[string][]time.Time),
	}
}

// Allow records a request from ip and reports whether it is within the
// limit. Denied requests still count (they refresh nothing but stay in the
// window only if already present).
func (r *RateLimiter) Allow(ip string) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	hits := r.hits[ip]
	keep := hits[:0]
	for _, t := range hits {
		if now.Sub(t) < r.window {
			keep = append(keep, t)
		}
	}
	if len(keep) >= r.limit {
		r.hits[ip] = keep
		return false
	}
	r.hits[ip] = append(keep, now)
	return true
}
