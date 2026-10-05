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
	"strconv"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const mailPartsActor = "mail parts"

type MailParts struct {
	s      *Store
	queue  *store.Queue
	bucket *blob.Bucket
	failed map[string]bool
	poke   chan struct{}
}

type mailPart struct {
	body                      []byte
	hash, mime, name, content string
}

func NewMailParts(s *Store, queue *store.Queue, bucket *blob.Bucket) *MailParts {
	p := &MailParts{s: s, queue: queue, bucket: bucket, failed: map[string]bool{}, poke: make(chan struct{}, 1)}
	go p.run()
	queue.OnSwap(func() {
		select {
		case p.poke <- struct{}{}:
		default:
		}
	})
	return p
}

func (p *MailParts) run() {
	for range p.poke {
		for _, id := range p.pending() {
			start := time.Now()
			n, err := p.split(id)
			if err != nil {
				p.failed[id] = true
				slog.Error("mail parts: split", "document", id, "error", err)
				continue
			}
			slog.Info("mail parts: split", "document", id, "parts", n, "took", time.Since(start).Round(time.Millisecond))
		}
	}
}

func (p *MailParts) pending() []string {
	m := p.s.Model()
	contents := m.Table("CONTENT")
	out := []string{}
	for _, row := range m.Table("DOCUMENT").All() {
		if row["extracted"] != "" || row["content"] == "" || p.failed[row["id"]] {
			continue
		}
		if c, ok := contents.Get(row["content"]); ok && c["mime"] == "message/rfc822" {
			out = append(out, row["id"])
		}
	}
	return out
}

func readParts(raw []byte) ([]mailPart, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("not a mail message: %w", err)
	}
	parts := []mailPart{}
	err = mail.Parts(msg, func(part mail.Part) error {
		body, err := io.ReadAll(part.Body)
		if err != nil {
			return fmt.Errorf("part %d: %w", len(parts)+1, err)
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
		sum := sha256.Sum256(body)
		parts = append(parts, mailPart{body: body, hash: hex.EncodeToString(sum[:]), mime: mimeType, name: name, content: part.ContentID})
		return nil
	})
	return parts, err
}

func (p *MailParts) split(id string) (int, error) {
	ctx := context.Background()
	m := p.s.Model()
	doc, ok := m.Table("DOCUMENT").Get(id)
	if !ok || doc["extracted"] != "" {
		return 0, nil
	}
	content, ok := m.Table("CONTENT").Get(doc["content"])
	if !ok {
		return 0, fmt.Errorf("its content %s is missing", doc["content"])
	}
	raw, _, err := p.bucket.Get(ctx, content["blob"])
	if err != nil {
		return 0, err
	}
	parts, err := readParts(raw)
	if err != nil {
		return 0, err
	}
	stored := map[string]bool{}
	for _, part := range parts {
		if _, held := m.Table("CONTENT").Find(part.hash); held || stored[part.hash] {
			continue
		}
		if err := p.bucket.Put(ctx, contentFolder+"/"+part.hash, part.mime, part.body); err != nil {
			return 0, err
		}
		stored[part.hash] = true
	}
	orders := store.Order(make([]string, len(parts)))
	_, err = p.queue.Transact(ctx, access.System(mailPartsActor), func(tx *store.Tx) error {
		stage := func(e Edit) (string, error) {
			written, c, err := stageWrite(p.s, tx, e, mailPartsActor, nil, true, unchecked)
			if err != nil {
				return "", err
			}
			return written, fire(p.s, tx, c, mailPartsActor)
		}
		now, ok := p.s.In(tx).Table("DOCUMENT").Get(id)
		if !ok || now["extracted"] != "" {
			return nil
		}
		contents := map[string]string{}
		for i, part := range parts {
			if _, done := contents[part.hash]; !done {
				if held, ok := p.s.In(tx).Table("CONTENT").Find(part.hash); ok {
					contents[part.hash] = held["id"]
				} else {
					inserted, err := stage(Edit{Insert: "CONTENT", Row: map[string]any{"hash": part.hash, "blob": contentFolder + "/" + part.hash, "mime": part.mime, "size": strconv.Itoa(len(part.body))}})
					if err != nil {
						return err
					}
					contents[part.hash] = inserted
				}
			}
			row := map[string]any{"parent": id, "relation": "part", "order": orders[i], "content": contents[part.hash]}
			if part.name != "" {
				row["filename"] = part.name
			}
			if part.content != "" {
				row["content_id"] = part.content
			}
			if _, err := stage(Edit{Insert: "DOCUMENT", Row: row}); err != nil {
				return err
			}
		}
		_, err := stage(Edit{Set: id, Cells: map[string]any{"extracted": time.Now().In(School).Format(cells.StampFormat)}})
		return err
	})
	return len(parts), err
}
