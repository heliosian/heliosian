package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

const (
	DocumentsApp             = "artifacts"
	documentsTab             = "Documents"
	DocumentsFolder          = "artifacts"
	lexicalWeight            = 0.15
	documentReaders          = 32
	DocumentKindNewsletter   = "newsletter"
	DocumentKindList         = "list"
	DocumentKindPage         = "page"
	DocumentKindPortal       = "portal"
	DocumentKindGroup        = "group"
	DocumentKindAnnouncement = "announcement"
)

var DocumentColumns = []string{"Key", "Title", "Date", "Author", "Kind", "Channel", "Source", "Chunks", "Object", DocumentPointsColumn, DocumentAudienceColumn, DocumentJudgedColumn}

const DocumentPointsColumn = "Key Points"

const DocumentAudienceColumn = "Audience"

const DocumentJudgedColumn = "Judged"

const DocumentForEveryone = "Everyone"

var stopwords = map[string]bool{"the": true, "and": true, "for": true, "are": true, "was": true, "our": true, "you": true, "your": true, "with": true, "this": true, "that": true, "from": true, "what": true, "when": true, "where": true, "who": true, "how": true, "does": true, "did": true, "will": true, "about": true, "there": true, "have": true, "has": true, "any": true, "can": true, "is": true, "in": true, "on": true, "at": true, "to": true, "of": true, "an": true, "or": true, "be": true, "it": true, "my": true, "me": true, "we": true, "us": true, "do": true, "up": true, "so": true, "if": true, "as": true, "by": true, "its": true, "not": true, "tell": true, "know": true, "say": true, "said": true, "school": true, "helios": true}

type Document struct {
	Key      string          `json:"key"`
	Title    string          `json:"title"`
	Date     string          `json:"date"`
	Time     string          `json:"time,omitempty"`
	Author   string          `json:"author"`
	Kind     string          `json:"kind"`
	Channel  string          `json:"channel"`
	Source   string          `json:"source"`
	Model    string          `json:"model"`
	Markdown string          `json:"markdown"`
	Chunks   []DocumentChunk `json:"chunks"`
}

type DocumentChunk struct {
	Section string `json:"section"`
	Text    string `json:"text"`
	Vector  Vector `json:"vector,omitempty"`
}

func DocumentKey(messageID string) string {
	sum := sha256.Sum256([]byte(messageID))
	return hex.EncodeToString(sum[:])
}

func (d *Document) Object() string {
	return DocumentsFolder + "/" + d.ObjectFile()
}

func (d *Document) ObjectFile() string {
	return d.Key + "-" + d.fingerprint() + ".json"
}

func (d *Document) fingerprint() string {
	bare := *d
	bare.Chunks = []DocumentChunk{}
	for _, c := range d.Chunks {
		bare.Chunks = append(bare.Chunks, DocumentChunk{Section: c.Section, Text: c.Text})
	}
	encoded, err := json.Marshal(bare)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:8])
}

func (d *Document) URL() string {
	if d.Kind != DocumentKindPage && d.Kind != DocumentKindPortal {
		return ""
	}
	return d.Source
}

func (d *Document) Row() map[string]string {
	return map[string]string{
		"Key": d.Key, "Title": d.Title, "Date": d.Date, "Author": d.Author, "Kind": d.Kind,
		"Channel": d.Channel, "Source": d.Source, "Chunks": strconv.Itoa(len(d.Chunks)), "Object": d.Object(),
	}
}

func (d *Document) embedText(c DocumentChunk) string {
	date := d.Date
	if day, err := time.ParseInLocation(DateFormat, d.Date, Location); err == nil {
		date = day.Format("January 2, 2006")
	}
	head := d.Title + ", " + date
	if d.Channel != "" {
		head += " (" + d.Channel + ")"
	}
	if c.Section != "" {
		head += pathSeparator + c.Section
	}
	return head + "\n\n" + c.Text
}

func (d *Document) Embed(ctx context.Context, embedder *artifacts.Vertex) error {
	if embedder.Model() != d.Model {
		return fmt.Errorf("the document is for %s, the embedder is %s", d.Model, embedder.Model())
	}
	texts := []string{}
	for _, c := range d.Chunks {
		texts = append(texts, d.embedText(c))
	}
	vectors, err := embedder.Embed(ctx, texts, false)
	if err != nil {
		return err
	}
	if len(vectors) != len(d.Chunks) {
		return fmt.Errorf("%d chunks, %d vectors", len(d.Chunks), len(vectors))
	}
	for i := range d.Chunks {
		d.Chunks[i].Vector = vectors[i]
	}
	return nil
}

func (d *Document) normalize() error {
	if len(d.Chunks) == 0 {
		return fmt.Errorf("document %s has no chunks", d.Key)
	}
	for i := range d.Chunks {
		if len(d.Chunks[i].Vector) == 0 {
			return fmt.Errorf("document %s chunk %d has no vector", d.Key, i)
		}
		NormalizeVector(d.Chunks[i].Vector)
	}
	return nil
}

type Documents struct {
	Documents  []*Document
	Fetched    int
	Points     map[string][]string
	Audience   map[string]string
	Judged     map[string]string
	ids        map[string]*Document
	idOf       map[*Document]string
	passages   map[string]documentPassage
	passageIDs map[documentPassage]string
}

type documentObjects struct {
	objects  *blob.Bucket
	embedder *artifacts.Vertex
	idKey    []byte
	mu       sync.Mutex
	held     map[string]*Document
}

func (d *documentObjects) hold(doc *Document) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.held[doc.Object()] = doc
}

func (d *documentObjects) keep(m *Documents) {
	held := map[string]*Document{}
	for _, doc := range m.Documents {
		held[doc.Object()] = doc
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.held = held
}

func (d *documentObjects) build(ctx context.Context, tables store.Tables) (*Documents, error) {
	rows := tables[documentsTab]
	d.mu.Lock()
	held := maps.Clone(d.held)
	d.mu.Unlock()
	m := &Documents{Documents: make([]*Document, len(rows)), Points: map[string][]string{}, Audience: map[string]string{}, Judged: map[string]string{}}
	for _, row := range rows {
		if judged := strings.TrimSpace(row[DocumentJudgedColumn]); judged != "" {
			m.Judged[row["Key"]] = judged
		}
		if points := splitPoints(row[DocumentPointsColumn]); len(points) > 0 {
			m.Points[row["Key"]] = points
		}
		if audience := strings.TrimSpace(row[DocumentAudienceColumn]); audience != "" {
			m.Audience[row["Key"]] = audience
		}
	}
	wanted := []int{}
	for i, row := range rows {
		if doc, ok := held[row["Object"]]; ok && doc.Key == row["Key"] {
			m.Documents[i] = doc
			continue
		}
		wanted = append(wanted, i)
	}
	var mu sync.Mutex
	var first error
	var wg sync.WaitGroup
	slots := make(chan struct{}, documentReaders)
	for _, i := range wanted {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			doc, err := readDocument(ctx, d.objects, rows[i], d.embedder)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if first == nil {
					first = err
				}
				return
			}
			m.Documents[i] = doc
		}()
	}
	wg.Wait()
	if first != nil {
		return nil, first
	}
	d.mu.Lock()
	for _, i := range wanted {
		d.held[rows[i]["Object"]] = m.Documents[i]
	}
	d.mu.Unlock()
	m.Fetched = len(wanted)
	m.sort()
	m.index(d.idKey)
	return m, nil
}

func readDocument(ctx context.Context, objects *blob.Bucket, row map[string]string, embedder *artifacts.Vertex) (*Document, error) {
	raw, _, err := objects.Get(ctx, row["Object"])
	if err != nil {
		return nil, fmt.Errorf("document %s: %w", row["Key"], err)
	}
	doc := &Document{}
	if err := json.Unmarshal(raw, doc); err != nil {
		return nil, fmt.Errorf("document %s: %w", row["Key"], err)
	}
	if doc.Key != row["Key"] {
		return nil, fmt.Errorf("document %s: the object says it is %s", row["Key"], doc.Key)
	}
	if doc.Model != embedder.Model() {
		return nil, fmt.Errorf("document %s was embedded with %s, not %s; import it again", doc.Key, doc.Model, embedder.Model())
	}
	if err := doc.normalize(); err != nil {
		return nil, err
	}
	return doc, nil
}

func (m *Documents) sort() {
	sort.SliceStable(m.Documents, func(i, j int) bool {
		if m.Documents[i].Date != m.Documents[j].Date {
			return m.Documents[i].Date > m.Documents[j].Date
		}
		return m.Documents[i].Title < m.Documents[j].Title
	})
}

func (m *Documents) Document(key string) *Document {
	for _, d := range m.Documents {
		if d.Key == key {
			return d
		}
	}
	return nil
}

func (m *Documents) Chunks() int {
	n := 0
	for _, d := range m.Documents {
		n += len(d.Chunks)
	}
	return n
}

func (m *Documents) Channels() map[string]int {
	out := map[string]int{}
	for _, d := range m.Documents {
		out[d.Channel]++
	}
	return out
}

func (m *Documents) Span() (oldest, newest string) {
	if len(m.Documents) == 0 {
		return "", ""
	}
	return m.Documents[len(m.Documents)-1].Date, m.Documents[0].Date
}

type DocumentHit struct {
	Document *Document
	Index    int
	Score    float64
}

func (m *Documents) Search(vector []float32, query string, limit int, keep func(d *Document, chunk int) bool) []DocumentHit {
	NormalizeVector(vector)
	terms := []string{}
	for _, t := range wordTokens(query) {
		if !stopwords[t] && !slices.Contains(terms, t) {
			terms = append(terms, t)
		}
	}
	hits := []DocumentHit{}
	for _, d := range m.Documents {
		for i, c := range d.Chunks {
			if !keep(d, i) {
				continue
			}
			score := dotProduct(vector, c.Vector)
			if len(terms) > 0 {
				lower := strings.ToLower(c.Section + "\n" + c.Text)
				found := 0
				for _, t := range terms {
					if strings.Contains(lower, t) {
						found++
					}
				}
				score += lexicalWeight * float64(found) / float64(len(terms))
			}
			hits = append(hits, DocumentHit{Document: d, Index: i, Score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Document.Date > hits[j].Document.Date
	})
	out := []DocumentHit{}
	taken := []string{}
	for _, hit := range hits {
		if len(out) >= limit {
			break
		}
		words := plainWords(hit.Document.Chunks[hit.Index].Text)
		if quotes(words, taken) {
			continue
		}
		taken = append(taken, words)
		out = append(out, hit)
	}
	return out
}

func plainWords(text string) string {
	return strings.Join(wordTokens(text), " ")
}

func quotes(words string, taken []string) bool {
	if len(words) < 80 {
		return slices.Contains(taken, words)
	}
	for _, other := range taken {
		if strings.Contains(other, words) || (len(other) >= 80 && strings.Contains(words, other)) {
			return true
		}
	}
	return false
}

var documentsTabs = []store.Tab{{Name: documentsTab, Columns: DocumentColumns, Key: []string{"Key"}}}

func splitPoints(cell string) []string {
	out := []string{}
	for _, line := range strings.Split(cell, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func (s *Store) SetPoints(ctx context.Context, actor access.Actor, key string, points []string, audience, judged string) error {
	return s.Commit(ctx, actor, DocumentsApp, s.Model().Documents.setPoints(actor, key, points, audience, judged)...)
}

func AudienceClassrooms(audience string) []string {
	out := []string{}
	if audience == DocumentForEveryone {
		return out
	}
	for _, name := range strings.Split(audience, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func (d *Document) School() bool {
	switch d.Kind {
	case DocumentKindNewsletter, DocumentKindAnnouncement:
		return true
	case DocumentKindList:
		return d.Channel != "chat" && d.Channel != "chat2"
	}
	return false
}

func (s *Store) Hold(doc *Document) error {
	if err := doc.normalize(); err != nil {
		return err
	}
	s.documents.hold(doc)
	return nil
}
