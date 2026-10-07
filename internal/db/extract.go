package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	netmail "net/mail"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/tomarkdown"
)

const (
	extractActor   = "extract"
	pdfType        = "application/pdf"
	extractTimeout = 30 * time.Minute
)

type Extractor struct {
	s      *Store
	queue  *store.Queue
	bucket *blob.Bucket
	client anthropic.Client
	failed map[string]bool
	poke   chan struct{}
}

type extracted struct {
	relation                   string
	body                       []byte
	mime, name, contentID, url string
	title, skip                string
}

type extractFunc func(x *Extractor, ctx context.Context, m *Model, doc, content store.Row, raw []byte) ([]extracted, error)

var extractors = map[string]extractFunc{
	"message/rfc822": bytesOnly(mailParts),
	"text/html":      bytesOnly(htmlMarkdown),
	"text/plain":     bytesOnly(textMarkdown),
	// "application/pdf": (*Extractor).readPDF,
	// "image/jpeg":      (*Extractor).readImage,
	// "image/png":       (*Extractor).readImage,
	"image/gif": (*Extractor).readImage,
	"image/bmp": (*Extractor).readImage,
	// "image/webp":      (*Extractor).readImage,
}

func bytesOnly(read func([]byte) ([]extracted, error)) extractFunc {
	return func(_ *Extractor, _ context.Context, _ *Model, _, _ store.Row, raw []byte) ([]extracted, error) {
		return read(raw)
	}
}

func extractable(kind string) bool {
	_, ok := extractors[kind]
	return ok
}

func NewExtractor(s *Store, queue *store.Queue, bucket *blob.Bucket, anthropicKey string) *Extractor {
	x := &Extractor{s: s, queue: queue, bucket: bucket, client: anthropic.NewClient(option.WithAPIKey(anthropicKey), option.WithRequestTimeout(extractTimeout)), failed: map[string]bool{}, poke: make(chan struct{}, 1)}
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
		again := false
		for _, id := range x.pending() {
			start := time.Now()
			n, err := x.extract(id)
			if errors.Is(err, errAskAgain) {
				slog.Error("extract, to be asked again", "document", id, "error", err)
				time.Sleep(linkBackoff)
				again = true
				continue
			}
			if err != nil {
				x.failed[id] = true
				slog.Error("extract", "document", id, "error", err)
				continue
			}
			slog.Info("extract: made", "document", id, "children", n, "took", time.Since(start).Round(time.Millisecond))
		}
		if !again {
			continue
		}
		select {
		case x.poke <- struct{}{}:
		default:
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
		if extractable(baseType(c["mime"])) {
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
	images, err := htmlImages(raw)
	if err != nil {
		return nil, err
	}
	return append(markdownExtract(tomarkdown.Trim(markdown)), images...), nil
}

func textMarkdown(raw []byte) ([]extracted, error) {
	return markdownExtract(tomarkdown.Trim(tomarkdown.Text(string(raw)))), nil
}

func (m *Model) htmlAlongside(doc store.Row) bool {
	if doc["relation"] != "part" {
		return false
	}
	contents := m.Table("CONTENT")
	return slices.ContainsFunc(m.Table("DOCUMENT").Referencing("parent", doc["parent"]), func(sibling store.Row) bool {
		c, _ := contents.Get(sibling["content"])
		return sibling["relation"] == "part" && baseType(c["mime"]) == "text/html"
	})
}

func stager(s *Store, tx *store.Tx, actor string) func(Edit) (string, error) {
	return func(e Edit) (string, error) {
		written, c, err := stageWrite(s, tx, e, actor, nil, true, unchecked)
		if err != nil {
			return "", err
		}
		return written, fire(s, tx, c, actor)
	}
}

func stageContent(s *Store, tx *store.Tx, stage func(Edit) (string, error), held map[string]string, hash, mimeType string, size int) (string, error) {
	key := hash + "\x00" + mimeType
	if id, ok := held[key]; ok {
		return id, nil
	}
	if row, ok := s.In(tx).Table("CONTENT").Find(hash, mimeType); ok {
		held[key] = row["id"]
		return row["id"], nil
	}
	id, err := stage(Edit{Insert: "CONTENT", Row: map[string]any{"hash": hash, "blob": contentFolder + "/" + hash, "mime": mimeType, "size": strconv.Itoa(size)}})
	if err != nil {
		return "", err
	}
	held[key] = id
	return id, nil
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
	kind := baseType(content["mime"])
	read, ok := extractors[kind]
	if !ok {
		return 0, nil
	}
	children := []extracted{}
	var raw []byte
	if kind != "text/plain" || !m.htmlAlongside(doc) {
		var err error
		if raw, _, err = x.bucket.Get(ctx, content["blob"]); err != nil {
			return 0, err
		}
		if children, err = read(x, ctx, m, doc, content, raw); err != nil {
			return 0, err
		}
	}
	if doc["relation"] != "part" {
		children = slices.DeleteFunc(children, func(child extracted) bool { return child.relation == "image" })
	}
	if root, mail := m.mailRootOf(doc); mail && doc["relation"] == "part" && baseType(content["mime"]) == "text/html" {
		links, err := ChooseLinks(ctx, x.client, LinkEmail{Subject: root["name"], Kind: root["kind"], Sent: root["published"]}, raw)
		if err != nil {
			return 0, err
		}
		for _, l := range links {
			children = append(children, extracted{relation: "linked", url: l.URL, title: l.Text, skip: l.Skip})
		}
	}
	hashes := make([]string, len(children))
	stored := map[string]bool{}
	for i, child := range children {
		if child.body == nil {
			continue
		}
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
	committed := false
	_, err := x.queue.Transact(ctx, access.System(extractActor), func(tx *store.Tx) error {
		stage := stager(x.s, tx, extractActor)
		now, ok := x.s.In(tx).Table("DOCUMENT").Get(id)
		if !ok || now["extracted"] != "" {
			return nil
		}
		contents := map[string]string{}
		for i, child := range children {
			row := map[string]any{"parent": id, "relation": child.relation, "order": orders[i]}
			if child.body != nil {
				content, err := stageContent(x.s, tx, stage, contents, hashes[i], child.mime, len(child.body))
				if err != nil {
					return err
				}
				row["content"] = content
			}
			if child.name != "" {
				row["filename"] = child.name
			}
			if child.contentID != "" {
				row["content_id"] = child.contentID
			}
			if child.url != "" {
				row["url"] = child.url
			}
			if child.title != "" {
				row["name"] = child.title
			}
			if child.skip != "" {
				row["fetch"], row["link"] = "skipped", child.skip
			}
			if _, err := stage(Edit{Insert: "DOCUMENT", Row: row}); err != nil {
				return err
			}
		}
		_, err := stage(Edit{Set: id, Cells: map[string]any{"extracted": time.Now().In(School).Format(cells.StampFormat)}})
		committed = err == nil
		return err
	})
	if err != nil || !committed {
		names := []string{}
		for hash := range stored {
			names = append(names, contentFolder+"/"+hash)
		}
		dropUnheld(x.s, x.bucket, names)
	}
	return len(children), err
}
