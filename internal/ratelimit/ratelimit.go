package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	per    int
	span   time.Duration
	mu     sync.Mutex
	recent map[string][]time.Time
}

func New(per int, span time.Duration) *Limiter {
	return &Limiter{per: per, span: span, recent: map[string][]time.Time{}}
}

func (l *Limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := []time.Time{}
	for _, t := range l.recent[key] {
		if now.Sub(t) < l.span {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.per {
		l.recent[key] = kept
		return false
	}
	l.recent[key] = append(kept, now)
	return true
}
