package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
)

type Span struct {
	Name     string         `json:"name"`
	At       float64        `json:"ms"`
	Dur      float64        `json:"dur"`
	CPU      *float64       `json:"cpu,omitempty"`
	Count    int            `json:"count,omitempty"`
	Counts   map[string]int `json:"counts,omitempty"`
	Attrs    map[string]any `json:"attrs,omitempty"`
	Children []*Span        `json:"children,omitempty"`
	mu       sync.Mutex
	origin   time.Time
	began    time.Time
	cpu      time.Duration
	total    time.Duration
	tally    bool
}

type key struct{}

func New(name string) *Span {
	now := time.Now()
	return &Span{Name: name, origin: now, began: now, cpu: cpuTime()}
}

func With(ctx context.Context, s *Span) context.Context {
	return context.WithValue(ctx, key{}, s)
}

func From(ctx context.Context) *Span {
	s, _ := ctx.Value(key{}).(*Span)
	return s
}

func Start(ctx context.Context, name string) (context.Context, *Span) {
	s := From(ctx).Start(name)
	if s == nil {
		return ctx, nil
	}
	return With(ctx, s), s
}

func (s *Span) Start(name string) *Span {
	if s == nil {
		return nil
	}
	now := time.Now()
	child := &Span{Name: name, At: millis(now.Sub(s.origin)), origin: s.origin, began: now, cpu: cpuTime()}
	s.mu.Lock()
	s.Children = append(s.Children, child)
	s.mu.Unlock()
	return child
}

func (s *Span) End() {
	if s == nil {
		return
	}
	s.Dur = millis(time.Since(s.began))
	cpu := millis(cpuTime() - s.cpu)
	s.CPU = &cpu
}

func (s *Span) Set(key string, value any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Attrs == nil {
		s.Attrs = map[string]any{}
	}
	s.Attrs[key] = value
}

func (s *Span) Tally(name string) *Span {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.Children {
		if c.Name == name && c.tally {
			return c
		}
	}
	child := &Span{Name: name, At: millis(time.Since(s.origin)), origin: s.origin, tally: true}
	s.Children = append(s.Children, child)
	return child
}

func (s *Span) Add(d time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Count++
	s.total += d
	s.Dur = millis(s.total)
}

func (s *Span) Inc(key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Counts == nil {
		s.Counts = map[string]int{}
	}
	s.Counts[key]++
}

func (s *Span) Header() string {
	raw, err := json.Marshal(s)
	if err != nil {
		panic("trace: encode: " + err.Error())
	}
	var b strings.Builder
	for _, r := range string(raw) {
		if r < 0x80 {
			b.WriteRune(r)
			continue
		}
		if r > 0xFFFF {
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&b, `\u%04x\u%04x`, hi, lo)
			continue
		}
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	return b.String()
}

func millis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

func cpuTime() time.Duration {
	var u syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &u); err != nil {
		panic("trace: getrusage: " + err.Error())
	}
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}
