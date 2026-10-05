package db

import (
	"context"
	"log/slog"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

const (
	sweepActor = "sweep"
	sweepBatch = 25
)

type sweeper struct {
	s      *Store
	queue  *store.Queue
	bucket *blob.Bucket
	poke   chan struct{}
}

func StartSweeper(s *Store, queue *store.Queue, bucket *blob.Bucket) {
	x := &sweeper{s: s, queue: queue, bucket: bucket, poke: make(chan struct{}, 1)}
	go x.run()
	queue.OnSwap(func() {
		select {
		case x.poke <- struct{}{}:
		default:
		}
	})
}

func (x *sweeper) run() {
	for range x.poke {
		for len(x.s.Model().unreferencedContent()) > 0 {
			start := time.Now()
			rows, objects, written, err := x.sweep()
			if err != nil {
				slog.Error("sweep: content", "error", err)
				break
			}
			if written != nil {
				<-written
			}
			slog.Info("sweep: content", "rows", rows, "objects", objects, "took", time.Since(start).Round(time.Millisecond))
		}
	}
}

func (m *Model) unreferencedContent() []store.Row {
	out := []store.Row{}
	for _, row := range m.Table("CONTENT").All() {
		if !m.contentReferenced(row["id"]) {
			out = append(out, row)
		}
	}
	return out
}

func (m *Model) contentReferenced(id string) bool {
	for _, t := range Tables {
		if t.Generated || t.Name == ChangesTable {
			continue
		}
		for _, c := range t.Columns {
			if c.Kind != Ref || (c.Target != "" && c.Target != "CONTENT") {
				continue
			}
			if len(m.Table(t.Name).Referencing(c.Name, id)) > 0 {
				return true
			}
		}
	}
	return false
}

func dropUnheld(s *Store, bucket *blob.Bucket, names []string) {
	held := map[string]bool{}
	for _, row := range s.Model().Table("CONTENT").All() {
		held[row["blob"]] = true
	}
	for _, name := range names {
		if held[name] {
			continue
		}
		if err := bucket.Remove(context.Background(), name); err != nil {
			slog.Error("sweep: remove", "object", name, "error", err)
		}
	}
}

func (x *sweeper) sweep() (int, int, <-chan struct{}, error) {
	rows, objects := 0, 0
	written, err := x.queue.Transact(context.Background(), access.System(sweepActor), func(tx *store.Tx) error {
		m := x.s.In(tx)
		gone := m.unreferencedContent()
		gone = gone[:min(len(gone), sweepBatch)]
		deleted := map[string]bool{}
		for _, row := range gone {
			deleted[row["id"]] = true
		}
		blobs := map[string]bool{}
		for _, row := range gone {
			blobs[row["blob"]] = true
		}
		for _, row := range m.Table("CONTENT").All() {
			if !deleted[row["id"]] {
				delete(blobs, row["blob"])
			}
		}
		ops := []store.Op{}
		for _, row := range gone {
			ops = append(ops, store.Delete("CONTENT", store.Row{"id": row["id"]}))
		}
		if err := x.s.Stage(tx, DocumentsSheet, ops...); err != nil {
			return err
		}
		tx.After(func() {
			for name := range blobs {
				if err := x.bucket.Remove(context.Background(), name); err != nil {
					slog.Error("sweep: remove", "object", name, "error", err)
					continue
				}
				objects++
			}
		})
		rows = len(gone)
		return nil
	})
	return rows, objects, written, err
}
