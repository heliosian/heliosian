package db

import (
	"context"
	"log/slog"

	"heliosian/internal/blob"
	"heliosian/internal/store"
)

type stockPreview struct {
	done chan struct{}
	data []byte
	mime string
	err  error
}

func (st *Stock) Preview(ctx context.Context, id string, fetch func(store.Row) ([]byte, string, error)) ([]byte, string, error) {
	st.mu.Lock()
	row, ok := st.row(id)
	if !ok {
		st.mu.Unlock()
		return nil, "", blob.ErrNotFound
	}
	if row["preview"] != "" {
		st.mu.Unlock()
		return st.Bytes(ctx, row["preview"])
	}
	p, fetching := st.previews[id]
	if !fetching {
		p = &stockPreview{done: make(chan struct{})}
		st.previews[id] = p
	}
	st.mu.Unlock()
	if fetching {
		select {
		case <-p.done:
			return p.data, p.mime, p.err
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	p.data, p.mime, p.err = fetch(row)
	close(p.done)
	if p.err != nil {
		st.mu.Lock()
		delete(st.previews, id)
		st.mu.Unlock()
		return nil, "", p.err
	}
	st.queue.Hold()
	go func() {
		defer st.queue.Release()
		_, _, err := st.Keep(context.Background(), id, "preview", func(store.Row) ([]byte, string, error) {
			return p.data, p.mime, nil
		})
		if err != nil {
			slog.Error("save stock preview", "stock", id, "error", err)
		}
		st.mu.Lock()
		delete(st.previews, id)
		st.mu.Unlock()
	}()
	return p.data, p.mime, nil
}
