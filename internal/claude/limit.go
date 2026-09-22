// Package claude holds what every call to Claude shares: one hourly window per person, counted across Helios Ask and the Generate buttons alike.
package claude

import (
	"errors"
	"sync"
	"time"
)

const perHour = 30

var ErrTooMany = errors.New("that's a lot of Claude for one hour; try again a little later")

type Limiter struct {
	mu     sync.Mutex
	recent map[string][]time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{recent: map[string][]time.Time{}}
}

func (l *Limiter) Allow(email string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := []time.Time{}
	for _, t := range l.recent[email] {
		if now.Sub(t) < time.Hour {
			kept = append(kept, t)
		}
	}
	if len(kept) >= perHour {
		l.recent[email] = kept
		return false
	}
	l.recent[email] = append(kept, now)
	return true
}
