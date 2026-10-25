package db

import (
	"context"
	"errors"
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

func (m *Model) groupPhoto(group string) string {
	photos := m.Table("PHOTO").Referencing("group", group)
	slices.SortStableFunc(photos, func(x, y store.Row) int { return store.CompareKeys(x["order"], y["order"]) })
	for _, ph := range photos {
		if ph["image"] != "" {
			return ph["image"]
		}
	}
	return ""
}

type madeColumn struct {
	table, original, made string
}

var (
	cropColumns  = []string{"crop_left", "crop_top", "crop_width", "crop_height"}
	photoInputs  = append([]string{"original"}, cropColumns...)
	photoOutputs = []string{"reencode", "crop", "thumbnail"}

	madeColumns = []madeColumn{
		{"PERSON", "pronunciation_original", "pronunciation"},
		{"GROUP", "pronunciation_original", "pronunciation"},
		{"GROUP", "flyer_original", "flyer"},
	}
)

type Pictures struct {
	s       *Store
	bucket  *blob.Bucket
	mu      sync.Mutex
	pending []string
	waiting map[string]bool
	wake    chan struct{}
}

func NewPictures(s *Store, bucket *blob.Bucket) *Pictures {
	p := &Pictures{s: s, bucket: bucket, waiting: map[string]bool{}, wake: make(chan struct{}, 1)}
	in := s.Model().services.inbox(picturesInbox)
	go func() {
		for range in.wake {
			for _, d := range in.take() {
				if remake(d.change) || remade(d.change) {
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
	made := 0
	for _, m := range madeColumns {
		for _, row := range p.s.Model().Table(m.table).All() {
			if row[m.original] != "" && row[m.made] == "" {
				p.enqueue(row["id"])
				made++
			}
		}
	}
	slog.Info("pictures: queued photos, recordings and flyers not ready at startup", "queued", queued, "made", made)
}

func remake(c Change) bool {
	if c.Table != "PHOTO" || c.New == nil {
		return false
	}
	return c.Old == nil || slices.ContainsFunc(photoInputs, func(col string) bool { return c.Old[col] != c.New[col] })
}

func remade(c Change) bool {
	if c.New == nil {
		return false
	}
	return slices.ContainsFunc(madeColumns, func(m madeColumn) bool {
		return m.table == c.Table && c.New[m.original] != "" && (c.Old == nil || c.Old[m.original] != c.New[m.original])
	})
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
				slog.Error("pictures: make", "row", id, "error", err)
				continue
			}
			slog.Info("pictures: made", "row", id, "took", time.Since(start).Round(time.Millisecond))
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

const contentFolder = "content/"

type madeBytes struct {
	hash, mime string
	size       int
	written    bool
}

func madeOf(content []byte, mimeType string) madeBytes {
	return madeBytes{hash: blob.Hash(content), mime: mimeType, size: len(content)}
}

func (b madeBytes) row() store.Row {
	return store.Row{"hash": b.hash, "blob": contentFolder + b.hash, "mime": b.mime, "size": strconv.Itoa(b.size)}
}

func (b madeBytes) insert(as string) Edit {
	row := map[string]any{}
	for k, v := range b.row() {
		row[k] = v
	}
	return Edit{Insert: "CONTENT", As: as, Row: row}
}

func keepBytes(ctx context.Context, m *Model, bucket *blob.Bucket, mimeType string, content []byte) (madeBytes, error) {
	b := madeOf(content, mimeType)
	if _, held := m.Table("CONTENT").Find(b.hash, mimeType); held {
		return b, nil
	}
	b.written = true
	name := contentFolder + b.hash
	seen := int64(0)
	for {
		_, err := bucket.Put(ctx, name, mimeType, content, seen)
		if !errors.Is(err, blob.ErrChanged) {
			return b, err
		}
		now, err := bucket.Generation(ctx, name)
		if err != nil {
			return b, err
		}
		// An object no row holds may be about to be swept: writing it again moves its generation past the
		// sweep's listing, and so does another writer's write of these same bytes since we last looked.
		if seen != 0 && now != 0 {
			return b, nil
		}
		seen = now
	}
}

func (m *Model) contentBytes(ctx context.Context, bucket *blob.Bucket, id string) ([]byte, string, error) {
	row, ok := m.Table("CONTENT").Get(id)
	if !ok {
		return nil, "", fmt.Errorf("no content %s", id)
	}
	data, _, err := bucket.Get(ctx, row["blob"])
	return data, row["mime"], err
}

func (p *Pictures) make(id string) error {
	m := p.s.Model()
	if row, ok := m.Table("PHOTO").Get(id); ok {
		return p.makePhoto(id, row)
	}
	for _, made := range madeColumns {
		row, ok := m.Table(made.table).Get(id)
		if !ok || row[made.original] == "" || row[made.made] != "" {
			continue
		}
		if err := p.makeMade(made, id, row); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pictures) makeMade(made madeColumn, id string, row store.Row) error {
	ctx := context.Background()
	src, declared, err := p.s.Model().contentBytes(ctx, p.bucket, row[made.original])
	if err != nil {
		return err
	}
	var out []byte
	mimeType := "image/jpeg"
	if made.made == "pronunciation" {
		mimeType = "audio/mp4"
		audio, _, ok := blob.AudioType(declared)
		if !ok {
			return fmt.Errorf("%s is %s, not a recording type", row[made.original], declared)
		}
		out, err = blob.ReencodeRecording(audio, src)
	} else {
		out, err = blob.Reencode(src)
	}
	if err != nil {
		return err
	}
	kept, err := keepBytes(ctx, p.s.Model(), p.bucket, mimeType, out)
	if err != nil {
		return err
	}
	_, err = p.s.Transact(ctx, access.System("pictures"), func(tx *store.Tx[Model]) error {
		m := p.s.In(tx)
		now, ok := m.Table(made.table).Get(id)
		if !ok || now[made.original] != row[made.original] {
			return nil
		}
		w := &pendingWrites{}
		held, err := stageContent(m, w, map[string]string{}, kept)
		if err != nil {
			return err
		}
		if now[made.made] == held {
			return nil
		}
		w.add(Edit{Set: id, Cells: map[string]any{made.made: held}})
		_, err = stageEdits(p.s, tx, Env{System: "pictures"}, w.edits, "pictures", true)
		return err
	})
	return err
}

func (p *Pictures) makePhoto(id string, row store.Row) error {
	ctx := context.Background()
	m := p.s.Model()
	src, _, err := m.contentBytes(ctx, p.bucket, row["original"])
	if err != nil {
		return err
	}
	want := map[string]madeBytes{}
	re, err := blob.Reencode(src)
	if err != nil {
		return err
	}
	if want["reencode"], err = keepBytes(ctx, m, p.bucket, "image/jpeg", re); err != nil {
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
		if want["crop"], err = keepBytes(ctx, m, p.bucket, "image/jpeg", shown); err != nil {
			return err
		}
	}
	thumb, err := blob.Thumbnail(shown)
	if err != nil {
		return err
	}
	if want["thumbnail"], err = keepBytes(ctx, m, p.bucket, "image/jpeg", thumb); err != nil {
		return err
	}
	_, err = p.s.Transact(ctx, access.System("pictures"), func(tx *store.Tx[Model]) error {
		m := p.s.In(tx)
		now, ok := m.Table("PHOTO").Get(id)
		if !ok || slices.ContainsFunc(photoInputs, func(col string) bool { return now[col] != row[col] }) {
			return nil
		}
		w := &pendingWrites{}
		held := map[string]string{}
		set := map[string]any{}
		for _, col := range photoOutputs {
			v := ""
			if b, ok := want[col]; ok {
				var err error
				if v, err = stageContent(m, w, held, b); err != nil {
					return err
				}
			}
			if now[col] != v {
				set[col] = v
			}
		}
		if now["ready"] != "Yes" {
			set["ready"] = "Yes"
		}
		if len(set) == 0 {
			return nil
		}
		w.add(Edit{Set: id, Cells: set})
		_, err := stageEdits(p.s, tx, Env{System: "pictures"}, w.edits, "pictures", true)
		return err
	})
	return err
}
