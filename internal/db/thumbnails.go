package db

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/store"
)

const (
	thumbnailActor = "thumbnail"
	thumbnailTries = 3
)

type drawFunc func(ctx context.Context, raw []byte, page string) ([]byte, error)

var drawers = map[string]drawFunc{
	"text/html":  drawPage,
	"image/jpeg": drawImage,
	"image/png":  drawImage,
	"image/gif":  drawImage,
	"image/bmp":  drawImage,
	"image/webp": drawImage,
}

var pageImageProperties = []string{"og:image", "og:image:url", "og:image:secure_url", "twitter:image"}

type Thumbnailer struct {
	s      *Store
	bucket *blob.Bucket
	tries  map[[2]string]int
	in     *inbox
}

func StartThumbnailer(s *Store, bucket *blob.Bucket) {
	t := &Thumbnailer{s: s, bucket: bucket, tries: map[[2]string]int{}, in: s.Model().services.inbox(thumbnailInbox)}
	go t.run()
	s.OnTick(t.in.wake.poke)
	t.in.wake.poke()
}

func (t *Thumbnailer) run() {
	for range t.in.wake {
		t.in.take()
		for _, doc := range t.pending() {
			id, key := doc["id"], thumbnailKey(doc)
			start := time.Now()
			drawn, err := t.make(doc)
			if err != nil {
				t.tries[key]++
				if t.tries[key] >= thumbnailTries {
					slog.Error("thumbnail, left until a restart", "document", id, "content", doc["content"], "tries", t.tries[key], "error", err)
					continue
				}
				slog.Warn("thumbnail, to be tried again", "document", id, "content", doc["content"], "tries", t.tries[key], "error", err)
				continue
			}
			slog.Info("thumbnail: made", "document", id, "drawn", drawn, "took", time.Since(start).Round(time.Millisecond))
		}
	}
}

func (t *Thumbnailer) pending() []store.Row {
	m := t.s.Model()
	contents := m.Table("CONTENT")
	out := []store.Row{}
	for _, doc := range m.Table("DOCUMENT").All() {
		if doc["content"] == "" || doc["thumbnailed"] != "" || t.tries[thumbnailKey(doc)] >= thumbnailTries {
			continue
		}
		c, ok := contents.Get(doc["content"])
		if !ok {
			continue
		}
		if _, ok := drawers[baseType(c["mime"])]; ok {
			out = append(out, doc)
		}
	}
	return out
}

func thumbnailKey(doc store.Row) [2]string {
	return [2]string{doc["id"], doc["content"]}
}

func (t *Thumbnailer) make(doc store.Row) (bool, error) {
	ctx := context.Background()
	m := t.s.Model()
	raw, mimeType, err := m.contentBytes(ctx, t.bucket, doc["content"])
	if err != nil {
		return false, err
	}
	thumb, err := drawers[baseType(mimeType)](ctx, raw, doc["url"])
	if err != nil {
		return false, err
	}
	return thumb != nil, saveThumbnail(ctx, t.s, t.bucket, access.System(thumbnailActor), Env{System: thumbnailActor}, doc, thumb, true)
}

func saveThumbnail(ctx context.Context, s *Store, bucket *blob.Bucket, actor access.Actor, env Env, doc store.Row, thumb []byte, whole bool) error {
	var kept madeBytes
	if thumb != nil {
		var err error
		if kept, err = keepBytes(ctx, s.Model(), bucket, "image/jpeg", thumb); err != nil {
			return err
		}
	}
	_, err := s.Transact(ctx, actor, func(tx *store.Tx[Model]) error {
		m := s.In(tx)
		now, ok := m.Table("DOCUMENT").Get(doc["id"])
		if !ok || now["thumbnailed"] != "" || now["content"] != doc["content"] {
			return nil
		}
		w := &pendingWrites{}
		set := map[string]any{"thumbnailed": time.Now().In(School).Format(cells.StampFormat)}
		if thumb != nil {
			id, err := stageContent(m, w, map[string]string{}, kept)
			if err != nil {
				return err
			}
			set["thumbnail"] = id
		}
		w.add(Edit{Set: doc["id"], Cells: set})
		_, err := stageEdits(s, tx, env, w.edits, thumbnailActor, whole)
		return err
	})
	return err
}

func drawImage(_ context.Context, raw []byte, _ string) ([]byte, error) {
	return blob.Thumbnail(raw)
}

func drawPage(ctx context.Context, raw []byte, page string) ([]byte, error) {
	src, err := pageImage(raw, page)
	if err != nil || src == "" {
		return nil, err
	}
	got, err := fetchFrom(ctx, store.Row{"relation": "image", "url": src}, src, false)
	if err != nil {
		return nil, fmt.Errorf("fetch its picture %s: %w", ShortSource(src), err)
	}
	if got.stop != "" {
		slog.Info("thumbnail: the page's picture is not kept", "url", ShortSource(src), "fetch", got.stop, "why", got.why)
		return nil, nil
	}
	return blob.Thumbnail(got.body)
}

func pageImage(raw []byte, page string) (string, error) {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	base, err := url.Parse(page)
	if err != nil {
		return "", nil
	}
	found := map[string]string{}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode {
			continue
		}
		name, content, href := "", "", ""
		for _, attr := range n.Attr {
			switch strings.ToLower(attr.Key) {
			case "property", "name":
				name = strings.ToLower(strings.TrimSpace(attr.Val))
			case "content":
				content = strings.TrimSpace(attr.Val)
			case "href":
				href = strings.TrimSpace(attr.Val)
			}
		}
		if n.Data == "base" && href != "" {
			if b, err := url.Parse(href); err == nil {
				base = base.ResolveReference(b)
			}
		}
		if _, ok := found[name]; n.Data == "meta" && !ok && content != "" {
			found[name] = content
		}
	}
	for _, property := range pageImageProperties {
		src, ok := found[property]
		if !ok {
			continue
		}
		ref, err := url.Parse(src)
		if err != nil {
			continue
		}
		abs := base.ResolveReference(ref).String()
		if cells.URL(abs, false) == nil {
			return abs, nil
		}
	}
	return "", nil
}
