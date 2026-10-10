package vitals

import (
	"log/slog"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"time"
)

const (
	keptErrors   = 200
	keptSamples  = 720
	keptRequests = 5000
	sampleEvery  = 5 * time.Second
	window       = time.Hour
)

type Error struct {
	Time    time.Time         `json:"time"`
	Message string            `json:"message"`
	Attrs   map[string]string `json:"attrs"`
}

type Sample struct {
	Time       time.Time `json:"time"`
	HeapMiB    uint64    `json:"heapMiB"`
	SysMiB     uint64    `json:"sysMiB"`
	CPU        float64   `json:"cpu"`
	GC         uint32    `json:"gc"`
	Goroutines int       `json:"goroutines"`
}

type request struct {
	time    time.Time
	app     string
	latency time.Duration
	status  int
}

type Latency struct {
	App      string  `json:"app"`
	Requests int     `json:"requests"`
	Failed   int     `json:"failed"`
	P50      float64 `json:"p50"`
	P95      float64 `json:"p95"`
	Max      float64 `json:"max"`
	Slow     int     `json:"slow"`
}

var state = struct {
	sync.Mutex
	errors   []Error
	samples  []Sample
	requests []request
	watchers map[chan struct{}]bool
}{watchers: map[chan struct{}]bool{}}

func kept[T any](list []T, item T, limit int) []T {
	list = append(list, item)
	if len(list) > limit {
		list = slices.Delete(list, 0, len(list)-limit)
	}
	return list
}

func notify() {
	for w := range state.watchers {
		select {
		case w <- struct{}{}:
		default:
		}
	}
}

func Watch() (<-chan struct{}, func()) {
	w := make(chan struct{}, 1)
	state.Lock()
	state.watchers[w] = true
	state.Unlock()
	return w, func() {
		state.Lock()
		delete(state.watchers, w)
		state.Unlock()
	}
}

func Poke() {
	state.Lock()
	defer state.Unlock()
	notify()
}

func RecordError(at time.Time, message string, attrs map[string]string) {
	state.Lock()
	defer state.Unlock()
	state.errors = kept(state.errors, Error{Time: at, Message: message, Attrs: attrs}, keptErrors)
	notify()
}

func RecordRequest(app string, latency time.Duration, status int) {
	state.Lock()
	defer state.Unlock()
	state.requests = kept(state.requests, request{time: time.Now(), app: app, latency: latency, status: status}, keptRequests)
}

func Errors() []Error {
	state.Lock()
	defer state.Unlock()
	return append([]Error{}, state.errors...)
}

func Samples() []Sample {
	state.Lock()
	defer state.Unlock()
	return append([]Sample{}, state.samples...)
}

func Latencies(slow time.Duration) []Latency {
	state.Lock()
	since := time.Now().Add(-window)
	byApp := map[string][]request{}
	for _, r := range state.requests {
		if r.time.After(since) {
			byApp[r.app] = append(byApp[r.app], r)
		}
	}
	state.Unlock()
	out := []Latency{}
	for app, rs := range byApp {
		l := Latency{App: app, Requests: len(rs)}
		ms := []float64{}
		for _, r := range rs {
			ms = append(ms, float64(r.latency.Microseconds())/1000)
			if r.status >= 500 {
				l.Failed++
			}
			if r.latency >= slow {
				l.Slow++
			}
		}
		slices.Sort(ms)
		l.P50, l.P95, l.Max = ms[len(ms)/2], ms[len(ms)*95/100], ms[len(ms)-1]
		out = append(out, l)
	}
	slices.SortFunc(out, func(a, b Latency) int { return b.Requests - a.Requests })
	return out
}

func cpuTime() time.Duration {
	var u syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &u); err != nil {
		panic("vitals: getrusage: " + err.Error())
	}
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}

func Sampler() {
	lastCPU, lastAt := cpuTime(), time.Now()
	for at := range time.Tick(sampleEvery) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		cpu := cpuTime()
		s := Sample{Time: at, HeapMiB: m.HeapAlloc >> 20, SysMiB: m.Sys >> 20, CPU: float64(cpu-lastCPU) / float64(at.Sub(lastAt)), GC: m.NumGC, Goroutines: runtime.NumGoroutine()}
		lastCPU, lastAt = cpu, at
		slog.Info("memory", "heap_mib", s.HeapMiB, "sys_mib", s.SysMiB, "gc", s.GC, "goroutines", s.Goroutines, "cpu", s.CPU)
		state.Lock()
		state.samples = kept(state.samples, s, keptSamples)
		notify()
		state.Unlock()
	}
}
