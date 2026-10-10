package db

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/ops"
	"heliosian/internal/serve"
	"heliosian/internal/store"
	"heliosian/internal/vitals"
)

const (
	slowRequest = 750 * time.Millisecond
	shownErrors = 50
)

type tableSize struct {
	Name    string `json:"name"`
	Sheet   string `json:"sheet"`
	Rows    int    `json:"rows"`
	Columns int    `json:"columns"`
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
	Ops         ops.Snapshot     `json:"ops"`
}

type board struct {
	store  *Store
	queue  *store.Queue
	ops    *ops.Board
	mu     sync.Mutex
	model  *Model
	queues []queueCount
}

func RegisterDashboard(mux *http.ServeMux, s *Store, queue *store.Queue, external *ops.Board) {
	b := &board{store: s, queue: queue, ops: external}
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
	b.ops.Freshen()
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
	out := dashboard{At: time.Now(), Queues: b.queues, Ops: b.ops.Snapshot()}
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
