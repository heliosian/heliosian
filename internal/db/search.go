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
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	searchFolder   = "search"
	searchVersion  = 1
	searchResults  = 50
	searchChunk    = 1500
	searchMakers   = 4
	searchLoaders  = 16
	searchTimeout  = 2 * time.Minute
	searchAttempts = 3
	searchShortest = 20
)

var searchTables = []string{"GROUP", "PERSON", "DOCUMENT"}

const searchSystem = `You write the search entry for one thing in Helios, the apps of a small K-8 school community: a person, a group - a family, a classroom, an event, a volunteer activity, a party, an email list, a category and the like - or one part of an email the community received, its body or an attachment, read as text and headed by the email it came in. You are given what everyone who can see it can read.

summary: one or two plain sentences saying what it is, shown under its name in a list of search results. Say only what the text supports.
keywords: every word and short phrase someone might type to find it: the names of the people, groups, classes, places and events it is about, its subjects, and the dates it gives, then other names for those - synonyms, related terms, likely misspellings. Search matches these keywords alone, never the text, so a term left out here cannot find it. Leave out what says nothing about it, such as greetings, sign-offs and mailing-list boilerplate. At most 100.`

var searchSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"summary":  map[string]any{"type": "string"},
		"keywords": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required":             []string{"summary", "keywords"},
	"additionalProperties": false,
}

type SearchEntry struct {
	Version     int           `json:"version,omitempty"`
	Summary     string        `json:"summary"`
	Keywords    []string      `json:"keywords"`
	Chunks      []SearchChunk `json:"chunks"`
	Fingerprint []uint32      `json:"fingerprint,omitempty"`
	Failures    int           `json:"failures,omitempty"`
}

type SearchChunk struct {
	Text   string    `json:"text"`
	Vector []float32 `json:"vector"`
}

type SearchHit struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Href    string `json:"href,omitempty"`
	Summary string `json:"summary"`
}

type SearchResults struct {
	Words   []SearchHit `json:"words"`
	Meaning []SearchHit `json:"meaning"`
}

type SearchRow struct {
	Table, Head, Input, Object, Name, Href string
	named                                  store.Row
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
		row := store.Row{"id": Derive(SearchPrefix, id), "target": id, "input": r.Input, "object": r.Object, "made": "No"}
		if e := x.entries[r.Object]; e != nil {
			row["summary"], row["keywords"], row["chunks"], row["failures"], row["made"] = e.Summary, strings.Join(e.Keywords, ", "), strconv.Itoa(len(e.Chunks)), strconv.Itoa(e.Failures), "Yes"
			if e.Version > 0 {
				row["version"] = strconv.Itoa(e.Version)
			}
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

func (x *Searcher) Make(id string) (string, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.client == nil {
		return "", access.Refuse(http.StatusServiceUnavailable, "search entries are not made on this server")
	}
	r, ok := x.rows[id]
	if !ok {
		return "", access.Missing("no search row for %s", id)
	}
	x.enqueue(r.Object)
	return r.Object, nil
}

func SearchObject(input string) string {
	sum := sha256.Sum256([]byte(input))
	return searchFolder + "/" + hex.EncodeToString(sum[:]) + ".json"
}

func (m *Model) SearchInputs(texts map[string]string) map[string]SearchRow {
	out := map[string]SearchRow{}
	for _, table := range []string{"GROUP", "PERSON"} {
		for _, row := range m.Shown(table).All() {
			input := m.searchInput(table, row)
			if input == "" {
				continue
			}
			name := row["name"]
			if table == "PERSON" {
				name = row["name_show"]
			}
			out[row["id"]] = SearchRow{Table: table, Input: input, Object: SearchObject(input), Name: name, named: row}
		}
	}
	for _, row := range m.markdownExtracts() {
		text, ok := texts[row["content"]]
		if !ok {
			continue
		}
		terminal := m.terminal(row)
		head := m.extractHead(row, terminal)
		input := head + "\n\n" + strings.TrimSpace(text)
		name := terminal["name"]
		if name == "" {
			name = terminal["id"]
		}
		out[row["id"]] = SearchRow{Table: "DOCUMENT", Head: head, Input: input, Object: SearchObject(input), Name: name, named: terminal}
	}
	return out
}

func (m *Model) markdownExtracts() []store.Row {
	contents := m.Shown("CONTENT")
	out := []store.Row{}
	for _, row := range m.Shown("DOCUMENT").All() {
		if row["relation"] != "extract" {
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
	for _, row := range m.markdownExtracts() {
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

func (m *Model) terminal(row store.Row) store.Row {
	docs := m.Shown("DOCUMENT")
	for seen := map[string]bool{}; row["kind"] == "" && row["parent"] != "" && !seen[row["id"]]; {
		seen[row["id"]] = true
		parent, ok := docs.Get(row["parent"])
		if !ok {
			break
		}
		row = parent
	}
	return row
}

func (m *Model) extractHead(row, terminal store.Row) string {
	part, _ := m.Shown("DOCUMENT").Get(row["parent"])
	lines := []string{}
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			lines = append(lines, label+": "+value)
		}
	}
	add("Email", terminal["name"])
	add("Kind", terminal["kind"])
	add("Sent", terminal["published"])
	if author, ok := m.Shown("PERSON").Get(terminal["author"]); ok {
		add("From", author["name_show"])
	}
	if content, ok := m.Shown("CONTENT").Get(part["content"]); ok {
		add("Part", strings.TrimSpace(part["filename"]+" "+baseType(content["mime"])))
	}
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
			rows[id] = r
		}
		x.mu.Lock()
		wanted := map[string]bool{}
		unknown := []string{}
		for _, r := range rows {
			wanted[r.Object] = true
			if x.entries[r.Object] == nil && !x.missing[r.Object] && !x.dropped[r.Object] && !slices.Contains(unknown, r.Object) {
				unknown = append(unknown, r.Object)
			}
		}
		removed := []string{}
		for _, r := range x.rows {
			if x.client != nil && !wanted[r.Object] && x.entries[r.Object].current() && !slices.Contains(removed, r.Object) {
				removed = append(removed, r.Object)
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
			removed = append(removed, x.strays(wanted, removed)...)
		}
		if len(removed) > 0 {
			slog.Info("search: remove the unwanted entries", "objects", len(removed))
		}
		for _, hash := range removed {
			if err := x.bucket.Remove(context.Background(), hash); err != nil {
				slog.Error("search: remove", "object", hash, "error", err)
			}
		}
		loaded, errs := loadAll(len(unknown), func(i int) (*SearchEntry, error) { return x.load(unknown[i]) })
		x.mu.Lock()
		for i, hash := range unknown {
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
		x.mu.Unlock()
		if len(unknown) > 0 {
			slog.Info("search: read the index", "rows", len(rows), "loaded", len(unknown), "missing", missing, "took", time.Since(start).Round(time.Millisecond))
		}
		x.signal()
	}
}

func (x *Searcher) strays(wanted map[string]bool, removed []string) []string {
	held, err := x.bucket.List(context.Background(), searchFolder+"/")
	if err != nil {
		slog.Error("search: list the entries", "error", err)
		x.mu.Lock()
		x.swept = false
		x.mu.Unlock()
		return nil
	}
	candidates := []string{}
	for _, hash := range held {
		if !wanted[hash] && !slices.Contains(removed, hash) {
			candidates = append(candidates, hash)
		}
	}
	loaded, errs := loadAll(len(candidates), func(i int) (*SearchEntry, error) { return x.load(candidates[i]) })
	out := []string{}
	for i, hash := range candidates {
		if errs[i] == nil && loaded[i].current() {
			out = append(out, hash)
		}
	}
	return out
}

func (x *Searcher) index() {
	vectors := &vectorIndex{}
	for range x.reindex {
		start := time.Now()
		x.mu.RLock()
		rows := x.rows
		entries := map[string]*SearchEntry{}
		for _, r := range rows {
			if e := x.entries[r.Object]; e.current() {
				entries[r.Object] = e
			}
		}
		x.mu.RUnlock()
		vectors = vectors.update(entries)
		view := buildView(rows, entries, vectors)
		x.mu.Lock()
		x.view = view
		x.mu.Unlock()
		slog.Info("search: indexed", "entries", len(entries), "words", len(view.words), "cells", len(vectors.centroids), "groups", len(view.members), "took", time.Since(start).Round(time.Millisecond))
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
		if e := x.entries[hash]; !x.missing[hash] && e.current() && !e.unfinished() {
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
		if !was.current() {
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

func (e *SearchEntry) current() bool {
	return e != nil && e.Version == searchVersion
}

func (e *SearchEntry) unfinished() bool {
	return e.current() && e.Summary == "" && e.Failures < searchAttempts
}

func (x *Searcher) entry(ctx context.Context, row SearchRow, was *SearchEntry) (*SearchEntry, error) {
	entry := &SearchEntry{Version: searchVersion, Summary: was.Summary, Keywords: was.Keywords, Chunks: was.Chunks, Fingerprint: was.Fingerprint, Failures: was.Failures}
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
		if _, err := claude.JSON(ctx, *x.client, anthropic.MessageNewParams{
			Model:        claude.SearchSummaryModel,
			MaxTokens:    32000,
			System:       []anthropic.TextBlockParam{{Text: searchSystem}},
			Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(row.Input))},
			OutputConfig: anthropic.OutputConfigParam{Effort: claude.SearchSummaryEffort, Format: anthropic.JSONOutputFormatParam{Schema: searchSchema}},
		}, answer); err != nil {
			return entry, err
		}
		if utf8.RuneCountInString(strings.TrimSpace(answer.Summary)) < searchShortest {
			return entry, fmt.Errorf("the summary is too short: %q", answer.Summary)
		}
		entry.Summary, entry.Keywords = answer.Summary, searchKeywords(answer.Keywords)
	}
	return entry, nil
}

func searchKeywords(keywords []string) []string {
	out := []string{}
	for _, k := range keywords {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
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
		if len(t) > 1 && !slices.Contains(out, t) {
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

func (v *searchView) pick(m *Model, env Env, scores map[string]float64) map[string][]SearchHit {
	objects := slices.Collect(maps.Keys(scores))
	slices.SortFunc(objects, func(a, b string) int {
		if c := cmpDesc(scores[a], scores[b]); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	r := m.newRun(env)
	out := map[string][]SearchHit{}
	for _, t := range searchTables {
		out[t] = []SearchHit{}
	}
	shown := map[string]bool{}
	full := 0
	for _, o := range objects {
		if full == len(searchTables) {
			break
		}
		g := v.group(o)
		for _, id := range v.objects[o] {
			row := v.rows[id]
			if len(out[row.Table]) == searchResults || row.Table == "DOCUMENT" && shown[g] {
				continue
			}
			t, _ := Lookup(row.Table)
			got, ok := r.table(t.Name).Get(id)
			if !ok || !r.readable(t, got) {
				continue
			}
			if row.Table == "DOCUMENT" {
				shown[g] = true
			}
			out[row.Table] = append(out[row.Table], SearchHit{ID: id, Name: row.Name, Href: row.Href, Summary: v.entries[o].Summary})
			if len(out[row.Table]) == searchResults {
				full++
			}
		}
	}
	return out
}

func (x *Searcher) Words(m *Model, env Env, words string) map[string][]SearchHit {
	v := x.snapshot()
	return v.pick(m, env, v.byWords(words))
}

func (x *Searcher) Search(ctx context.Context, m *Model, env Env, words string) (map[string]*SearchResults, error) {
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
	byWords := v.pick(m, env, v.byWords(words))
	wordsTook := time.Since(start)
	e := <-done
	if e.err != nil {
		return nil, fmt.Errorf("embed the words: %w", e.err)
	}
	waited := time.Since(start)
	byMeaning := v.pick(m, env, v.vectors.search(e.vector))
	out := map[string]*SearchResults{}
	for _, t := range searchTables {
		out[t] = &SearchResults{Words: byWords[t], Meaning: byMeaning[t]}
	}
	slog.InfoContext(ctx, "search", "viewer", env.Viewer, "system", env.System, "words_took", wordsTook.Round(time.Millisecond), "embed_took", e.took.Round(time.Millisecond), "meaning_took", (time.Since(start) - waited).Round(time.Millisecond), "took", time.Since(start).Round(time.Millisecond))
	return out, nil
}

func (x *Searcher) Similar(m *Model, env Env, id string) ([]SearchHit, error) {
	v := x.snapshot()
	row, ok := v.rows[id]
	if !ok || v.entries[row.Object] == nil {
		return nil, access.Missing("no search entry for %s", id)
	}
	r := m.newRun(env)
	t, _ := Lookup(row.Table)
	if got, ok := r.table(t.Name).Get(id); !ok || !r.readable(t, got) {
		return nil, access.Missing("no search entry for %s", id)
	}
	objects := v.members[v.group(row.Object)]
	if len(objects) == 0 {
		objects = []string{row.Object}
	}
	out := []SearchHit{}
	for _, o := range objects {
		for _, other := range v.objects[o] {
			if other == id {
				continue
			}
			got, ok := r.table(t.Name).Get(other)
			if !ok || !r.readable(t, got) {
				continue
			}
			twin := v.rows[other]
			out = append(out, SearchHit{ID: other, Name: twin.Name, Href: twin.Href, Summary: v.entries[o].Summary})
		}
	}
	return out, nil
}

func RegisterSearch(mux *http.ServeMux, s *Store, x *Searcher, importKey []byte, now func() time.Time) {
	mux.HandleFunc("POST "+doPrefix+"search", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, _, ok := caller(w, r, m, importKey, now())
		if !ok {
			return
		}
		var asked struct {
			Words string `json:"words"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&asked); err != nil {
			serve.Error(w, r, access.Invalid("send {\"words\": \"…\"}: %v", err))
			return
		}
		if strings.TrimSpace(asked.Words) == "" {
			serve.Error(w, r, access.Invalid("words is required"))
			return
		}
		results, err := x.Search(r.Context(), m, env, asked.Words)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		serve.Write(w, r, http.StatusOK, results)
	})
	mux.HandleFunc("POST "+doPrefix+"search/make", func(w http.ResponseWriter, r *http.Request) {
		env, _, ok := caller(w, r, s.Model(), importKey, now())
		if !ok {
			return
		}
		if env.System != importReader {
			serve.Error(w, r, access.Forbidden("only the import key makes search entries"))
			return
		}
		var asked struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&asked); err != nil {
			serve.Error(w, r, access.Invalid("send {\"id\": \"…\"}: %v", err))
			return
		}
		object, err := x.Make(strings.TrimSpace(asked.ID))
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		serve.Write(w, r, http.StatusOK, map[string]string{"object": object})
	})
}
