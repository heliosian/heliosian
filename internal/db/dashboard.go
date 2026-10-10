package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/serve"
	"heliosian/internal/store"
	"heliosian/internal/vitals"
)

const (
	slowRequest  = 750 * time.Millisecond
	shownErrors  = 50
	measureAfter = 10 * time.Minute
)

type tableSize struct {
	Name    string `json:"name"`
	Sheet   string `json:"sheet"`
	Rows    int    `json:"rows"`
	Columns int    `json:"columns"`
}

type bucketSize struct {
	Name     string                `json:"name"`
	Measured time.Time             `json:"measured"`
	Error    string                `json:"error,omitempty"`
	Folders  map[string]blob.Usage `json:"folders"`
}

type dashboard struct {
	At          time.Time        `json:"at"`
	Queues      []queueCount     `json:"queues"`
	LastRefresh time.Time        `json:"lastRefresh"`
	Errors      []vitals.Error   `json:"errors"`
	ErrorCount  int              `json:"errorCount"`
	Samples     []vitals.Sample  `json:"samples"`
	Latency     []vitals.Latency `json:"latency"`
	Tables      []tableSize      `json:"tables"`
	Buckets     []bucketSize     `json:"buckets"`
}

type Measurable interface {
	Usage(ctx context.Context) (map[string]blob.Usage, error)
}

type measured struct {
	bucket  Measurable
	size    bucketSize
	running bool
}

type board struct {
	store   *Store
	queue   *store.Queue
	mu      sync.Mutex
	model   *Model
	queues  []queueCount
	buckets []*measured
}

func RegisterDashboard(mux *http.ServeMux, s *Store, queue *store.Queue, buckets map[string]Measurable) {
	b := &board{store: s, queue: queue}
	for _, name := range slices.Sorted(maps.Keys(buckets)) {
		b.buckets = append(b.buckets, &measured{bucket: buckets[name], size: bucketSize{Name: name, Folders: map[string]blob.Usage{}}})
	}
	queue.OnSwap(vitals.Poke)
	allowed := func(r *http.Request) bool {
		return s.Model().SuperAdmin(auth.Email(r))
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if !allowed(r) {
			http.Redirect(w, r, "/resources", http.StatusFound)
			return
		}
		serve.File(w, r, "web/admin/dashboard/index.html")
	})
	mux.HandleFunc("GET /api/system", func(w http.ResponseWriter, r *http.Request) {
		serve.Write(w, r, http.StatusOK, map[string]bool{"allowed": allowed(r)})
	})
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if !allowed(r) {
			http.Error(w, "super admins only", http.StatusForbidden)
			return
		}
		b.stream(w, r)
	})
}

func (b *board) stream(w http.ResponseWriter, r *http.Request) {
	changed, stop := vitals.Watch()
	defer stop()
	b.measure()
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	for {
		encoded, err := json.Marshal(b.snapshot())
		if err != nil {
			slog.ErrorContext(r.Context(), "encode dashboard", "error", err)
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", encoded)
		if err := controller.Flush(); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-changed:
		}
	}
}

func (b *board) snapshot() dashboard {
	m := b.store.Model()
	b.mu.Lock()
	if b.model != m {
		b.model, b.queues = m, m.queueCounts()
	}
	out := dashboard{At: time.Now(), Queues: b.queues, Buckets: []bucketSize{}}
	for _, mb := range b.buckets {
		out.Buckets = append(out.Buckets, mb.size)
	}
	b.mu.Unlock()
	status := b.queue.Status()
	out.LastRefresh = status.LastRefresh
	out.Queues = append(slices.Clone(out.Queues), queueCount{Name: "pending writes", About: "commits and refreshes waiting on the write queue, plus work held open", Pending: status.Pending + status.Held})
	errors := vitals.Errors()
	since := time.Now().Add(-time.Hour)
	for _, e := range errors {
		if e.Time.After(since) {
			out.ErrorCount++
		}
	}
	slices.Reverse(errors)
	out.Errors = errors[:min(len(errors), shownErrors)]
	out.Samples = vitals.Samples()
	out.Latency = vitals.Latencies(slowRequest)
	out.Tables = []tableSize{}
	for _, t := range Tables {
		if t.Generated {
			continue
		}
		columns := 0
		for _, c := range t.Columns {
			if !c.Generated {
				columns++
			}
		}
		out.Tables = append(out.Tables, tableSize{Name: t.Name, Sheet: t.Sheet, Rows: m.Table(t.Name).Len(), Columns: columns})
	}
	return out
}

func (b *board) measure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, mb := range b.buckets {
		if mb.running || time.Since(mb.size.Measured) < measureAfter {
			continue
		}
		mb.running = true
		go func() {
			start := time.Now()
			folders, err := mb.bucket.Usage(context.Background())
			b.mu.Lock()
			mb.running = false
			mb.size.Measured = time.Now()
			mb.size.Error = ""
			if err != nil {
				slog.Error("measure bucket", "bucket", mb.size.Name, "error", err)
				mb.size.Error = err.Error()
			} else {
				mb.size.Folders = folders
			}
			b.mu.Unlock()
			slog.Info("measured bucket", "bucket", mb.size.Name, "took", time.Since(start))
			vitals.Poke()
		}()
	}
}
