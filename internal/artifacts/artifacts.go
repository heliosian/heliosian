// Package artifacts keeps the documents the community has been sent - the school's newsletters, everything its lists carried and its website's pages - as markdown in chunks with embeddings, for Helios Ask to search.
package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/calendar"
	"heliosian/internal/data"
)

const (
	appName          = "artifacts"
	documentsTab     = "Documents"
	Folder           = "artifacts"
	refreshInterval  = 5 * time.Minute
	lexicalWeight    = 0.15
	readers          = 32
	KindNewsletter   = "newsletter"
	KindList         = "list"
	KindAnnouncement = "announcement"
	KindPage         = "page"
	KindPortal       = "portal"
	KindGroup        = "group"
)

var DocumentColumns = []string{"Key", "Title", "Date", "Author", "Kind", "Channel", "Source", "Chunks", "Object"}

var stopwords = map[string]bool{"the": true, "and": true, "for": true, "are": true, "was": true, "our": true, "you": true, "your": true, "with": true, "this": true, "that": true, "from": true, "what": true, "when": true, "where": true, "who": true, "how": true, "does": true, "did": true, "will": true, "about": true, "there": true, "have": true, "has": true, "any": true, "can": true, "is": true, "in": true, "on": true, "at": true, "to": true, "of": true, "an": true, "or": true, "be": true, "it": true, "my": true, "me": true, "we": true, "us": true, "do": true, "up": true, "so": true, "if": true, "as": true, "by": true, "its": true, "not": true, "tell": true, "know": true, "say": true, "said": true, "school": true, "helios": true}

type Document struct {
	Key      string  `json:"key"`
	Title    string  `json:"title"`
	Date     string  `json:"date"`
	Author   string  `json:"author"`
	Kind     string  `json:"kind"`
	Channel  string  `json:"channel"`
	Source   string  `json:"source"`
	Model    string  `json:"model"`
	Markdown string  `json:"markdown"`
	Chunks   []Chunk `json:"chunks"`
}

type Chunk struct {
	Section string `json:"section"`
	Text    string `json:"text"`
	Vector  Vector `json:"vector,omitempty"`
}

func Key(messageID string) string {
	sum := sha256.Sum256([]byte(messageID))
	return hex.EncodeToString(sum[:])
}

func (d *Document) Object() string {
	return Folder + "/" + d.ObjectFile()
}

func (d *Document) ObjectFile() string {
	return d.Key + "-" + d.fingerprint() + ".json"
}

func (d *Document) fingerprint() string {
	bare := *d
	bare.Chunks = []Chunk{}
	for _, c := range d.Chunks {
		bare.Chunks = append(bare.Chunks, Chunk{Section: c.Section, Text: c.Text})
	}
	encoded, err := json.Marshal(bare)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:8])
}

func (d *Document) URL() string {
	if d.Kind != KindPage && d.Kind != KindPortal {
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

func (d *Document) embedText(c Chunk) string {
	when := d.Date
	if day, err := time.ParseInLocation(calendar.DateFormat, d.Date, calendar.Location); err == nil {
		when = day.Format("January 2, 2006")
	}
	head := d.Title + ", " + when
	if d.Channel != "" {
		head += " (" + d.Channel + ")"
	}
	if c.Section != "" {
		head += pathSeparator + c.Section
	}
	return head + "\n\n" + c.Text
}

func (d *Document) Embed(ctx context.Context, embedder Embedder) error {
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
		Normalize(d.Chunks[i].Vector)
	}
	return nil
}

type Model struct {
	Documents []*Document
	Fetched   int
}

type Objects interface {
	Get(name string) ([]byte, error)
}

func ReadRows(source data.Source) ([]map[string]string, error) {
	header, rows, err := source.Table(appName, documentsTab)
	if err != nil {
		return nil, err
	}
	if err := data.CheckColumns(documentsTab, header, DocumentColumns); err != nil {
		return nil, err
	}
	return rows, nil
}

func Load(source data.Source, objects Objects, embedder Embedder, previous *Model) (*Model, error) {
	rows, err := ReadRows(source)
	if err != nil {
		return nil, err
	}
	held := map[string]*Document{}
	if previous != nil {
		for _, doc := range previous.Documents {
			held[doc.Object()] = doc
		}
	}
	m := &Model{Documents: make([]*Document, len(rows))}
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
	slots := make(chan struct{}, readers)
	for _, i := range wanted {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			doc, err := read(objects, rows[i], embedder)
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
	m.Fetched = len(wanted)
	m.sort()
	return m, nil
}

func read(objects Objects, row map[string]string, embedder Embedder) (*Document, error) {
	raw, err := objects.Get(row["Object"])
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

func LoadDir(dir string, embedder Embedder) (*Model, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	m := &Model{Documents: []*Document{}}
	for _, file := range files {
		saved, err := ReadSaved(file)
		if err != nil {
			return nil, err
		}
		doc, err := saved.Build(NewResolver(), embedder.Model())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if err := doc.Embed(context.Background(), embedder); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if err := doc.normalize(); err != nil {
			return nil, err
		}
		m.Documents = append(m.Documents, doc)
	}
	m.sort()
	return m, nil
}

func (m *Model) sort() {
	sort.SliceStable(m.Documents, func(i, j int) bool {
		if m.Documents[i].Date != m.Documents[j].Date {
			return m.Documents[i].Date > m.Documents[j].Date
		}
		return m.Documents[i].Title < m.Documents[j].Title
	})
}

func (m *Model) Document(key string) *Document {
	for _, d := range m.Documents {
		if d.Key == key {
			return d
		}
	}
	return nil
}

func (m *Model) Chunks() int {
	n := 0
	for _, d := range m.Documents {
		n += len(d.Chunks)
	}
	return n
}

func (m *Model) Channels() map[string]int {
	out := map[string]int{}
	for _, d := range m.Documents {
		out[d.Channel]++
	}
	return out
}

func (m *Model) Span() (oldest, newest string) {
	if len(m.Documents) == 0 {
		return "", ""
	}
	return m.Documents[len(m.Documents)-1].Date, m.Documents[0].Date
}

func (m *Model) Where(keep func(*Document) bool) *Model {
	out := &Model{Documents: []*Document{}, Fetched: m.Fetched}
	for _, d := range m.Documents {
		if keep(d) {
			out.Documents = append(out.Documents, d)
		}
	}
	return out
}

func (m *Model) Between(since, until string) *Model {
	if since == "" && until == "" {
		return m
	}
	out := &Model{Documents: []*Document{}}
	for _, d := range m.Documents {
		if since != "" && d.Date < since {
			continue
		}
		if until != "" && d.Date > until {
			continue
		}
		out.Documents = append(out.Documents, d)
	}
	return out
}

type Hit struct {
	Document *Document
	Index    int
	Score    float64
}

func (m *Model) Search(vector []float32, query string, limit int) []Hit {
	Normalize(vector)
	terms := []string{}
	for _, t := range tokens(query) {
		if !stopwords[t] && !slices.Contains(terms, t) {
			terms = append(terms, t)
		}
	}
	hits := []Hit{}
	for _, d := range m.Documents {
		for i, c := range d.Chunks {
			score := dot(vector, c.Vector)
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
			hits = append(hits, Hit{Document: d, Index: i, Score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Document.Date > hits[j].Document.Date
	})
	out := []Hit{}
	taken := []string{}
	for _, hit := range hits {
		if len(out) >= limit {
			break
		}
		words := plain(hit.Document.Chunks[hit.Index].Text)
		if quotes(words, taken) {
			continue
		}
		taken = append(taken, words)
		out = append(out, hit)
	}
	return out
}

func plain(text string) string {
	return strings.Join(tokens(text), " ")
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

type Enqueuer interface {
	Add(func())
}

type Cache struct {
	load  func(previous *Model) (*Model, error)
	queue Enqueuer
	mu    sync.RWMutex
	model *Model
	edits int
}

func NewCache(load func(previous *Model) (*Model, error), queue Enqueuer) (*Cache, error) {
	c := &Cache{load: load, queue: queue}
	if err := c.refresh(); err != nil {
		return nil, err
	}
	go c.refreshLoop()
	return c, nil
}

func (c *Cache) refreshLoop() {
	for range time.Tick(refreshInterval) {
		c.queue.Add(func() {
			if err := c.refresh(); err != nil {
				slog.Error("artifacts model refresh", "error", err)
			}
		})
	}
}

func (c *Cache) refresh() error {
	start := time.Now()
	c.mu.RLock()
	before := c.edits
	c.mu.RUnlock()
	model, err := c.load(c.Model())
	if err != nil {
		return err
	}
	c.mu.Lock()
	stale := c.edits != before
	if !stale {
		c.model = model
	}
	c.mu.Unlock()
	if stale {
		slog.Info("artifacts model refresh skipped: edited while reading")
		return nil
	}
	oldest, newest := model.Span()
	slog.Info("loaded artifacts model", "documents", len(model.Documents), "fetched", model.Fetched, "chunks", model.Chunks(),
		"channels", len(model.Channels()), "oldest", oldest, "newest", newest, "took", time.Since(start).Round(time.Millisecond))
	return nil
}

func (c *Cache) add(doc *Document) {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := &Model{Documents: append(slices.Clone(c.model.Documents), doc), Fetched: c.model.Fetched}
	next.sort()
	c.model = next
	c.edits++
}

func (c *Cache) Model() *Model {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}
