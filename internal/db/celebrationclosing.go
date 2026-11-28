package db

import (
	"cmp"
	"context"
	"log/slog"
	"time"

	"heliosian/internal/access"
)

const celebrationJob = "celebration closing"

func StartCelebrationClosing(s *Store) {
	kick := newWake()
	go func() {
		for range kick {
			closePassedCelebrations(context.Background(), s, time.Now().In(School))
		}
	}()
	s.OnTick(kick.poke)
	kick.poke()
}

func (m *Model) passedCelebrations(today string) []string {
	ids := []string{}
	for _, c := range m.celebrations() {
		day := cmp.Or(c["end"], c["start"])
		if c["adding"] != "" && day != "" && day[:min(len(day), len(DateLayout))] < today {
			ids = append(ids, c["id"])
		}
	}
	return ids
}

func closePassedCelebrations(ctx context.Context, s *Store, now time.Time) {
	ids := s.Model().passedCelebrations(now.Format(DateLayout))
	if len(ids) == 0 {
		return
	}
	edits := []Edit{}
	for _, id := range ids {
		edits = append(edits, Edit{Set: id, Cells: map[string]any{"adding": ""}})
	}
	if _, err := Write(ctx, s, access.System(celebrationJob), Env{System: celebrationJob, Now: now}, Batch{Batch: edits}); err != nil {
		slog.Error("celebrations: stop taking parties", "error", err)
		return
	}
	slog.Info("celebrations: stopped taking parties once passed", "celebrations", ids)
}
