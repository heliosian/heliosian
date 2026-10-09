package db

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

var (
	cropColumns  = []string{"crop_left", "crop_top", "crop_width", "crop_height"}
	photoInputs  = append([]string{"original"}, cropColumns...)
	photoOutputs = []string{"reencode", "crop", "thumbnail"}
)

type Pictures struct {
	s       *Store
	queue   *store.Queue
	bucket  *blob.Bucket
	mu      sync.Mutex
	pending []string
	waiting map[string]bool
	wake    chan struct{}
}

func NewPictures(s *Store, queue *store.Queue, bucket *blob.Bucket) *Pictures {
	return &Pictures{s: s, queue: queue, bucket: bucket, waiting: map[string]bool{}, wake: make(chan struct{}, 1)}
}

func (p *Pictures) Start() {
	go p.run()
	queued := 0
	for _, row := range p.s.Model().Table("PHOTO").All() {
		if row["ready"] == "" {
			p.enqueue(row["id"])
			queued++
		}
	}
	slog.Info("pictures: queued photos not ready at startup", "queued", queued)
}

func (p *Pictures) watch(tx *store.Tx, c Change) {
	if c.Table != "PHOTO" || c.New == nil {
		return
	}
	if c.Old != nil && !slices.ContainsFunc(photoInputs, func(col string) bool { return c.Old[col] != c.New[col] }) {
		return
	}
	id := c.New["id"]
	tx.After(func() { p.enqueue(id) })
}

func (p *Pictures) enqueue(id string) {
	p.mu.Lock()
	if !p.waiting[id] {
		p.waiting[id] = true
		p.pending = append(p.pending, id)
	}
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *Pictures) next() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.pending) == 0 {
		return "", false
	}
	id := p.pending[0]
	p.pending = p.pending[1:]
	delete(p.waiting, id)
	return id, true
}

func (p *Pictures) run() {
	for range p.wake {
		for id, ok := p.next(); ok; id, ok = p.next() {
			start := time.Now()
			if err := p.make(id); err != nil {
				slog.Error("pictures: make", "photo", id, "error", err)
				continue
			}
			slog.Info("pictures: made", "photo", id, "took", time.Since(start).Round(time.Millisecond))
		}
	}
}

func cropBox(row store.Row) (image.Rectangle, bool, error) {
	cells := []string{}
	for _, col := range cropColumns {
		cells = append(cells, strings.TrimSpace(row[col]))
	}
	if !slices.ContainsFunc(cells, func(s string) bool { return s != "" }) {
		return image.Rectangle{}, false, nil
	}
	n := [4]int{}
	for i, cell := range cells {
		v, err := strconv.Atoi(cell)
		if err != nil {
			return image.Rectangle{}, false, fmt.Errorf("the crop box %v is not four whole numbers", cells)
		}
		n[i] = v
	}
	return image.Rect(n[0], n[1], n[0]+n[2], n[1]+n[3]), true, nil
}

func (p *Pictures) store(ctx context.Context, content []byte) (string, error) {
	name := "photos/" + blob.Name(content, "jpg")
	held, err := p.bucket.Exists(ctx, name)
	if err != nil || held {
		return name, err
	}
	return name, p.bucket.Put(ctx, name, "image/jpeg", content)
}

func (p *Pictures) make(id string) error {
	ctx := context.Background()
	row, ok := p.s.Model().Table("PHOTO").Get(id)
	if !ok {
		return nil
	}
	src, _, err := p.bucket.Get(ctx, row["original"])
	if err != nil {
		return err
	}
	want := map[string]any{"crop": ""}
	re, err := blob.Reencode(src)
	if err != nil {
		return err
	}
	if want["reencode"], err = p.store(ctx, re); err != nil {
		return err
	}
	shown := re
	box, cropped, err := cropBox(row)
	if err != nil {
		return err
	}
	if cropped {
		if shown, err = blob.Crop(re, box); err != nil {
			return err
		}
		if want["crop"], err = p.store(ctx, shown); err != nil {
			return err
		}
	}
	thumb, err := blob.Thumbnail(shown)
	if err != nil {
		return err
	}
	if want["thumbnail"], err = p.store(ctx, thumb); err != nil {
		return err
	}
	_, err = p.queue.Transact(ctx, access.System("pictures"), func(tx *store.Tx) error {
		now, ok := p.s.In(tx).Table("PHOTO").Get(id)
		if !ok || slices.ContainsFunc(photoInputs, func(col string) bool { return now[col] != row[col] }) {
			return nil
		}
		set := map[string]any{}
		for _, col := range photoOutputs {
			if now[col] != want[col] {
				set[col] = want[col]
			}
		}
		if now["ready"] != "Yes" {
			set["ready"] = "Yes"
		}
		if len(set) == 0 {
			return nil
		}
		_, _, err := stageWrite(p.s, tx, Edit{Set: id, Cells: set}, "pictures", nil, true, unchecked)
		return err
	})
	return err
}
