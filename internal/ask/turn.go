package ask

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/db"
	"heliosian/internal/store"
)

type found struct {
	mu   sync.Mutex
	refs map[string]string
}

func newFound() *found {
	return &found{refs: map[string]string{}}
}

func (f *found) note(href, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refs[href] = id
}

func (f *found) of(href string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.refs[href]
	return id, ok
}

type turn struct {
	ctx     context.Context
	m       *db.Model
	env     db.Env
	email   string
	sources Sources
	found   *found
}

func (a app) turn(r *http.Request) *turn {
	m := a.sources.Data.Model()
	email := auth.Email(r)
	return &turn{ctx: r.Context(), m: m, env: db.Env{Viewer: m.SignedIn(email), Now: a.sources.Now()}, email: email, sources: a.sources, found: newFound()}
}

func (t *turn) query(format string, args ...any) (db.Result, error) {
	q, err := db.Parse(fmt.Sprintf(format, args...))
	if err != nil {
		return db.Result{}, fmt.Errorf("a built-in query was refused: %w", err)
	}
	return t.m.Run(t.ctx, q, t.env), nil
}

func (t *turn) rows(format string, args ...any) ([]store.Row, db.Result, error) {
	res, err := t.query(format, args...)
	if err != nil {
		return nil, res, err
	}
	return res.Rows(), res, nil
}

func (t *turn) href(table string, row store.Row) string {
	href := t.m.Link(table, row, t.sources.Origin)
	if href != "" {
		t.found.note(href, row["id"])
	}
	return href
}

func (t *turn) today() time.Time {
	now := t.env.Now
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}
