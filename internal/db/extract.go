package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	netmail "net/mail"
	"net/url"
	"strconv"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/tomarkdown"
)

const extractActor = "extract"

type Extractor struct {
	s      *Store
	queue  *store.Queue
	bucket *blob.Bucket
	failed map[string]bool
	poke   chan struct{}
}

type extracted struct {
	relation              string
	body                  []byte
	mime, name, contentID string
}

var extractors = map[string]func([]byte) ([]extracted, error){
	"message/rfc822": mailParts,
	"text/html":      htmlMarkdown,
	"text/plain":     textMarkdown,
}

func NewExtractor(s *Store, queue *store.Queue, bucket *blob.Bucket) *Extractor {
	x := &Extractor{s: s, queue: queue, bucket: bucket, failed: map[string]bool{}, poke: make(chan struct{}, 1)}
	go x.run()
	queue.OnSwap(func() {
		select {
		case x.poke <- struct{}{}:
		default:
		}
	})
	return x
}

func (x *Extractor) run() {
	for range x.poke {
		for _, id := range x.pending() {
			start := time.Now()
			n, err := x.extract(id)
			if err != nil {
				x.failed[id] = true
				slog.Error("extract", "document", id, "error", err)
				continue
			}
			slog.Info("extract: made", "document", id, "children", n, "took", time.Since(start).Round(time.Millisecond))
		}
	}
}

func baseType(mimeType string) string {
	base, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return mimeType
	}
	return base
}

func (x *Extractor) pending() []string {
	m := x.s.Model()
	contents := m.Table("CONTENT")
	out := []string{}
	for _, row := range m.Table("DOCUMENT").All() {
		if row["extracted"] != "" || row["content"] == "" || x.failed[row["id"]] {
			continue
		}
		c, ok := contents.Get(row["content"])
		if !ok {
			continue
		}
		if _, ok := extractors[baseType(c["mime"])]; ok {
			out = append(out, row["id"])
		}
	}
	return out
}

func mailParts(raw []byte) ([]extracted, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("not a mail message: %w", err)
	}
	out := []extracted{}
	err = mail.Parts(msg, func(part mail.Part) error {
		body, err := io.ReadAll(part.Body)
		if err != nil {
			return fmt.Errorf("part %d: %w", len(out)+1, err)
		}
		name, err := new(mime.WordDecoder).DecodeHeader(part.Name)
		if err != nil {
			name = part.Name
		}
		mimeType := http.DetectContentType(body)
		if part.MediaType == "message/rfc822" {
			if _, err := netmail.ReadMessage(bytes.NewReader(body)); err == nil {
				mimeType = "message/rfc822"
			}
		}
		out = append(out, extracted{relation: "part", body: body, mime: mimeType, name: name, contentID: part.ContentID})
		return nil
	})
	return out, err
}

func markdownExtract(markdown string) []extracted {
	if markdown == "" {
		return nil
	}
	return []extracted{{relation: "extract", body: []byte(markdown), mime: "text/markdown"}}
}

func htmlMarkdown(raw []byte) ([]extracted, error) {
	markdown, err := tomarkdown.HTML(string(raw), (&tomarkdown.LinkResolver{}).Links(&url.URL{}))
	if err != nil {
		return nil, err
	}
	return markdownExtract(tomarkdown.Trim(markdown)), nil
}

func textMarkdown(raw []byte) ([]extracted, error) {
	return markdownExtract(tomarkdown.Trim(tomarkdown.Text(string(raw)))), nil
}

func (x *Extractor) extract(id string) (int, error) {
	ctx := context.Background()
	m := x.s.Model()
	doc, ok := m.Table("DOCUMENT").Get(id)
	if !ok || doc["extracted"] != "" {
		return 0, nil
	}
	content, ok := m.Table("CONTENT").Get(doc["content"])
	if !ok {
		return 0, fmt.Errorf("its content %s is missing", doc["content"])
	}
	read, ok := extractors[baseType(content["mime"])]
	if !ok {
		return 0, nil
	}
	raw, _, err := x.bucket.Get(ctx, content["blob"])
	if err != nil {
		return 0, err
	}
	children, err := read(raw)
	if err != nil {
		return 0, err
	}
	hashes := make([]string, len(children))
	stored := map[string]bool{}
	for i, child := range children {
		sum := sha256.Sum256(child.body)
		hashes[i] = hex.EncodeToString(sum[:])
		if _, held := m.Table("CONTENT").Find(hashes[i], child.mime); held || stored[hashes[i]] {
			continue
		}
		if err := x.bucket.Put(ctx, contentFolder+"/"+hashes[i], child.mime, child.body); err != nil {
			return 0, err
		}
		stored[hashes[i]] = true
	}
	orders := store.Order(make([]string, len(children)))
	_, err = x.queue.Transact(ctx, access.System(extractActor), func(tx *store.Tx) error {
		stage := func(e Edit) (string, error) {
			written, c, err := stageWrite(x.s, tx, e, extractActor, nil, true, unchecked)
			if err != nil {
				return "", err
			}
			return written, fire(x.s, tx, c, extractActor)
		}
		now, ok := x.s.In(tx).Table("DOCUMENT").Get(id)
		if !ok || now["extracted"] != "" {
			return nil
		}
		contents := map[string]string{}
		for i, child := range children {
			hash, key := hashes[i], hashes[i]+"\x00"+child.mime
			if _, done := contents[key]; !done {
				if held, ok := x.s.In(tx).Table("CONTENT").Find(hash, child.mime); ok {
					contents[key] = held["id"]
				} else {
					inserted, err := stage(Edit{Insert: "CONTENT", Row: map[string]any{"hash": hash, "blob": contentFolder + "/" + hash, "mime": child.mime, "size": strconv.Itoa(len(child.body))}})
					if err != nil {
						return err
					}
					contents[key] = inserted
				}
			}
			row := map[string]any{"parent": id, "relation": child.relation, "order": orders[i], "content": contents[key]}
			if child.name != "" {
				row["filename"] = child.name
			}
			if child.contentID != "" {
				row["content_id"] = child.contentID
			}
			if _, err := stage(Edit{Insert: "DOCUMENT", Row: row}); err != nil {
				return err
			}
		}
		_, err := stage(Edit{Set: id, Cells: map[string]any{"extracted": time.Now().In(School).Format(cells.StampFormat)}})
		return err
	})
	return len(children), err
}
