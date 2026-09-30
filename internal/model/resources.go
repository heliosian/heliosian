package model

import (
	"log/slog"
	"net/http"
	"slices"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/store"
)

func NewRegistry(s *Store, queue *store.Queue, calendar CalendarHooks, parties PartiesHooks, activities ActivitiesHooks, documents *DocumentFiler) *api.Registry[*Model] {
	reg := api.New(api.Config[*Model]{
		Actor:  func(r *http.Request, m *Model) access.Actor { return m.Directory.Actor(r, m.Held) },
		Held:   s.Held,
		Now:    now,
		Queue:  queue,
		Staged: s.In,
		Scope:  (*Model).at,
	})
	types := slices.Concat(DirectoryResources(), BirthdayResources(s), EmailListResources(s, documents), calendar.Resources(), parties.Resources(), activities.Resources(), MagicTagResources())
	for _, t := range types {
		reg.Add(t)
	}
	queue.OnSwap(func() { reg.Publish(s.Model()) })
	return reg
}

func (m *Model) personID(email string) []string {
	if p := m.Directory.Person(m.Directory.Resolve(email)); p != nil && p.ID != "" {
		return []string{p.ID}
	}
	return nil
}

func logAfter(wr api.Write[*Model], msg string, args ...any) {
	actor := wr.Query.Actor.Email
	ctx := wr.Request.Context()
	wr.Tx.After(func() { slog.InfoContext(ctx, msg, append([]any{"actor", actor}, args...)...) })
}

func (s *Store) stage(wr api.Write[*Model], sheet string, ops []store.Op, err error) error {
	if err != nil {
		return err
	}
	return s.Stage(wr.Tx, sheet, ops...)
}

func permitted(err error) bool {
	return err == nil
}
