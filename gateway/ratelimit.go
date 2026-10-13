package main

import (
	"sync"
	"time"
)

// RateLimiter is a simple per-IP sliding-window limiter.
type RateLimiter struct {
	mu      sync.Mutex
	hits    map[string][]time.Time
	maxHits int
	window  time.Duration
}

func NewRateLimiter(maxHits int, window time.Duration) *RateLimiter {
	return &RateLimiter{hits: make(map[string][]time.Time), maxHits: maxHits, window: window}
}

// Allow reports whether a request from ip may proceed.
func (l *RateLimiter) Allow(ip string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.maxHits {
		l.hits[ip] = kept
		return false
	}
	l.hits[ip] = append(kept, now)
	return true
}
