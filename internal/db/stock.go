package db

import (
	"context"
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

type Stock struct {
	s       *Store
	queue   *store.Queue
	bucket  *blob.Bucket
	mu      sync.Mutex
	pending map[string]chan struct{}
}

func NewStock(s *Store, queue *store.Queue, bucket *blob.Bucket) *Stock {
	return &Stock{s: s, queue: queue, bucket: bucket, pending: map[string]chan struct{}{}}
}

func (st *Stock) Remember(ctx context.Context, hits []StockHit) ([]string, error) {
	ids := make([]string, len(hits))
	for start := 0; start < len(hits); start += stockBatch {
		chunk := hits[start:min(len(hits), start+stockBatch)]
		_, err := st.queue.Transact(ctx, access.System(stockActor), func(tx *store.Tx) error {
			m := st.s.In(tx)
			edits := []Edit{}
			added := map[string]int{}
			at := map[int]int{}
			for n, h := range chunk {
				if row, ok := m.Table("STOCK").Find(h.Source, h.SourceID); ok {
					ids[start+n] = row["id"]
					continue
				}
				k := h.Source + "/" + h.SourceID
				if e, ok := added[k]; ok {
					at[start+n] = e
					continue
				}
				row := map[string]any{"source": h.Source, "source_id": h.SourceID, "preview_url": h.PreviewURL, "url": h.URL}
				for col, v := range map[string]string{"download": h.Download, "context": h.Context} {
					if v != "" {
						row[col] = v
					}
				}
				added[k] = len(edits)
				at[start+n] = len(edits)
				edits = append(edits, Edit{Insert: "STOCK", Row: row})
			}
			if len(edits) == 0 {
				return nil
			}
			written, err := stageEdits(st.s, tx, Env{System: stockActor}, edits, stockActor, true)
			if err != nil {
				return err
			}
			for i, e := range at {
				ids[i] = written[e]
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (st *Stock) Row(id string) (store.Row, bool) {
	return st.s.Model().Table("STOCK").Get(id)
}

func (st *Stock) Bytes(ctx context.Context, content string) ([]byte, string, error) {
	return st.s.Model().contentBytes(ctx, st.bucket, content)
}

func (st *Stock) Keep(ctx context.Context, id, column string, fetch func(store.Row) ([]byte, string, error)) (string, bool, error) {
	for {
		row, ok := st.Row(id)
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
	kept, err := keepBytes(ctx, st.s.Model(), st.bucket, mimeType, content)
	if err != nil {
		return "", err
	}
	held := ""
	_, err = st.queue.Transact(ctx, access.System(stockActor), func(tx *store.Tx) error {
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
