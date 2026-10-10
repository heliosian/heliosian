package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	searchFolder   = "search"
	SearchLimit    = 10
	searchRefs     = 5
	searchFloor    = 0.67
	searchChunk    = 1500
	searchMakers   = 4
	searchLoaders  = 128
	searchTimeout  = 2 * time.Minute
	searchAttempts = 3
	searchShortest = 10
)

var searchTables = []string{"GROUP", "PERSON", "DOCUMENT"}

var searchExclusions = []string{
	`(from GROUP (where (or (in id (select GROUP.rsvp_yes)) (in id (select GROUP.rsvp_no)))))`,
	`(from GROUP (where (= kind "event") (= parent.kind "event")))`,
	`(from GROUP (where (in kind "day_part" "day" "category")))`,
	`(from GROUP (where (in id (select GROUP.waitlist))))`,
	`(from GROUP (where (= kind "admins")))`,
	`(from GROUP (where (in id (select GROUP.managed_by)) (!= kind "family")))`,
}

var searchFiller = map[string]bool{
	"an": true, "and": true, "are": true, "as": true, "at": true, "be": true, "by": true, "did": true, "do": true, "does": true,
	"for": true, "from": true, "had": true, "has": true, "have": true, "how": true, "in": true, "is": true, "it": true, "its": true,
	"of": true, "on": true, "or": true, "that": true, "the": true, "their": true, "them": true, "they": true, "this": true, "to": true,
	"was": true, "were": true, "what": true, "when": true, "where": true, "which": true, "who": true, "whom": true, "whose": true, "why": true,
	"will": true, "with": true,
}

var terminalNames = map[string]struct{ label, of, published string }{
	"mail":     {"Email", "email", "Sent"},
	"file":     {"File", "file", "Updated"},
	"calendar": {"Calendar", "calendar", "Updated"},
	"wiki":     {"Wiki page", "wiki page", "Updated"},
}

const searchSystem = `You write the search entry for one thing in Helios, the apps of a small K-8 school community: a person, a group - a family, a classroom, an event, a volunteer activity, a party, an email list, a category and the like - or the text of something the community received or shared - an email's body, a file attached to it, an image it shows, a page it links to, or a shared file, year calendar or wiki page itself - read as text and headed by where it came from. You are given what everyone who can see it can read. Search shows its summary under its name in a list of results.`

var searchSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"summary": map[string]any{
			"type":        "string",
			"description": "One or two plain sentences saying what it is. Say only what the text says, never what it leaves out: no \"no further details are given\". Never comment on the text itself: not what it gives, lacks or appears to be. Write one however little the text says, saying what that little is.",
		},
	},
	"required":             []string{"summary"},
	"additionalProperties": false,
}

func SearchSummaryRequest(input string) anthropic.MessageNewParams {
	return anthropic.MessageNewParams{
		Model:        claude.SearchSummaryModel,
		MaxTokens:    32000,
		System:       []anthropic.TextBlockParam{{Text: searchSystem}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(input))},
		OutputConfig: anthropic.OutputConfigParam{Effort: claude.SearchSummaryEffort, Format: anthropic.JSONOutputFormatParam{Schema: searchSchema}},
	}
}

type SearchEntry struct {
	Summary     string        `json:"summary"`
	Chunks      []SearchChunk `json:"chunks"`
	Fingerprint []uint32      `json:"fingerprint,omitempty"`
	Failures    int           `json:"failures,omitempty"`
}

type SearchChunk struct {
	Text   string    `json:"text"`
	Vector []float32 `json:"vector"`
}

type SearchRef struct {
	Extract  string  `json:"extract"`
	Document string  `json:"document"`
	Source   string  `json:"source,omitempty"`
	Href     string  `json:"href,omitempty"`
	Terminal string  `json:"terminal"`
	Name     string  `json:"name"`
	Summary  string  `json:"summary"`
	Score    float64 `json:"score,omitempty"`
}

type SearchResult struct {
	ID      string      `json:"id,omitempty"`
	Name    string      `json:"name,omitempty"`
	Href    string      `json:"href,omitempty"`
	Summary string      `json:"summary,omitempty"`
	Score   float64     `json:"score,omitempty"`
	Refs    []SearchRef `json:"refs,omitempty"`
}

type SearchRow struct {
	Table, Head, Input, Object, Name, Href, When string
	Source, SourceLine, SourceHref, Terminal     string
	NameOnly                                     bool
	named, source                                store.Row
}

func nameOnly(input string) bool {
	for _, line := range strings.Split(input, "\n")[1:] {
		if !strings.HasPrefix(line, "Pronouns: ") {
			return false
		}
	}
	return true
}

func (r SearchRow) body() string {
	if r.Head == "" {
		return r.Input
	}
	return strings.TrimPrefix(r.Input, r.Head+"\n\n")
}

type SearchIndex struct {
	mu      sync.RWMutex
	rows    map[string]SearchRow
	entries map[string]*SearchEntry
	missing map[string]bool
	dropped map[string]bool
	view    *searchView
	version int
	poke    chan struct{}
	reindex chan struct{}
}

func NewSearchIndex() *SearchIndex {
	return &SearchIndex{rows: map[string]SearchRow{}, entries: map[string]*SearchEntry{}, missing: map[string]bool{}, dropped: map[string]bool{}, view: emptyView(), poke: make(chan struct{}, 1), reindex: make(chan struct{}, 1)}
}

func (x *SearchIndex) drop(object string) {
	x.mu.Lock()
	delete(x.entries, object)
	x.dropped[object] = true
	x.version++
	x.mu.Unlock()
	select {
	case x.poke <- struct{}{}:
	default:
	}
}

func (x *SearchIndex) signal() {
	select {
	case x.reindex <- struct{}{}:
	default:
	}
}

func (x *SearchIndex) current() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.version
}

func (x *SearchIndex) generatedRows() ([]store.Row, int) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	out := []store.Row{}
	for _, id := range slices.Sorted(maps.Keys(x.rows)) {
		r := x.rows[id]
		row := store.Row{"id": Derive(SearchPrefix, id), "target": id, "input": r.Input, "object": r.Object, "made": "No", "source": r.Source, "terminal": r.Terminal}
		if e := x.entries[r.Object]; e != nil {
			row["summary"], row["chunks"], row["failures"], row["made"] = e.Summary, strconv.Itoa(len(e.Chunks)), strconv.Itoa(e.Failures), "Yes"
		}
		out = append(out, row)
	}
	return out, x.version
}

type Searcher struct {
	*SearchIndex
	s       *Store
	bucket  *blob.Bucket
	vertex  *artifacts.Vertex
	origin  func(app string) string
	client  *anthropic.Client
	texts   map[string]string
	pending []string
	waiting map[string]bool
	swept   bool
	wake    chan struct{}
}

func NewSearcher(s *Store, queue *store.Queue, bucket *blob.Bucket, vertex *artifacts.Vertex, origin func(app string) string) *Searcher {
	x := &Searcher{SearchIndex: s.Model().index, s: s, bucket: bucket, vertex: vertex, origin: origin, texts: map[string]string{}, waiting: map[string]bool{}, wake: make(chan struct{}, 1)}
	go x.follow()
	go x.index()
	queue.OnSwap(func() {
		select {
		case x.poke <- struct{}{}:
		default:
		}
	})
	return x
}

func (x *Searcher) StartMaking(anthropicKey string) {
	client := anthropic.NewClient(option.WithAPIKey(anthropicKey))
	x.mu.Lock()
	x.client = &client
	x.mu.Unlock()
	for range searchMakers {
		go x.make()
	}
	select {
	case x.poke <- struct{}{}:
	default:
	}
}

func SearchObject(input string) string {
	sum := sha256.Sum256([]byte(input))
	return searchFolder + "/" + hex.EncodeToString(sum[:]) + ".json"
}

func (m *Model) searchExcluded() map[string]bool {
	out := map[string]bool{}
	for _, src := range searchExclusions {
		q, err := Parse(src)
		if err != nil {
			panic(fmt.Sprintf("search exclusion %s: %v", src, err))
		}
		for _, id := range m.Run(context.Background(), q, Env{System: "search", Now: time.Now()}).IDs {
			out[id] = true
		}
	}
	return out
}

func (m *Model) SearchInputs(texts map[string]string) map[string]SearchRow {
	out := map[string]SearchRow{}
	excluded := m.searchExcluded()
	for _, table := range []string{"GROUP", "PERSON"} {
		for _, row := range m.Shown(table).All() {
			input := m.searchInput(table, row)
			if input == "" || excluded[row["id"]] {
				continue
			}
			name, when := row["name"], row["start"]
			if table == "PERSON" {
				name, when = row["name_show"], ""
			}
			out[row["id"]] = SearchRow{Table: table, Input: input, Object: SearchObject(input), Name: name, When: when, NameOnly: table == "PERSON" && nameOnly(input), named: row}
		}
	}
	docs := m.Shown("DOCUMENT")
	for _, row := range m.markdownDocuments() {
		text, ok := texts[row["content"]]
		if !ok {
			continue
		}
		terminal, source := row, row
		if row["kind"] == "" {
			terminal, _ = docs.Get(row["terminal"])
			source, _ = docs.Get(row["source"])
		}
		line := m.sourceLine(source, terminal)
		head := m.extractHead(terminal, line)
		input := head + "\n\n" + strings.TrimSpace(text)
		name := terminal["name"]
		if name == "" {
			name = terminal["id"]
		}
		out[row["id"]] = SearchRow{Table: "DOCUMENT", Head: head, Input: input, Object: SearchObject(input), Name: name, When: terminal["published"], Source: source["id"], SourceLine: line, Terminal: terminal["id"], named: terminal, source: source}
	}
	return out
}

func (m *Model) sourceLine(source, terminal store.Row) string {
	if source["id"] == terminal["id"] {
		return ""
	}
	of := "the " + terminalNames[terminal["kind"]].of
	content, _ := m.Shown("CONTENT").Get(source["content"])
	mime := baseType(content["mime"])
	parts := []string{}
	switch source["relation"] {
	case "part":
		switch {
		case source["filename"] != "":
			parts = append(parts, "attached file", mime, fmt.Sprintf("filename %q", source["filename"]))
		case strings.HasPrefix(mime, "text/"):
			parts = append(parts, "email body", mime)
		default:
			parts = append(parts, "embedded in "+of, mime)
		}
	case "image":
		parts = append(parts, "image shown in "+of, mime)
	case "linked":
		parts = append(parts, "linked from "+of, mime)
		if source["name"] != "" {
			parts = append(parts, fmt.Sprintf("link text %q", source["name"]))
		}
		parts = append(parts, source["url"])
	default:
		panic(fmt.Sprintf("search: %s is the source of an extract with relation %q", source["id"], source["relation"]))
	}
	return strings.Join(slices.DeleteFunc(parts, func(p string) bool { return p == "" }), ", ")
}

func (m *Model) markdownDocuments() []store.Row {
	contents := m.Shown("CONTENT")
	out := []store.Row{}
	for _, row := range m.Shown("DOCUMENT").All() {
		if row["relation"] != "extract" && row["kind"] != "wiki" {
			continue
		}
		if c, ok := contents.Get(row["content"]); ok && baseType(c["mime"]) == "text/markdown" {
			out = append(out, row)
		}
	}
	return out
}

func SearchTexts(ctx context.Context, m *Model, bucket *blob.Bucket, held map[string]string) (map[string]string, error) {
	contents := m.Shown("CONTENT")
	out := map[string]string{}
	missing := []store.Row{}
	for _, row := range m.markdownDocuments() {
		id := row["content"]
		if _, done := out[id]; done {
			continue
		}
		if text, ok := held[id]; ok {
			out[id] = text
			continue
		}
		c, _ := contents.Get(id)
		out[id] = ""
		missing = append(missing, c)
	}
	texts, errs := loadAll(len(missing), func(i int) (string, error) {
		raw, _, err := bucket.Get(ctx, missing[i]["blob"])
		return string(raw), err
	})
	for i, c := range missing {
		if errs[i] != nil {
			return nil, fmt.Errorf("read %s: %w", c["blob"], errs[i])
		}
		out[c["id"]] = texts[i]
	}
	return out, nil
}

func (m *Model) extractHead(terminal store.Row, source string) string {
	lines := []string{}
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			lines = append(lines, label+": "+value)
		}
	}
	names := terminalNames[terminal["kind"]]
	add(names.label, terminal["name"])
	add("Kind", terminal["kind"])
	add(names.published, terminal["published"])
	if author, ok := m.Shown("PERSON").Get(terminal["author"]); ok {
		add("From", author["name_show"])
	}
	add("Source", source)
	return strings.Join(lines, "\n")
}

func (m *Model) searchInput(table string, row store.Row) string {
	lines := []string{}
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value == "" {
			return
		}
		if label == "" {
			lines = append(lines, value)
			return
		}
		lines = append(lines, label+": "+value)
	}
	groups := m.Shown("GROUP")
	name := func(id string) string {
		g, _ := groups.Get(id)
		return g["name"]
	}
	switch table {
	case "GROUP":
		add("", row["name"])
		add("", row["subtitle"])
		add("Kind", row["kind"])
		above := []string{}
		for id, seen := row["parent"], map[string]bool{}; id != "" && !seen[id]; {
			seen[id] = true
			g, ok := groups.Get(id)
			if !ok {
				break
			}
			above = append([]string{g["name"]}, above...)
			id = g["parent"]
		}
		add("Under", strings.Join(above, " › "))
		add("When", strings.TrimSpace(row["start"]+" "+row["end"]))
		add("Timing", row["timing"])
		add("Location", row["location"])
		add("", row["description"])
	case "PERSON":
		add("", row["name_show"])
		add("Pronouns", row["pronouns"])
		add("Grade", row["grade"])
		add("Classroom", name(row["classroom"]))
		add("Crew", name(row["crew"]))
		add("Department", name(row["department"]))
		add("Job", row["job_title"])
		add("", row["facts"])
		add("", row["vc_bio"])
	}
	return strings.Join(lines, "\n")
}

func (x *Searcher) follow() {
	for range x.poke {
		start := time.Now()
		x.mu.Lock()
		dropped := slices.Collect(maps.Keys(x.dropped))
		clear(x.dropped)
		x.mu.Unlock()
		for _, hash := range dropped {
			if err := x.bucket.Remove(context.Background(), hash); err != nil && !errors.Is(err, blob.ErrNotFound) {
				slog.Error("search: remove a deleted entry", "object", hash, "error", err)
			}
		}
		if len(dropped) > 0 {
			slog.Info("search: removed deleted entries", "objects", len(dropped))
		}
		m := x.s.Model()
		texts, err := SearchTexts(context.Background(), m, x.bucket, x.texts)
		if err != nil {
			slog.Error("search: read the extracts", "error", err)
			continue
		}
		x.texts = texts
		rows := m.SearchInputs(texts)
		for id, r := range rows {
			r.Href = m.Link(r.Table, r.named, x.origin)
			if r.source != nil {
				r.SourceHref = m.Link("DOCUMENT", r.source, x.origin)
			}
			rows[id] = r
		}
		x.mu.Lock()
		wanted := map[string]bool{}
		unknown := map[string]bool{}
		for _, r := range rows {
			wanted[r.Object] = true
			if x.entries[r.Object] == nil && !x.missing[r.Object] && !x.dropped[r.Object] {
				unknown[r.Object] = true
			}
		}
		removed := map[string]bool{}
		for _, r := range x.rows {
			if x.client != nil && !wanted[r.Object] {
				removed[r.Object] = true
			}
		}
		sweep := x.client != nil && !x.swept
		x.swept = x.swept || sweep
		x.rows = rows
		x.version++
		for hash := range x.entries {
			if !wanted[hash] {
				delete(x.entries, hash)
			}
		}
		for hash := range x.missing {
			if !wanted[hash] {
				delete(x.missing, hash)
			}
		}
		x.mu.Unlock()
		if sweep {
			x.strays(wanted, removed)
		}
		if len(removed) > 0 {
			slog.Info("search: remove the unwanted entries", "objects", len(removed))
		}
		for _, hash := range slices.Sorted(maps.Keys(removed)) {
			if err := x.bucket.Remove(context.Background(), hash); err != nil {
				slog.Error("search: remove", "object", hash, "error", err)
			}
		}
		toLoad := slices.Sorted(maps.Keys(unknown))
		loaded, errs := loadAll(len(toLoad), func(i int) (*SearchEntry, error) { return x.load(toLoad[i]) })
		x.mu.Lock()
		for i, hash := range toLoad {
			if errs[i] != nil {
				slog.Error("search: load", "object", hash, "error", errs[i])
				continue
			}
			if loaded[i] != nil {
				x.entries[hash] = loaded[i]
				x.version++
				continue
			}
			x.missing[hash] = true
		}
		missing := len(x.missing)
		if x.client != nil {
			for _, hash := range slices.Sorted(maps.Keys(wanted)) {
				if e := x.entries[hash]; e == nil || e.unfinished() {
					x.enqueue(hash)
				}
			}
		}
		x.mu.Unlock()
		if len(unknown) > 0 {
			slog.Info("search: read the index", "rows", len(rows), "loaded", len(unknown), "missing", missing, "took", time.Since(start).Round(time.Millisecond))
		}
		x.signal()
	}
}

func (x *Searcher) strays(wanted, removed map[string]bool) {
	held, err := x.bucket.List(context.Background(), searchFolder+"/")
	if err != nil {
		slog.Error("search: list the entries", "error", err)
		x.mu.Lock()
		x.swept = false
		x.mu.Unlock()
		return
	}
	for _, hash := range held {
		if !wanted[hash] {
			removed[hash] = true
		}
	}
}

func (x *Searcher) index() {
	vectors := map[string]*vectorIndex{}
	for _, t := range searchTables {
		vectors[t] = &vectorIndex{}
	}
	for range x.reindex {
		start := time.Now()
		x.mu.RLock()
		rows := x.rows
		entries := map[string]*SearchEntry{}
		byTable := map[string]map[string]*SearchEntry{}
		for _, t := range searchTables {
			byTable[t] = map[string]*SearchEntry{}
		}
		for _, r := range rows {
			if e := x.entries[r.Object]; e != nil {
				entries[r.Object] = e
				if !r.NameOnly {
					byTable[r.Table][r.Object] = e
				}
			}
		}
		x.mu.RUnlock()
		next := map[string]*vectorIndex{}
		cells := 0
		for _, t := range searchTables {
			next[t] = vectors[t].update(byTable[t])
			cells += len(next[t].centroids)
		}
		vectors = next
		view := buildView(rows, entries, vectors)
		x.mu.Lock()
		x.view = view
		x.mu.Unlock()
		slog.Info("search: indexed", "entries", len(entries), "names", len(view.names), "cells", cells, "groups", len(view.members), "took", time.Since(start).Round(time.Millisecond))
	}
}

func loadAll[T any](n int, f func(i int) (T, error)) ([]T, []error) {
	results := make([]T, n)
	errs := make([]error, n)
	next := make(chan int)
	var wg sync.WaitGroup
	for range searchLoaders {
		wg.Go(func() {
			for i := range next {
				results[i], errs[i] = f(i)
			}
		})
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()
	return results, errs
}

func (x *Searcher) load(hash string) (*SearchEntry, error) {
	raw, _, err := x.bucket.Get(context.Background(), hash)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entry := &SearchEntry{}
	if err := json.Unmarshal(raw, entry); err != nil {
		return nil, fmt.Errorf("read %s: %w", hash, err)
	}
	return entry, nil
}

func (x *Searcher) enqueue(hash string) {
	if x.waiting[hash] {
		return
	}
	x.waiting[hash] = true
	x.pending = append(x.pending, hash)
	select {
	case x.wake <- struct{}{}:
	default:
	}
}

func (x *Searcher) next() (SearchRow, bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	for len(x.pending) > 0 {
		hash := x.pending[0]
		x.pending = x.pending[1:]
		delete(x.waiting, hash)
		if e := x.entries[hash]; !x.missing[hash] && e != nil && !e.unfinished() {
			continue
		}
		for _, r := range x.rows {
			if r.Object != hash {
				continue
			}
			if len(x.pending) > 0 {
				select {
				case x.wake <- struct{}{}:
				default:
				}
			}
			return r, true
		}
	}
	return SearchRow{}, false
}

func (x *Searcher) make() {
	for {
		row, ok := x.next()
		if !ok {
			<-x.wake
			continue
		}
		hash := row.Object
		start := time.Now()
		x.mu.Lock()
		was := x.entries[hash]
		x.mu.Unlock()
		if was == nil {
			was = &SearchEntry{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
		entry, err := x.entry(ctx, row, was)
		cancel()
		if err != nil {
			entry.Failures++
			slog.Warn("search: make failed", "object", hash, "failures", entry.Failures, "error", err)
		}
		raw, _ := json.Marshal(entry)
		if err := x.bucket.Put(context.Background(), hash, "application/json", raw); err != nil {
			slog.Error("search: store", "object", hash, "error", err)
		}
		x.mu.Lock()
		if !x.missing[hash] && x.entries[hash] == nil {
			x.mu.Unlock()
			if err := x.bucket.Remove(context.Background(), hash); err != nil {
				slog.Error("search: remove", "object", hash, "error", err)
			}
			continue
		}
		x.entries[hash] = entry
		delete(x.missing, hash)
		x.version++
		if entry.unfinished() {
			x.enqueue(hash)
		}
		x.mu.Unlock()
		x.signal()
		slog.Info("search: made", "object", hash, "failures", entry.Failures, "took", time.Since(start).Round(time.Millisecond))
	}
}

func (e *SearchEntry) unfinished() bool {
	return e.Summary == "" && e.Failures < searchAttempts
}

func (x *Searcher) entry(ctx context.Context, row SearchRow, was *SearchEntry) (*SearchEntry, error) {
	entry := &SearchEntry{Summary: was.Summary, Chunks: was.Chunks, Fingerprint: was.Fingerprint, Failures: was.Failures}
	if row.Table == "DOCUMENT" && entry.Fingerprint == nil {
		entry.Fingerprint = fingerprint(row.body())
	}
	if len(entry.Chunks) == 0 {
		texts := searchChunks(row.Head, row.body())
		vectors, err := x.vertex.Embed(ctx, texts, false, artifacts.SearchDims)
		if err != nil {
			return entry, err
		}
		for i, text := range texts {
			normalize(vectors[i])
			entry.Chunks = append(entry.Chunks, SearchChunk{Text: text, Vector: vectors[i]})
		}
	}
	if entry.Summary == "" {
		answer := &SearchEntry{}
		if _, err := claude.JSON(ctx, *x.client, SearchSummaryRequest(row.Input), answer); err != nil {
			return entry, err
		}
		if utf8.RuneCountInString(strings.TrimSpace(answer.Summary)) < searchShortest {
			return entry, fmt.Errorf("the summary is too short: %q", answer.Summary)
		}
		entry.Summary = strings.TrimSpace(answer.Summary)
	}
	return entry, nil
}

func searchChunks(head, body string) []string {
	pieces := []string{}
	current := ""
	for line := range strings.SplitSeq(body, "\n") {
		if current != "" && len(current)+len(line)+1 > searchChunk {
			pieces = append(pieces, current)
			current = ""
		}
		if current != "" {
			current += "\n"
		}
		current += line
	}
	pieces = append(pieces, current)
	if head == "" {
		return pieces
	}
	for i, p := range pieces {
		pieces[i] = head + "\n\n" + p
	}
	return pieces
}

func normalize(v []float32) {
	sum := 0.0
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	if sum == 0 {
		return
	}
	n := float32(math.Sqrt(sum))
	for i := range v {
		v[i] /= n
	}
}

func searchTerms(words string) []string {
	out := []string{}
	for _, t := range strings.FieldsFunc(strings.ToLower(words), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(t) > 1 && !searchFiller[t] && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

func (x *Searcher) snapshot() *searchView {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.view
}

func SearchLimits(asked map[string]int) (map[string]int, error) {
	out := map[string]int{}
	for _, t := range searchTables {
		out[t] = SearchLimit
	}
	for t, n := range asked {
		if _, ok := out[t]; !ok {
			return nil, access.Invalid("limits names %s; search answers %s", t, strings.Join(searchTables, ", "))
		}
		if n < 0 {
			return nil, access.Invalid("the limit for %s is %d; give 0 or more", t, n)
		}
		out[t] = n
	}
	return out, nil
}

func full(out map[string][]SearchResult, limits map[string]int) bool {
	for _, t := range searchTables {
		if len(out[t]) < limits[t] {
			return false
		}
	}
	return true
}

func readable(r *run, table, id string) bool {
	t, _ := Lookup(table)
	got, ok := r.table(t.Name).Get(id)
	return ok && r.readable(t, got)
}

func (v *searchView) pick(r *run, scores map[string]float64, exact map[string]bool, limits map[string]int) map[string][]SearchResult {
	objects := slices.Collect(maps.Keys(scores))
	when := func(o string) string {
		return v.rows[v.objects[o][0]].When
	}
	slices.SortFunc(objects, func(a, b string) int {
		if c := cmpDesc(scores[a], scores[b]); c != 0 {
			return c
		}
		if exact[a] != exact[b] {
			if exact[a] {
				return -1
			}
			return 1
		}
		if c := strings.Compare(when(b), when(a)); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	out := map[string][]SearchResult{}
	for _, t := range searchTables {
		out[t] = []SearchResult{}
	}
	at := map[string]int{}
	clusters := map[string]int{}
	shown := map[string]bool{}
	for _, o := range objects {
		if full(out, limits) {
			break
		}
		cluster := v.group(o)
		summary := v.entries[o].Summary
		score := math.Round(scores[o]*1000) / 1000
		for _, id := range v.objects[o] {
			row := v.rows[id]
			if row.Table != "DOCUMENT" {
				if len(out[row.Table]) < limits[row.Table] && readable(r, row.Table, id) {
					out[row.Table] = append(out[row.Table], SearchResult{ID: id, Name: row.Name, Href: row.Href, Summary: summary, Score: score})
				}
				continue
			}
			if shown[row.Source] || !readable(r, row.Table, id) {
				continue
			}
			docs := out["DOCUMENT"]
			i, ok := at[row.Terminal]
			if !ok {
				i, ok = clusters[cluster]
			}
			if !ok {
				if len(docs) >= limits["DOCUMENT"] {
					continue
				}
				docs = append(docs, SearchResult{})
				i = len(docs) - 1
				at[row.Terminal] = i
			}
			if len(docs[i].Refs) < searchRefs {
				ref := row.ref(id, summary)
				ref.Score = score
				docs[i].Refs = append(docs[i].Refs, ref)
			}
			if _, ok := clusters[cluster]; !ok {
				clusters[cluster] = i
			}
			shown[row.Source] = true
			out["DOCUMENT"] = docs
		}
	}
	return out
}

func (v *searchView) byMeaning(r *run, query []float32, limits map[string]int) (map[string]float64, map[string]int, int) {
	scores := map[string]float64{}
	scanned := map[string]int{}
	of := 0
	for _, t := range searchTables {
		index := v.vectors[t]
		order := index.order(query)
		of += len(order)
		if limits[t] == 0 {
			continue
		}
		own := map[string]float64{}
		for scanned[t] < len(order) {
			batch := order[scanned[t]:min(scanned[t]+searchProbe, len(order))]
			index.scan(query, batch, own)
			scanned[t] += len(batch)
			if len(v.pick(r, own, nil, map[string]int{t: limits[t]})[t]) == limits[t] {
				break
			}
		}
		maps.Copy(scores, own)
	}
	return scores, scanned, of
}

func (x *Searcher) Search(ctx context.Context, m *Model, env Env, words string, limits map[string]int) (map[string][]SearchResult, error) {
	start := time.Now()
	type embedded struct {
		vector []float32
		err    error
		took   time.Duration
	}
	done := make(chan embedded, 1)
	go func() {
		vectors, err := x.vertex.Embed(ctx, []string{words}, true, artifacts.SearchDims)
		if err != nil {
			done <- embedded{err: err, took: time.Since(start)}
			return
		}
		normalize(vectors[0])
		done <- embedded{vector: vectors[0], took: time.Since(start)}
	}()
	v := x.snapshot()
	r := m.newRun(env)
	scores, exact := v.byName(words)
	namesTook := time.Since(start)
	e := <-done
	if e.err != nil {
		return nil, fmt.Errorf("embed the words: %w", e.err)
	}
	waited := time.Since(start)
	near, scanned, of := v.byMeaning(r, e.vector, limits)
	for o, score := range near {
		if _, named := scores[o]; !named {
			scores[o] = score
		}
	}
	out := v.pick(r, scores, exact, limits)
	cells := 0
	for _, n := range scanned {
		cells += n
	}
	slog.InfoContext(ctx, "search", "viewer", env.Viewer, "system", env.System, "names_took", namesTook.Round(time.Millisecond), "embed_took", e.took.Round(time.Millisecond), "meaning_took", (time.Since(start) - waited).Round(time.Millisecond), "cells", cells, "of", of, "cells_group", scanned["GROUP"], "cells_person", scanned["PERSON"], "cells_document", scanned["DOCUMENT"], "took", time.Since(start).Round(time.Millisecond))
	return out, nil
}

func (r SearchRow) ref(extract, summary string) SearchRef {
	return SearchRef{Extract: extract, Document: r.Source, Source: r.SourceLine, Href: r.SourceHref, Terminal: r.Terminal, Name: r.Name, Summary: summary}
}

func (x *Searcher) Similar(m *Model, env Env, id string) ([]SearchRef, error) {
	v := x.snapshot()
	r := m.newRun(env)
	clusters := map[string]bool{}
	asked := map[string]bool{}
	for _, o := range slices.Sorted(maps.Keys(v.objects)) {
		for _, rid := range v.objects[o] {
			row := v.rows[rid]
			if row.Table == "DOCUMENT" && (rid == id || row.Source == id || row.Terminal == id) && readable(r, row.Table, rid) {
				clusters[v.group(o)] = true
				asked[rid] = true
			}
		}
	}
	if len(clusters) == 0 {
		return nil, access.Missing("no search entry for %s", id)
	}
	out := []SearchRef{}
	for _, o := range slices.Sorted(maps.Keys(v.objects)) {
		if !clusters[v.group(o)] {
			continue
		}
		for _, rid := range v.objects[o] {
			if !asked[rid] && readable(r, "DOCUMENT", rid) {
				out = append(out, v.rows[rid].ref(rid, v.entries[o].Summary))
			}
		}
	}
	return out, nil
}

func RegisterSearch(mux *http.ServeMux, s *Store, x *Searcher, tokens auth.Tokens, now func() time.Time) {
	mux.HandleFunc("POST /api/do/search", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, _, ok := caller(w, r, m, tokens, now())
		if !ok {
			return
		}
		var asked struct {
			Words  string         `json:"words"`
			Limits map[string]int `json:"limits"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&asked); err != nil {
			serve.Error(w, r, access.Invalid("send {\"words\": \"…\", \"limits\": {\"DOCUMENT\": 10}}: %v", err))
			return
		}
		if strings.TrimSpace(asked.Words) == "" {
			serve.Error(w, r, access.Invalid("words is required"))
			return
		}
		limits, err := SearchLimits(asked.Limits)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		results, err := x.Search(r.Context(), m, env, asked.Words, limits)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		serve.Write(w, r, http.StatusOK, results)
	})
}
