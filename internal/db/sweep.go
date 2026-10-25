package db

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

const (
	sweepActor = "sweep"
	sweepBatch = 25
	orphanAge  = time.Hour
)

type sweeper struct {
	s      *Store
	bucket *blob.Bucket
	in     *inbox
	tick   wake
}

func StartSweeper(s *Store, bucket *blob.Bucket) {
	x := &sweeper{s: s, bucket: bucket, in: s.Model().services.inbox(sweepInbox), tick: newWake()}
	go x.run()
	go x.runObjects()
	s.OnTick(x.tick.poke)
	x.in.wake.poke()
	x.tick.poke()
}

func (x *sweeper) runObjects() {
	for range x.tick {
		start := time.Now()
		removed, err := x.sweepObjects(context.Background(), start.Add(-orphanAge))
		if err != nil {
			slog.Error("sweep: objects", "error", err)
			continue
		}
		if removed > 0 {
			slog.Info("sweep: objects no content names", "removed", removed, "took", time.Since(start).Round(time.Millisecond))
		}
	}
}

func (x *sweeper) sweepObjects(ctx context.Context, before time.Time) (int, error) {
	old, err := x.bucket.Older(ctx, contentFolder, before)
	if err != nil {
		return 0, err
	}
	held := map[string]bool{}
	for _, row := range x.s.Model().Table("CONTENT").All() {
		held[row["blob"]] = true
	}
	removed := 0
	for _, o := range old {
		if held[o.Name] {
			continue
		}
		err := x.bucket.Remove(ctx, o.Name, o.Generation)
		if errors.Is(err, blob.ErrChanged) {
			slog.Info("sweep: object rewritten since listed", "object", o.Name)
			continue
		}
		if err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func (x *sweeper) run() {
	for range x.in.wake {
		x.in.take()
		for len(x.s.Model().unreferencedContent()) > 0 {
			start := time.Now()
			rows, written, err := x.sweep()
			if err != nil {
				slog.Error("sweep: content", "error", err)
				break
			}
			if written != nil {
				<-written
			}
			slog.Info("sweep: content", "rows", rows, "took", time.Since(start).Round(time.Millisecond))
		}
	}
}

func (m *Model) unreferencedContent() []store.Row {
	contents := m.Table("CONTENT")
	held := make([]bool, contents.Len())
	for _, t := range Tables {
		if t.Generated {
			continue
		}
		for _, c := range t.Columns {
			if c.Kind != Ref || c.Generated || (c.Target != "" && c.Target != "CONTENT") {
				continue
			}
			for id := range m.Table(t.Name).by[c.Name] {
				if i, ok := contents.byID[id]; ok {
					held[i] = true
				}
			}
		}
	}
	out := []store.Row{}
	for i, row := range contents.All() {
		if !held[i] {
			out = append(out, row)
		}
	}
	return out
}

func (x *sweeper) sweep() (int, <-chan struct{}, error) {
	rows := 0
	written, err := x.s.Transact(context.Background(), access.System(sweepActor), func(tx *store.Tx[Model]) error {
		gone := x.s.In(tx).unreferencedContent()
		gone = gone[:min(len(gone), sweepBatch)]
		rows = len(gone)
		return stageContentSweep(x.s, tx, gone)
	})
	return rows, written, err
}
