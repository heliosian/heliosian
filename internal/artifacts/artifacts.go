package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/calendar"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

const (
	appName        = "artifacts"
	documentsTab   = "Documents"
	Folder         = "artifacts"
	lexicalWeight  = 0.15
	readers        = 32
	KindNewsletter = "newsletter"
	KindList       = "list"
	KindPage       = "page"
	KindPortal     = "portal"
	KindGroup      = "group"
	// KindAnnouncement is mail of that shape already in the corpus; nothing
	// files more of it (docs/ask/artifacts.md).
	KindAnnouncement = "announcement"
)

var DocumentColumns = []string{"Key", "Title", "Date", "Author", "Kind", "Channel", "Source", "Chunks", "Object", PointsColumn, AudienceColumn, JudgedColumn}

// PointsColumn holds a school email's key points, one a line, written once
// the email is in (internal/keypoints) for Heliosian's Inbox
// widget; blank until then, and for everything else.
const PointsColumn = "Key Points"

// AudienceColumn is who a school email was written to, judged with its key
// points: Everyone, or the classrooms and grades it names ("Condors,
// Ospreys", "Grade 2, Grade 4", "Condors, Grade 6") - for
// mail the school sends through Veracross, which says nothing of whom it
// went to; blank until judged.
const AudienceColumn = "Audience"

// JudgedColumn is the day a school email's points and audience were
// written (YYYY-MM-DD), so the key points pass can read again the emails
// judged before its question last changed (keypoints.Revision); blank
// until judged, and for a row judged before the column was.
const JudgedColumn = "Judged"

// Everyone is the Audience of an email to the whole school.
const Everyone = "Everyone"

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
	// Points are the key points written for a document, by key, Audience
	// who it was judged to be written to (AudienceColumn), and Judged the
	// day that was (JudgedColumn).
	Points   map[string][]string
	Audience map[string]string
	Judged   map[string]string
}

type Objects interface {
	Get(name string) ([]byte, error)
}

type documents struct {
	objects  Objects
	embedder Embedder
	mu       sync.Mutex
	held     map[string]*Document
}

func (d *documents) hold(doc *Document) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.held[doc.Object()] = doc
}

func (d *documents) keep(m *Model) {
	held := map[string]*Document{}
	for _, doc := range m.Documents {
		held[doc.Object()] = doc
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.held = held
}

func (d *documents) build(_ context.Context, tables store.Tables) (*Model, error) {
	rows := tables[documentsTab]
	d.mu.Lock()
	held := maps.Clone(d.held)
	d.mu.Unlock()
	m := &Model{Documents: make([]*Document, len(rows)), Points: map[string][]string{}, Audience: map[string]string{}, Judged: map[string]string{}}
	for _, row := range rows {
		if judged := strings.TrimSpace(row[JudgedColumn]); judged != "" {
			m.Judged[row["Key"]] = judged
		}
		if points := splitPoints(row[PointsColumn]); len(points) > 0 {
			m.Points[row["Key"]] = points
		}
		if audience := strings.TrimSpace(row[AudienceColumn]); audience != "" {
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
	slots := make(chan struct{}, readers)
	for _, i := range wanted {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			doc, err := read(d.objects, rows[i], d.embedder)
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

type Cache struct {
	*store.Store[*Model]
	documents *documents
}

func NewCache(source data.Source, writer data.Writer, objects Objects, embedder Embedder, queue *store.Queue) (*Cache, error) {
	d := &documents{objects: objects, embedder: embedder, held: map[string]*Document{}}
	s, err := store.New(store.Spec[*Model]{
		App:   appName,
		Tabs:  []store.Tab{{Name: documentsTab, Columns: DocumentColumns, Key: []string{"Key"}}},
		Build: d.build,
		Loaded: func(model *Model, took time.Duration) {
			d.keep(model)
			oldest, newest := model.Span()
			slog.Info("loaded artifacts model", "documents", len(model.Documents), "fetched", model.Fetched, "chunks", model.Chunks(),
				"channels", len(model.Channels()), "oldest", oldest, "newest", newest, "took", took.Round(time.Millisecond))
		},
	}, source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s, documents: d}, nil
}

// splitPoints reads a Key Points cell: one point a line, blanks passed over.
func splitPoints(cell string) []string {
	out := []string{}
	for _, line := range strings.Split(cell, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// SetPoints writes a document's key points, who it was written to, and the
// day it was judged.
func (c *Cache) SetPoints(ctx context.Context, actor, key string, points []string, audience, judged string) error {
	return c.Commit(ctx, actor, store.Update(documentsTab, store.Row{"Key": key}, store.Row{PointsColumn: strings.Join(points, "\n"), AudienceColumn: audience, JudgedColumn: judged}))
}

// Classrooms are the classrooms and grades an Audience names, none for
// Everyone; the directory's grade names tell the two apart.
func Classrooms(audience string) []string {
	out := []string{}
	if audience == Everyone {
		return out
	}
	for _, name := range strings.Split(audience, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// School says a document is mail the school sent everyone or a whole class:
// the newsletter, an all-family or classroom list, an announcement - not the
// everyone chat, and not a Loop group's post, which only its members read.
func School(d *Document) bool {
	switch d.Kind {
	case KindNewsletter, KindAnnouncement:
		return true
	case KindList:
		return d.Channel != "chat" && d.Channel != "chat2"
	}
	return false
}

func (c *Cache) Hold(doc *Document) error {
	if err := doc.normalize(); err != nil {
		return err
	}
	c.documents.hold(doc)
	return nil
}
