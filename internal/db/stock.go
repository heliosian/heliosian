package db

import (
	"context"
	"log/slog"
	"slices"
	"sync"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

const (
	stockActor = "imagesearch"
	stockBatch = 20
)

type StockHit struct {
	Source, SourceID, PreviewURL, URL, Download, Context string
}

func (h StockHit) row(id string) store.Row {
	return store.Row{"id": id, "source": h.Source, "source_id": h.SourceID, "preview_url": h.PreviewURL, "url": h.URL, "download": h.Download, "context": h.Context}
}

type Stock struct {
	s           *Store
	bucket      *blob.Bucket
	mu          sync.Mutex
	pending     map[string]chan struct{}
	remembering map[string]*stockWrite
	reserved    map[string]*stockWrite
	previews    map[string]*stockPreview
}

func NewStock(s *Store, bucket *blob.Bucket) *Stock {
	return &Stock{s: s, bucket: bucket, pending: map[string]chan struct{}{}, remembering: map[string]*stockWrite{}, reserved: map[string]*stockWrite{}, previews: map[string]*stockPreview{}}
}

type stockWrite struct {
	id   string
	hit  StockHit
	done chan struct{}
	err  error
}

func (st *Stock) Remember(hits []StockHit) []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	ids := make([]string, len(hits))
	writes := []*stockWrite{}
	m := st.s.Model()
	for i, h := range hits {
		key := h.Source + "/" + h.SourceID
		if row, ok := m.Table("STOCK").Find(h.Source, h.SourceID); ok {
			ids[i] = row["id"]
			continue
		}
		w, ok := st.remembering[key]
		if !ok {
			w = &stockWrite{id: Mint(StockPrefix, func(id string) bool {
				return m.Has(id) || st.reserved[id] != nil
			}), hit: h, done: make(chan struct{})}
			st.remembering[key] = w
			st.reserved[w.id] = w
			writes = append(writes, w)
		}
		ids[i] = w.id
	}
	if len(writes) > 0 {
		st.s.Hold()
		go func() {
			defer st.s.Release()
			err := st.remember(context.Background(), writes)
			if err != nil {
				slog.Error("remember stock hits", "error", err)
				st.finish(writes, err)
			}
		}()
	}
	return ids
}

func (st *Stock) finish(writes []*stockWrite, err error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, w := range writes {
		if st.reserved[w.id] != w {
			continue
		}
		w.err = err
		close(w.done)
		delete(st.remembering, w.hit.Source+"/"+w.hit.SourceID)
		delete(st.reserved, w.id)
	}
}

func (st *Stock) wait(ctx context.Context, id string) error {
	st.mu.Lock()
	w := st.reserved[id]
	st.mu.Unlock()
	if w == nil {
		return nil
	}
	select {
	case <-w.done:
		return w.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (st *Stock) remember(ctx context.Context, writes []*stockWrite) error {
	for chunk := range slices.Chunk(writes, stockBatch) {
		_, err := st.s.Transact(ctx, access.System(stockActor), func(tx *store.Tx[Model]) error {
			edits := []Edit{}
			for _, w := range chunk {
				row := map[string]any{}
				for col, v := range w.hit.row(w.id) {
					if col != "id" {
						row[col] = v
					}
				}
				edits = append(edits, Edit{Insert: "STOCK", Row: row, id: w.id})
			}
			_, err := stageEdits(st.s, tx, Env{System: stockActor}, edits, stockActor, true)
			return err
		})
		if err != nil {
			return err
		}
		st.finish(chunk, nil)
	}
	return nil
}

func (st *Stock) Row(id string) (store.Row, bool) {
	return st.s.Model().Table("STOCK").Get(id)
}

func (st *Stock) row(id string) (store.Row, bool) {
	if row, ok := st.Row(id); ok {
		return row, true
	}
	if w := st.reserved[id]; w != nil {
		return w.hit.row(id), true
	}
	return nil, false
}

func (st *Stock) Bytes(ctx context.Context, content string) ([]byte, string, error) {
	return st.s.Model().contentBytes(ctx, st.bucket, content)
}

func (st *Stock) Keep(ctx context.Context, id, column string, fetch func(store.Row) ([]byte, string, error)) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	for {
		st.mu.Lock()
		row, ok := st.row(id)
		st.mu.Unlock()
		if !ok {
			return "", false, blob.ErrNotFound
		}
		if row[column] != "" {
			return row[column], false, nil
		}
		key := id + " " + column
		st.mu.Lock()
		wait, fetching := st.pending[key]
		if !fetching {
			st.pending[key] = make(chan struct{})
		}
		st.mu.Unlock()
		if !fetching {
			content, err := st.fetch(ctx, id, column, row, fetch)
			st.mu.Lock()
			close(st.pending[key])
			delete(st.pending, key)
			st.mu.Unlock()
			return content, err == nil, err
		}
		select {
		case <-wait:
		case <-ctx.Done():
			return "", false, ctx.Err()
		}
	}
}

func (st *Stock) fetch(ctx context.Context, id, column string, row store.Row, fetch func(store.Row) ([]byte, string, error)) (string, error) {
	content, mimeType, err := fetch(row)
	if err != nil {
		return "", err
	}
	if err := st.wait(ctx, id); err != nil {
		return "", err
	}
	kept, err := keepBytes(ctx, st.s.Model(), st.bucket, mimeType, content)
	if err != nil {
		return "", err
	}
	held := ""
	_, err = st.s.Transact(ctx, access.System(stockActor), func(tx *store.Tx[Model]) error {
		m := st.s.In(tx)
		now, ok := m.Table("STOCK").Get(id)
		if !ok {
			return blob.ErrNotFound
		}
		if now[column] != "" {
			held = now[column]
			return nil
		}
		w := &pendingWrites{}
		content, err := stageContent(m, w, map[string]string{}, kept)
		if err != nil {
			return err
		}
		w.add(Edit{Set: id, Cells: map[string]any{column: content}})
		if _, err := stageEdits(st.s, tx, Env{System: stockActor}, w.edits, stockActor, true); err != nil {
			return err
		}
		now, _ = st.s.In(tx).Table("STOCK").Get(id)
		held = now[column]
		return nil
	})
	return held, err
}
