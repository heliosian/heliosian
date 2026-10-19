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

	recordingTables = []string{"PERSON", "GROUP"}
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
	p := &Pictures{s: s, queue: queue, bucket: bucket, waiting: map[string]bool{}, wake: make(chan struct{}, 1)}
	in := s.Model().services.inbox(picturesInbox)
	go func() {
		for range in.wake {
			for _, d := range in.take() {
				if remake(d.change) || rerecord(d.change) {
					p.enqueue(d.change.New["id"])
				}
			}
		}
	}()
	return p
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
	recordings := 0
	for _, table := range recordingTables {
		for _, row := range p.s.Model().Table(table).All() {
			if row["pronunciation_original"] != "" && row["pronunciation"] == "" {
				p.enqueue(row["id"])
				recordings++
			}
		}
	}
	slog.Info("pictures: queued photos and recordings not ready at startup", "queued", queued, "recordings", recordings)
}

func remake(c Change) bool {
	if c.Table != "PHOTO" || c.New == nil {
		return false
	}
	return c.Old == nil || slices.ContainsFunc(photoInputs, func(col string) bool { return c.Old[col] != c.New[col] })
}

func rerecord(c Change) bool {
	if !slices.Contains(recordingTables, c.Table) || c.New == nil || c.New["pronunciation_original"] == "" {
		return false
	}
	return c.Old == nil || c.Old["pronunciation_original"] != c.New["pronunciation_original"]
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
	return name, p.put(ctx, name, "image/jpeg", content)
}

func (p *Pictures) put(ctx context.Context, name, mimeType string, content []byte) error {
	held, err := p.bucket.Exists(ctx, name)
	if err != nil || held {
		return err
	}
	return p.bucket.Put(ctx, name, mimeType, content)
}

func (p *Pictures) make(id string) error {
	m := p.s.Model()
	if row, ok := m.Table("PHOTO").Get(id); ok {
		return p.makePhoto(id, row)
	}
	for _, table := range recordingTables {
		if row, ok := m.Table(table).Get(id); ok {
			return p.makeRecording(table, id, row)
		}
	}
	return nil
}

func (p *Pictures) makeRecording(table, id string, row store.Row) error {
	ctx := context.Background()
	if row["pronunciation_original"] == "" {
		return nil
	}
	src, declared, err := p.bucket.Get(ctx, row["pronunciation_original"])
	if err != nil {
		return err
	}
	mimeType, _, ok := blob.AudioType(declared)
	if !ok {
		return fmt.Errorf("%s is stored as %s, not a recording type", row["pronunciation_original"], declared)
	}
	out, err := blob.ReencodeRecording(mimeType, src)
	if err != nil {
		return err
	}
	name := "pronunciation/" + blob.Name(out, "m4a")
	if err := p.put(ctx, name, "audio/mp4", out); err != nil {
		return err
	}
	_, err = p.queue.Transact(ctx, access.System("pictures"), func(tx *store.Tx) error {
		now, ok := p.s.In(tx).Table(table).Get(id)
		if !ok || now["pronunciation_original"] != row["pronunciation_original"] || now["pronunciation"] == name {
			return nil
		}
		_, err := stageEdits(p.s, tx, Env{System: "pictures"}, []Edit{{Set: id, Cells: map[string]any{"pronunciation": name}}}, "pictures", true)
		return err
	})
	return err
}

func (p *Pictures) makePhoto(id string, row store.Row) error {
	ctx := context.Background()
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
		_, err := stageEdits(p.s, tx, Env{System: "pictures"}, []Edit{{Set: id, Cells: set}}, "pictures", true)
		return err
	})
	return err
}
