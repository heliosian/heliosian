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
	searchResults  = 50
	searchChunk    = 1500
	searchLexical  = 0.15
	searchMakers   = 4
	searchLoaders  = 16
	searchTimeout  = 2 * time.Minute
	searchAttempts = 3
	searchShortest = 20
)

const searchSystem = `You write the search entry for one thing in Helios, the apps of a small K-8 school community: a person, a group - a family, a classroom, an event, a volunteer activity, a party, an email list, a category and the like - or one part of an email the community received, its body or an attachment, read as text and headed by the email it came in. You are given what everyone who can see it can read.

summary: one or two plain sentences saying what it is, shown under its name in a list of search results. Say only what the text supports.
keywords: words and short phrases someone might type looking for it that its text doesn't already contain - synonyms, related terms, other names for the same thing, likely misspellings. Lower case, at most 30.`

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
	Summary  string        `json:"summary"`
	Keywords []string      `json:"keywords"`
	Chunks   []SearchChunk `json:"chunks"`
	Failures int           `json:"failures,omitempty"`
}

type SearchChunk struct {
	Text   string    `json:"text"`
	Vector []float32 `json:"vector"`
}

type SearchHit struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

type SearchRow struct {
	Table, Input, Object string
}

type SearchIndex struct {
	mu      sync.RWMutex
	rows    map[string]SearchRow
	entries map[string]*SearchEntry
	missing map[string]bool
	dropped map[string]bool
	version int
	poke    chan struct{}
}

func NewSearchIndex() *SearchIndex {
	return &SearchIndex{rows: map[string]SearchRow{}, entries: map[string]*SearchEntry{}, missing: map[string]bool{}, dropped: map[string]bool{}, poke: make(chan struct{}, 1)}
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
	client  *anthropic.Client
	texts   map[string]string
	pending []string
	waiting map[string]bool
	swept   bool
	wake    chan struct{}
}

func NewSearcher(s *Store, queue *store.Queue, bucket *blob.Bucket, vertex *artifacts.Vertex) *Searcher {
	x := &Searcher{SearchIndex: s.Model().index, s: s, bucket: bucket, vertex: vertex, texts: map[string]string{}, waiting: map[string]bool{}, wake: make(chan struct{}, 1)}
	go x.follow()
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
	for hash := range x.missing {
		x.enqueue(hash)
	}
	for hash, e := range x.entries {
		if e.unfinished() {
			x.enqueue(hash)
		}
	}
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

func (m *Model) SearchInputs(texts map[string]string) map[string]SearchRow {
	out := map[string]SearchRow{}
	for _, table := range []string{"GROUP", "PERSON"} {
		for _, row := range m.Shown(table).All() {
			input := m.searchInput(table, row)
			if input == "" {
				continue
			}
			out[row["id"]] = SearchRow{Table: table, Input: input, Object: SearchObject(input)}
		}
	}
	for _, row := range m.markdownExtracts() {
		text, ok := texts[row["content"]]
		if !ok {
			continue
		}
		input := m.extractInput(row, text)
		out[row["id"]] = SearchRow{Table: "DOCUMENT", Input: input, Object: SearchObject(input)}
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

func (m *Model) extractInput(row store.Row, text string) string {
	docs := m.Shown("DOCUMENT")
	part, _ := docs.Get(row["parent"])
	root := part
	for seen := map[string]bool{}; root["parent"] != "" && !seen[root["id"]]; {
		seen[root["id"]] = true
		root, _ = docs.Get(root["parent"])
	}
	lines := []string{}
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			lines = append(lines, label+": "+value)
		}
	}
	add("Email", root["name"])
	add("Kind", root["kind"])
	add("Sent", root["published"])
	if author, ok := m.Shown("PERSON").Get(root["author"]); ok {
		add("From", author["name_show"])
	}
	if content, ok := m.Shown("CONTENT").Get(part["content"]); ok {
		add("Part", strings.TrimSpace(part["filename"]+" "+baseType(content["mime"])))
	}
	return strings.Join(lines, "\n") + "\n\n" + strings.TrimSpace(text)
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
			if x.client != nil && !wanted[r.Object] && !slices.Contains(removed, r.Object) {
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
			held, err := x.bucket.List(context.Background(), searchFolder+"/")
			if err != nil {
				slog.Error("search: list the entries", "error", err)
				x.mu.Lock()
				x.swept = false
				x.mu.Unlock()
			}
			for _, hash := range held {
				if !wanted[hash] && !slices.Contains(removed, hash) {
					removed = append(removed, hash)
				}
			}
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
				if x.client != nil && loaded[i].unfinished() {
					x.enqueue(hash)
				}
				continue
			}
			x.missing[hash] = true
			if x.client != nil {
				x.enqueue(hash)
			}
		}
		missing := len(x.missing)
		x.mu.Unlock()
		if len(unknown) > 0 {
			slog.Info("search: read the index", "rows", len(rows), "loaded", len(unknown), "missing", missing, "took", time.Since(start).Round(time.Millisecond))
		}
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

func (x *Searcher) next() (string, string, bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	for len(x.pending) > 0 {
		hash := x.pending[0]
		x.pending = x.pending[1:]
		delete(x.waiting, hash)
		if e := x.entries[hash]; !x.missing[hash] && (e == nil || !e.unfinished()) {
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
			return hash, r.Input, true
		}
	}
	return "", "", false
}

func (x *Searcher) make() {
	for {
		hash, input, ok := x.next()
		if !ok {
			<-x.wake
			continue
		}
		start := time.Now()
		x.mu.Lock()
		was := x.entries[hash]
		x.mu.Unlock()
		if was == nil {
			was = &SearchEntry{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
		entry, err := x.entry(ctx, input, was)
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
		left := len(x.missing)
		x.mu.Unlock()
		slog.Info("search: made", "object", hash, "missing", left, "failures", entry.Failures, "took", time.Since(start).Round(time.Millisecond))
	}
}

func (e *SearchEntry) unfinished() bool {
	return e.Summary == "" && e.Failures < searchAttempts
}

func (x *Searcher) entry(ctx context.Context, input string, was *SearchEntry) (*SearchEntry, error) {
	entry := &SearchEntry{Summary: was.Summary, Keywords: was.Keywords, Chunks: was.Chunks, Failures: was.Failures}
	if len(entry.Chunks) == 0 {
		texts := searchChunks(input)
		vectors, err := x.vertex.Embed(ctx, texts, false)
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
			Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(input))},
			OutputConfig: anthropic.OutputConfigParam{Effort: claude.SearchSummaryEffort, Format: anthropic.JSONOutputFormatParam{Schema: searchSchema}},
		}, answer); err != nil {
			return entry, err
		}
		if utf8.RuneCountInString(strings.TrimSpace(answer.Summary)) < searchShortest {
			return entry, fmt.Errorf("the summary is too short: %q", answer.Summary)
		}
		entry.Summary, entry.Keywords = answer.Summary, answer.Keywords
	}
	return entry, nil
}

func searchChunks(input string) []string {
	out := []string{}
	current := ""
	for line := range strings.SplitSeq(input, "\n") {
		if current != "" && len(current)+len(line)+1 > searchChunk {
			out = append(out, current)
			current = ""
		}
		if current != "" {
			current += "\n"
		}
		current += line
	}
	return append(out, current)
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

type scored struct {
	id      string
	summary string
	score   float64
}

func (x *Searcher) rank(m *Model, env Env, words string, vector []float32) []SearchHit {
	terms := searchTerms(words)
	r := m.newRun(env)
	x.mu.RLock()
	hits := []scored{}
	for id, row := range x.rows {
		entry := x.entries[row.Object]
		found := 0
		if len(terms) > 0 {
			text := strings.ToLower(row.Input)
			if entry != nil {
				text += "\n" + strings.ToLower(entry.Summary+"\n"+strings.Join(entry.Keywords, "\n"))
			}
			for _, t := range terms {
				if strings.Contains(text, t) {
					found++
				}
			}
		}
		lexical := 0.0
		if len(terms) > 0 {
			lexical = float64(found) / float64(len(terms))
		}
		score := lexical
		if vector != nil {
			if entry == nil {
				continue
			}
			best := -1.0
			for _, c := range entry.Chunks {
				best = max(best, dot(vector, c.Vector))
			}
			score = best + searchLexical*lexical
		} else if found == 0 {
			continue
		}
		summary := ""
		if entry != nil {
			summary = entry.Summary
		}
		hits = append(hits, scored{id: id, summary: summary, score: score})
	}
	tables := map[string]string{}
	for _, h := range hits {
		tables[h.id] = x.rows[h.id].Table
	}
	x.mu.RUnlock()
	slices.SortFunc(hits, func(a, b scored) int {
		if a.score != b.score {
			if a.score > b.score {
				return -1
			}
			return 1
		}
		return strings.Compare(a.id, b.id)
	})
	out := []SearchHit{}
	shown := map[string]bool{}
	for _, h := range hits {
		if len(out) == searchResults {
			break
		}
		t, _ := Lookup(tables[h.id])
		row, ok := r.table(t.Name).Get(h.id)
		if !ok || !r.readable(t, row) {
			continue
		}
		if t.Name == "DOCUMENT" {
			if shown[row["content"]] {
				continue
			}
			shown[row["content"]] = true
		}
		out = append(out, SearchHit{ID: h.id, Summary: h.summary})
	}
	return out
}

func dot(a, b []float32) float64 {
	sum := 0.0
	for i := range min(len(a), len(b)) {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
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
		controller := http.NewResponseController(w)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		emit := func(kind string, data any) {
			encoded, err := json.Marshal(data)
			if err != nil {
				slog.ErrorContext(r.Context(), "search: encode", "event", kind, "error", err)
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, encoded)
			if err := controller.Flush(); err != nil {
				slog.ErrorContext(r.Context(), "search: flush", "error", err)
			}
		}
		start := time.Now()
		words := x.Words(m, env, asked.Words)
		emit("words", map[string]any{"result": words})
		took := time.Since(start)
		meaning, err := x.Meaning(r.Context(), m, env, asked.Words)
		if err != nil {
			slog.ErrorContext(r.Context(), "search: meaning", "error", err)
			emit("error", map[string]string{"error": "the search by meaning failed"})
			return
		}
		emit("meaning", map[string]any{"result": meaning})
		slog.InfoContext(r.Context(), "search", "viewer", env.Viewer, "system", env.System, "words", len(words), "meaning", len(meaning), "words_took", took.Round(time.Millisecond), "took", time.Since(start).Round(time.Millisecond))
	})
}

func (x *Searcher) Words(m *Model, env Env, words string) []SearchHit {
	return x.rank(m, env, words, nil)
}

func (x *Searcher) Meaning(ctx context.Context, m *Model, env Env, words string) ([]SearchHit, error) {
	vectors, err := x.vertex.Embed(ctx, []string{words}, true)
	if err != nil {
		return nil, err
	}
	normalize(vectors[0])
	return x.rank(m, env, words, vectors[0]), nil
}
