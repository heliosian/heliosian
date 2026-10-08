package db

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/intercept"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

var testLimits, _ = SearchLimits(nil)

func testOrigin(app string) string {
	return "https://" + app + ".example.org"
}

func newSearcher(t *testing.T, claude http.Handler, bucket *blob.Bucket) (*Store, *store.Queue, *Searcher) {
	t.Helper()
	intercept.GoogleLogin(t.TempDir())
	intercept.Install(intercept.VertexHost, intercept.Vertex())
	intercept.Install(intercept.ClaudeHost, claude)
	vertex, err := artifacts.NewVertex()
	if err != nil {
		t.Fatal(err)
	}
	s, queue := sampleWithQueue(t)
	x := NewSearcher(s, queue, bucket, vertex, testOrigin)
	x.StartMaking("test")
	return s, queue, x
}

func searcher(t *testing.T) (*Store, *store.Queue, *blob.Bucket, *Searcher) {
	t.Helper()
	bucket := blob.NewMemoryBucket()
	s, queue, x := newSearcher(t, testkit.SearchClaude(), bucket)
	return s, queue, bucket, x
}

func waitRows(t *testing.T, s *Store, x *Searcher) map[string]SearchRow {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		m := s.Model()
		texts, err := SearchTexts(context.Background(), m, x.bucket, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		want := m.SearchInputs(texts)
		x.mu.RLock()
		done := len(x.rows) == len(want)
		for id, r := range want {
			if x.rows[id].Object != r.Object {
				done = false
			}
		}
		x.mu.RUnlock()
		if done {
			return want
		}
	}
	t.Fatal("the searcher never read every row")
	return nil
}

func askAll(t *testing.T, s *Store, x *Searcher) map[string]SearchRow {
	t.Helper()
	rows := waitRows(t, s, x)
	for id := range rows {
		if _, err := x.Make(id); err != nil {
			t.Fatal(err)
		}
	}
	return rows
}

func waitIndexed(t *testing.T, x *Searcher, rows map[string]SearchRow) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		v := x.snapshot()
		done := true
		for _, r := range rows {
			if e := v.entries[r.Object]; e == nil || e.Summary == "" {
				done = false
			}
		}
		if done {
			return
		}
	}
	t.Fatal("the searcher never indexed every row it was asked to make")
}

func makeAll(t *testing.T, s *Store, x *Searcher) {
	t.Helper()
	waitIndexed(t, x, askAll(t, s, x))
}

func hitIDs(results []SearchResult) []string {
	out := []string{}
	for _, r := range results {
		out = append(out, r.ID)
	}
	return out
}

func extractsOf(results []SearchResult) []string {
	out := []string{}
	for _, r := range results {
		for _, ref := range r.Refs {
			out = append(out, ref.Extract)
		}
	}
	return out
}

func sendMail(t *testing.T, s *Store, pics *Pictures, subject, body string) string {
	t.Helper()
	root := uploadMail(t, s, pics, "From: Maya Lindqvist <maya.lindqvist@example.org>\r\nDate: Thu, 12 Feb 2026 01:48:03 +0000\r\nSubject: "+subject+"\r\nList-Id: <hummingbirds.parents.heliosschool.org>\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>"+body+"</p>\r\n")
	made(t, s, "DOCUMENT", root, "extracted")
	part := children(s, root)[0]
	made(t, s, "DOCUMENT", part["id"], "extracted")
	return children(s, part["id"])[0]["id"]
}

func TestNothingIsMadeUnasked(t *testing.T) {
	s, _, bucket, x := searcher(t)
	waitRows(t, s, x)
	time.Sleep(200 * time.Millisecond)
	held, err := bucket.List(context.Background(), searchFolder+"/")
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 0 {
		t.Fatalf("the searcher made %d entries nobody asked for", len(held))
	}
}

func TestAnAnswerCutShortIsTriedThreeTimes(t *testing.T) {
	var mu sync.Mutex
	asked := map[string]int{}
	s, _, x := newSearcher(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, _ := io.ReadAll(r.Body)
		mu.Lock()
		asked[string(request)]++
		mu.Unlock()
		testkit.ClaudeStream(w, `{"summary": "cut`, "max_tokens")
	}), blob.NewMemoryBucket())
	rows := askAll(t, s, x)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		x.mu.RLock()
		done := true
		for _, r := range rows {
			e := x.entries[r.Object]
			done = done && e != nil && e.Failures == searchAttempts
		}
		x.mu.RUnlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the entries never reached three failures")
		}
	}
	time.Sleep(200 * time.Millisecond)
	x.mu.RLock()
	for _, e := range x.entries {
		if e.Summary != "" || len(e.Chunks) == 0 {
			t.Fatalf("an entry whose answer was cut short: %+v", e)
		}
	}
	x.mu.RUnlock()
	mu.Lock()
	defer mu.Unlock()
	for request, n := range asked {
		if n != searchAttempts {
			t.Fatalf("an input was asked %d times, want %d: %.80s", n, searchAttempts, request)
		}
	}
}

func TestAShortSummaryCountsAsAFailure(t *testing.T) {
	var mu sync.Mutex
	asked := map[string]int{}
	s, _, x := newSearcher(t, testkit.ClaudeReplying(func(request string) string {
		mu.Lock()
		defer mu.Unlock()
		asked[request]++
		if asked[request] == 1 {
			return `{"summary": "x", "keywords": ["outing"]}`
		}
		return `{"summary": "a thing in the sample, asked twice", "keywords": ["outing"]}`
	}), blob.NewMemoryBucket())
	askAll(t, s, x)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		rows := runAs(t, s.Model(), "", `(from SEARCH (where (= failures 1) (= summary "a thing in the sample, asked twice") (> chunks 0)))`).Rows()
		if len(rows) > 0 && len(rows) == len(runAs(t, s.Model(), "", `(from SEARCH)`).Rows()) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d entries have one failure and the second answer", len(rows))
		}
	}
}

func TestDeletingASearchEntryRemovesItUntilAskedAgain(t *testing.T) {
	s, queue, bucket, x := searcher(t)
	makeAll(t, s, x)
	rows := runAs(t, s.Model(), "", `(from SEARCH (where (= target "grp00000000040")))`).Rows()
	if len(rows) != 1 {
		t.Fatalf("search rows for the event: %v", rows)
	}
	b := Batch{Batch: []Edit{{Delete: rows[0]["id"]}}}
	if _, err := Write(context.Background(), s, queue, newPictures(s, queue), access.Actor{Email: "maya@example.com"}, Env{Viewer: "per00000000002", Now: testNow}, b); err == nil {
		t.Fatal("a parent deleted a search entry")
	}
	if _, err := Write(context.Background(), s, queue, newPictures(s, queue), access.System(importReader), Env{System: importReader, Now: testNow}, b); err != nil {
		t.Fatal(err)
	}
	object := rows[0]["object"]
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		held, err := bucket.Exists(context.Background(), object)
		if err != nil {
			t.Fatal(err)
		}
		if !held && x.snapshot().entries[object] == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the deleted entry is still stored or indexed")
		}
	}
	time.Sleep(200 * time.Millisecond)
	if held, _ := bucket.Exists(context.Background(), object); held {
		t.Fatal("the deleted entry was made again unasked")
	}
	if _, err := x.Make("grp00000000040"); err != nil {
		t.Fatal(err)
	}
	waitIndexed(t, x, map[string]SearchRow{"grp00000000040": {Object: object}})
}

func TestEveryShownRowHasAnInput(t *testing.T) {
	s, _, bucket, x := searcher(t)
	makeAll(t, s, x)
	inputs := s.Model().SearchInputs(nil)
	if _, ok := inputs["grp00000000041"]; !ok {
		t.Error("a managers group has no search input")
	}
	picnic := inputs["grp00000000040"]
	if !strings.Contains(picnic.Input, "Fall Picnic") || !strings.Contains(picnic.Input, "Under: Community") {
		t.Errorf("the picnic's input is %q", picnic.Input)
	}
	raw, _, err := bucket.Get(context.Background(), picnic.Object)
	if err != nil {
		t.Fatal(err)
	}
	entry := SearchEntry{}
	if err := json.Unmarshal(raw, &entry); err != nil || len(entry.Chunks[0].Vector) != artifacts.SearchDims {
		t.Errorf("the picnic's entry has %d dimensions: %v", len(entry.Chunks[0].Vector), err)
	}
}

func TestKeywordsAreLowercased(t *testing.T) {
	s, _, x := newSearcher(t, testkit.ClaudeReplying(func(string) string {
		return `{"summary": "a thing in the sample, shouted", "keywords": ["Fall Picnic", "fall picnic", " PICNIC "]}`
	}), blob.NewMemoryBucket())
	makeAll(t, s, x)
	x.mu.RLock()
	defer x.mu.RUnlock()
	for _, e := range x.entries {
		if !slices.Equal(e.Keywords, []string{"fall picnic", "picnic"}) {
			t.Fatalf("keywords %q", e.Keywords)
		}
	}
}

func TestAnEmailsMarkdownIsSearchedByThoseItWasSentTo(t *testing.T) {
	s, queue, bucket, x := searcher(t)
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	extract := sendMail(t, s, pics, "Tide pools", "Bring <b>boots</b> for the tide pools.")
	makeAll(t, s, x)
	x.mu.RLock()
	row := x.rows[extract]
	x.mu.RUnlock()
	for _, want := range []string{"Email: Tide pools", "Kind: mail", "From: Maya Lindqvist", "Source: email body, text/html", "Bring **boots** for the tide pools."} {
		if !strings.Contains(row.Input, want) {
			t.Errorf("the extract's input lacks %q:\n%s", want, row.Input)
		}
	}
	if row.Name != "Tide pools" {
		t.Errorf("the extract is named %q", row.Name)
	}
	m := s.Model()
	if got := extractsOf(x.Words(m, Env{Viewer: parent, Now: testNow}, "boots", testLimits)["DOCUMENT"]); !slices.Contains(got, extract) {
		t.Errorf("the Hummingbirds parent's search for boots found %v", got)
	}
	if got := extractsOf(x.Words(m, Env{Viewer: student, Now: testNow}, "boots", testLimits)["DOCUMENT"]); slices.Contains(got, extract) {
		t.Errorf("a student the email was not sent to found it: %v", got)
	}
}

func TestEveryChunkCarriesTheHeader(t *testing.T) {
	s, queue, bucket, x := searcher(t)
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	paragraphs := []string{}
	for i := range 40 {
		paragraphs = append(paragraphs, fmt.Sprintf("<p>Paragraph %d of the long letter about the spring camping trip and what every family needs to pack for three nights.</p>", i))
	}
	extract := sendMail(t, s, pics, "Camping letter", strings.Join(paragraphs, ""))
	makeAll(t, s, x)
	x.mu.RLock()
	row := x.rows[extract]
	entry := x.entries[row.Object]
	x.mu.RUnlock()
	if len(entry.Chunks) < 2 {
		t.Fatalf("the long letter made %d chunks", len(entry.Chunks))
	}
	for i, c := range entry.Chunks {
		if !strings.HasPrefix(c.Text, row.Head+"\n\n") || !strings.Contains(c.Text, "Email: Camping letter") {
			t.Errorf("chunk %d lacks the header: %.120s", i, c.Text)
		}
	}
}

func TestTheHeaderSaysWhatTheThingIs(t *testing.T) {
	m := sample(t).Model()
	file := store.Row{"id": "doc1", "kind": "file", "name": "Supply list", "published": "2026-08-20 09:00:00"}
	wiki := store.Row{"id": "doc2", "kind": "wiki", "name": "Field trips"}
	mail := store.Row{"id": "doc3", "kind": "mail", "name": "Picture day"}
	calendar := store.Row{"id": "doc4", "kind": "calendar", "name": "2026-2027 Helios School Calendar"}
	for _, c := range []struct {
		terminal, source store.Row
		want             string
	}{
		{file, file, "File: Supply list\nKind: file\nUpdated: 2026-08-20 09:00:00"},
		{wiki, wiki, "Wiki page: Field trips\nKind: wiki"},
		{calendar, calendar, "Calendar: 2026-2027 Helios School Calendar\nKind: calendar"},
		{mail, store.Row{"id": "doc5", "relation": "part", "content": "nothing"}, "Email: Picture day\nKind: mail\nSource: embedded in the email"},
		{mail, store.Row{"id": "doc6", "relation": "part", "filename": "rules.pdf"}, "Email: Picture day\nKind: mail\nSource: attached file, filename \"rules.pdf\""},
		{file, store.Row{"id": "doc7", "relation": "linked", "name": "menu", "url": "https://example.org/menu"}, "File: Supply list\nKind: file\nUpdated: 2026-08-20 09:00:00\nSource: linked from the file, link text \"menu\", https://example.org/menu"},
	} {
		if got := m.extractHead(c.terminal, m.sourceLine(c.source, c.terminal)); got != c.want {
			t.Errorf("%s under %s:\n%s\nwant\n%s", c.source["id"], c.terminal["id"], got, c.want)
		}
	}
}

func TestTheSameTextIsFoundOnce(t *testing.T) {
	s, queue, bucket, x := searcher(t)
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	extracts := []string{}
	for _, subject := range []string{"Tide pools", "Tide pools again"} {
		extracts = append(extracts, sendMail(t, s, pics, subject, "Bring <b>boots</b> for the tide pools."))
	}
	makeAll(t, s, x)
	m := s.Model()
	env := Env{Viewer: parent, Now: testNow}
	results := x.Words(m, env, "boots", testLimits)["DOCUMENT"]
	if len(results) != 1 || len(results[0].Refs) != 2 || !slices.Equal(slices.Sorted(slices.Values(extractsOf(results))), slices.Sorted(slices.Values(extracts))) {
		t.Fatalf("the two copies are not one result with two refs: %+v", results)
	}
	first, second := results[0].Refs[0].Terminal, results[0].Refs[1].Terminal
	if results[0].ID != "" || results[0].Name != "" || results[0].Summary != "" || first == second || results[0].Refs[1].Name == "" || results[0].Refs[1].Summary == "" {
		t.Fatalf("the result is %+v", results[0])
	}
	similar, err := x.Similar(m, env, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(similar) != 1 || similar[0].Terminal != second || similar[0].Extract != results[0].Refs[1].Extract {
		t.Fatalf("the copies of %s are %+v", first, similar)
	}
	if _, err := x.Similar(m, Env{Viewer: student, Now: testNow}, first); err == nil {
		t.Error("a student the email was not sent to listed its copies")
	}
}

func TestNearlyTheSameTextIsFoundOnce(t *testing.T) {
	s, queue, bucket, x := searcher(t)
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	tidePools := "Bring boots for the tide pools. We meet at the north end of the beach at nine, walk the rocks with the ranger, count the anemones, sea stars and crabs in each pool, sketch what we find in our field journals, eat lunch on the bluff above the cove and walk back to the bus by one. Pack water, a hat, sunscreen and a change of socks in case a wave comes over the rocks."
	extracts := []string{}
	for i, body := range []string{tidePools + " See you Friday.", tidePools + " See you Monday.", "Rain boots are on sale at the book fair this week, in every size from toddler to adult, with half the money going to the library."} {
		extracts = append(extracts, sendMail(t, s, pics, fmt.Sprintf("Field trip %d", i), body))
	}
	makeAll(t, s, x)
	results := x.Words(s.Model(), Env{Viewer: parent, Now: testNow}, "boots", testLimits)["DOCUMENT"]
	together := slices.IndexFunc(results, func(r SearchResult) bool {
		got := extractsOf([]SearchResult{r})
		return slices.Contains(got, extracts[0]) && slices.Contains(got, extracts[1])
	})
	if len(results) != 2 || together < 0 || len(results[together].Refs) != 2 || !slices.Contains(extractsOf(results), extracts[2]) {
		t.Fatalf("the near copies are not one result beside the sale: %+v", results)
	}
}

func TestAHitJoinsItsOwnEmailBeforeItsCopy(t *testing.T) {
	s, queue, bucket, x := searcher(t)
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	headers := "From: Maya Lindqvist <maya.lindqvist@example.org>\r\nList-Id: <hummingbirds.parents.heliosschool.org>\r\nMIME-Version: 1.0\r\n"
	body := "<p>Bring <b>boots</b> for the tide pools.</p>"
	first := uploadMail(t, s, pics, headers+"Date: Fri, 13 Feb 2026 01:48:03 +0000\r\nSubject: Tide pools\r\nContent-Type: text/html; charset=utf-8\r\n\r\n"+body+"\r\n")
	second := uploadMail(t, s, pics, headers+"Date: Thu, 12 Feb 2026 01:48:03 +0000\r\nSubject: Tide pools again\r\nContent-Type: multipart/mixed; boundary=\"b\"\r\n\r\n"+
		"--b\r\nContent-Type: text/html; charset=utf-8\r\n\r\n"+body+"\r\n"+
		"--b\r\nContent-Type: text/html; charset=utf-8\r\nContent-Disposition: attachment; filename=\"sale.html\"\r\n\r\n<p>Rain boots are on sale at the book fair this week, in every size from toddler to adult.</p>\r\n--b--\r\n")
	for _, root := range []string{first, second} {
		made(t, s, "DOCUMENT", root, "extracted")
		for _, part := range children(s, root) {
			made(t, s, "DOCUMENT", part["id"], "extracted")
		}
	}
	makeAll(t, s, x)
	v := x.snapshot()
	object := func(terminal, source string) string {
		for _, r := range v.rows {
			if r.Terminal == terminal && r.SourceLine == source {
				return r.Object
			}
		}
		t.Fatalf("no extract of %s from %s", terminal, source)
		return ""
	}
	firstBody := object(first, "email body, text/html")
	secondBody := object(second, "email body, text/html")
	sale := object(second, `attached file, text/html, filename "sale.html"`)
	run := s.Model().newRun(Env{Viewer: parent, Now: testNow})
	grouped := func(scores map[string]float64) []string {
		got := []string{}
		for _, r := range v.pick(run, scores, testLimits)["DOCUMENT"] {
			line := []string{}
			for _, ref := range r.Refs {
				line = append(line, ref.Terminal+"|"+ref.Source)
			}
			got = append(got, strings.Join(line, " "))
		}
		return got
	}
	equalLines(t, "the copy ranked before the sale", grouped(map[string]float64{firstBody: 0.9, secondBody: 0.8, sale: 0.7}), []string{
		first + "|email body, text/html " + second + "|email body, text/html",
		second + "|attached file, text/html, filename \"sale.html\"",
	})
	equalLines(t, "the sale ranked before the copy", grouped(map[string]float64{firstBody: 0.9, sale: 0.8, secondBody: 0.7}), []string{
		first + "|email body, text/html",
		second + "|attached file, text/html, filename \"sale.html\" " + second + "|email body, text/html",
	})
}

func TestAnEmailsPartsAreOneResult(t *testing.T) {
	s, queue, bucket, x := searcher(t)
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	extract := sendMail(t, s, pics, "Tide pools", "Bring <b>boots</b> for the tide pools.")
	makeAll(t, s, x)
	x.mu.RLock()
	row := x.rows[extract]
	x.mu.RUnlock()
	results := x.Words(s.Model(), Env{Viewer: parent, Now: testNow}, "boots", testLimits)["DOCUMENT"]
	if len(results) != 1 || len(results[0].Refs) != 1 {
		t.Fatalf("the email's result: %+v", results)
	}
	want := SearchRef{Extract: extract, Document: row.Source, Source: "email body, text/html", Href: row.SourceHref, Terminal: row.Terminal, Name: "Tide pools", Summary: "Sample search entry for Email: Tide pools"}
	if ref := results[0].Refs[0]; ref != want {
		t.Fatalf("the email's ref: %+v, want %+v", ref, want)
	}
	if rows := as(t, s, staff, `(from SEARCH (where (= target "`+extract+`")))`); len(rows) != 1 || rows[0]["source"] != row.Source || rows[0]["terminal"] != row.Terminal {
		t.Fatalf("the search row: %v", rows)
	}
}

func TestFingerprintsMeasureSharedText(t *testing.T) {
	long := strings.Repeat("the class walked to the tide pools and counted sea stars before lunch ", 20)
	if r := resemblance(fingerprint(long), fingerprint(long)); r != 1 {
		t.Errorf("identical texts resemble each other %v", r)
	}
	if r := resemblance(fingerprint(long+"see you friday"), fingerprint(long+"see you monday")); r < searchSame {
		t.Errorf("texts differing by one word resemble each other %v", r)
	}
	if r := resemblance(fingerprint(long), fingerprint("rain boots are on sale at the book fair this week in every size")); r > 0.2 {
		t.Errorf("unrelated texts resemble each other %v", r)
	}
}

func TestTheVectorIndexFindsTheNearest(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	entries := map[string]*SearchEntry{}
	for i := range 3000 {
		v := make([]float32, 64)
		for j := range v {
			v[j] = float32(random.NormFloat64())
		}
		normalize(v)
		entries[fmt.Sprintf("o%04d", i)] = &SearchEntry{Chunks: []SearchChunk{{Vector: v}}}
	}
	index := (&vectorIndex{}).update(entries)
	if len(index.centroids) < 2 {
		t.Fatalf("%d cells for 3000 vectors", len(index.centroids))
	}
	missed := 0
	for i := 0; i < 3000; i += 100 {
		o := fmt.Sprintf("o%04d", i)
		query := entries[o].Chunks[0].Vector
		scores := map[string]float64{}
		index.scan(query, index.order(query)[:searchProbe], scores)
		if math.Abs(scores[o]-1) > 1e-4 {
			missed++
		}
	}
	if missed > 0 {
		t.Fatalf("%d of 30 vectors were not found as their own nearest", missed)
	}
	delete(entries, "o0000")
	entries["o9999"] = &SearchEntry{Chunks: []SearchChunk{{Vector: entries["o0100"].Chunks[0].Vector}}}
	index = index.update(entries)
	query := entries["o0100"].Chunks[0].Vector
	scores := map[string]float64{}
	index.scan(query, index.order(query), scores)
	if _, gone := scores["o0000"]; gone || math.Abs(scores["o9999"]-1) > 1e-4 {
		t.Fatalf("after an update the removed vector scores %v and the added one %v", scores["o0000"], scores["o9999"])
	}
}

func TestEachTableIsScannedUntilItAloneIsFull(t *testing.T) {
	m := sample(t).Model()
	rows := m.SearchInputs(nil)
	random := rand.New(rand.NewPCG(3, 4))
	near := func(sign float32) []float32 {
		v := make([]float32, 16)
		for j := range v {
			v[j] = float32(random.NormFloat64()) * 0.1
		}
		v[0] += sign
		normalize(v)
		return v
	}
	entries := map[string]*SearchEntry{}
	byTable := map[string]map[string]*SearchEntry{"GROUP": {}, "PERSON": {}, "DOCUMENT": {}}
	for _, r := range rows {
		e := &SearchEntry{Summary: "made"}
		switch r.Table {
		case "GROUP":
			for range 50 {
				e.Chunks = append(e.Chunks, SearchChunk{Vector: near(1)})
			}
		case "PERSON":
			e.Chunks = []SearchChunk{{Vector: near(-1)}}
		}
		entries[r.Object] = e
		byTable[r.Table][r.Object] = e
	}
	vectors := map[string]*vectorIndex{}
	for _, table := range searchTables {
		vectors[table] = (&vectorIndex{}).update(byTable[table])
	}
	if len(vectors["GROUP"].centroids) <= searchProbe {
		t.Fatalf("%d group cells; the test needs more than %d", len(vectors["GROUP"].centroids), searchProbe)
	}
	v := buildView(rows, entries, vectors)
	query := near(1)
	got, scanned, _ := v.byMeaning(m.newRun(Env{Viewer: staff, Now: testNow}), query, map[string]int{"GROUP": 1, "PERSON": 1, "DOCUMENT": 0})
	if len(got["GROUP"]) != 1 || len(got["PERSON"]) != 1 {
		t.Fatalf("found %d groups and %d people", len(got["GROUP"]), len(got["PERSON"]))
	}
	if scanned["GROUP"] != searchProbe {
		t.Errorf("scanned %d group cells to fill a limit of one", scanned["GROUP"])
	}
}

func keywordView(t *testing.T, m *Model, keywords map[string][]string) (*searchView, []string) {
	t.Helper()
	rows := m.SearchInputs(nil)
	dated := []string{}
	for _, id := range slices.Sorted(maps.Keys(rows)) {
		if rows[id].Table == "GROUP" && len(dated) < 3 {
			dated = append(dated, id)
		}
	}
	at := map[string]string{"oldest": dated[0], "middle": dated[1], "newest": dated[2]}
	for i, id := range dated {
		r := rows[id]
		r.When = fmt.Sprintf("2026-0%d-01 09:00:00", i+1)
		rows[id] = r
	}
	entries := map[string]*SearchEntry{}
	for _, r := range rows {
		entries[r.Object] = &SearchEntry{Summary: "made", Keywords: []string{"filler"}}
	}
	for name, words := range keywords {
		entries[rows[at[name]].Object].Keywords = words
	}
	return buildView(rows, entries, nil), dated
}

func TestTheNewestOfEqualHitsComesFirst(t *testing.T) {
	m := sample(t).Model()
	v, dated := keywordView(t, m, map[string][]string{"oldest": {"boots"}, "newest": {"boots"}})
	got := hitIDs(v.pick(m.newRun(Env{Viewer: staff, Now: testNow}), v.byWords("boots"), testLimits)["GROUP"])
	if !slices.Equal(got, []string{dated[2], dated[0]}) {
		t.Fatalf("the two holders of boots came back %v, want newest %s then oldest %s", got, dated[2], dated[0])
	}
}

func TestAWeakerWordIsKeptWhenNothingHoldsBoth(t *testing.T) {
	m := sample(t).Model()
	v, dated := keywordView(t, m, map[string][]string{"oldest": {"boots"}, "middle": {"kite"}, "newest": {"kite"}})
	got := hitIDs(v.pick(m.newRun(Env{Viewer: staff, Now: testNow}), v.byWords("boots kite"), testLimits)["GROUP"])
	if !slices.Equal(got, []string{dated[0], dated[2], dated[1]}) {
		t.Fatalf("boots kite found %v, want the rarer boots %s, then the kites %s and %s", got, dated[0], dated[2], dated[1])
	}
}

func TestARemovedExtractLeavesTheIndex(t *testing.T) {
	s, queue, bucket, x := searcher(t)
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	extract := sendMail(t, s, pics, "Tide pools", "Bring <b>boots</b> for the tide pools.")
	makeAll(t, s, x)
	x.mu.RLock()
	object := x.rows[extract].Object
	x.mu.RUnlock()
	if err := commit(s, DocumentsSheet, store.Delete("DOCUMENT", store.Row{"id": extract})); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		_, indexed := x.snapshot().rows[extract]
		held, err := bucket.Exists(context.Background(), object)
		if err != nil {
			t.Fatal(err)
		}
		if !indexed && !held {
			return
		}
	}
	t.Fatalf("the removed extract's entry %s is still indexed or stored", object)
}

func TestSearchKeepsToWhatTheCallerMayRead(t *testing.T) {
	s, _, _, x := searcher(t)
	makeAll(t, s, x)
	m := s.Model()
	if got := hitIDs(x.Words(m, Env{Viewer: parent, Now: testNow}, "picnic", testLimits)["GROUP"]); !slices.Contains(got, "grp00000000040") {
		t.Errorf("the parent's word search for picnic found %v", got)
	}
	if got := hitIDs(x.Words(m, Env{Viewer: guest, Now: testNow}, "picnic", testLimits)["GROUP"]); slices.Contains(got, "grp00000000040") {
		t.Errorf("a guest's word search found the picnic they can't see: %v", got)
	}
	results, err := x.Search(context.Background(), m, Env{Viewer: parent, Now: testNow}, "fall picnic", testLimits)
	if err != nil {
		t.Fatal(err)
	}
	if got := hitIDs(results["GROUP"].Meaning); !slices.Contains(got, "grp00000000040") {
		t.Errorf("the parent's search by meaning for fall picnic found %v", got)
	}
}

func TestAChangedRowLosesItsOldEntry(t *testing.T) {
	s, _, bucket, x := searcher(t)
	makeAll(t, s, x)
	before := s.Model().SearchInputs(nil)["grp00000000040"].Object
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000040"}, store.Row{"location": "the meadow"})); err != nil {
		t.Fatal(err)
	}
	after := s.Model().SearchInputs(nil)["grp00000000040"].Object
	if after == before {
		t.Fatal("a new location left the picnic's input unchanged")
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		held, err := bucket.Exists(context.Background(), before)
		if err != nil {
			t.Fatal(err)
		}
		if !held {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the picnic's old entry %s is still stored", before)
		}
	}
	if held, _ := bucket.Exists(context.Background(), after); held {
		t.Fatal("the picnic's new entry was made unasked")
	}
	makeAll(t, s, x)
	if held, err := bucket.Exists(context.Background(), after); err != nil || !held {
		t.Errorf("the picnic's new entry %s was never made: %v", after, err)
	}
}

func TestAStrayEntryIsRemovedOnceMaking(t *testing.T) {
	intercept.GoogleLogin(t.TempDir())
	intercept.Install(intercept.VertexHost, intercept.Vertex())
	intercept.Install(intercept.ClaudeHost, testkit.SearchClaude())
	vertex, err := artifacts.NewVertex()
	if err != nil {
		t.Fatal(err)
	}
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	stray := SearchObject("a row long gone")
	if err := bucket.Put(context.Background(), stray, "application/json", []byte(`{"summary":"gone"}`)); err != nil {
		t.Fatal(err)
	}
	x := NewSearcher(s, queue, bucket, vertex, testOrigin)
	x.StartMaking("test")
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		held, err := bucket.Exists(context.Background(), stray)
		if err != nil {
			t.Fatal(err)
		}
		if !held {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the stray entry %s is still stored", stray)
		}
	}
}

func TestTheIndexIsATableForSuperAdmins(t *testing.T) {
	s, _, _, x := searcher(t)
	makeAll(t, s, x)
	q := `(from SEARCH (where (= target "grp00000000040")))`
	rows := as(t, s, staff, q)
	if len(rows) != 1 || rows[0]["made"] != "Yes" || rows[0]["summary"] == "" || !strings.Contains(rows[0]["input"], "Fall Picnic") || rows[0]["chunks"] != "1" {
		t.Fatalf("the super admin reads the picnic's entry as %v", rows)
	}
	if n := len(as(t, s, parent, q)); n != 0 {
		t.Errorf("a parent reads %d search entries", n)
	}
	parsed, err := Parse(q)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s.Model().Run(t.Context(), parsed, Env{System: importReader, Now: testNow}).IDs); n != 1 {
		t.Errorf("the import reads %d search entries, want 1", n)
	}
}

func TestSearchAnswersOnceByTable(t *testing.T) {
	s, _, _, x := searcher(t)
	makeAll(t, s, x)
	mux := http.NewServeMux()
	RegisterSearch(mux, s, x, []byte(testImportKey), func() time.Time { return testNow })
	req := httptest.NewRequest(http.MethodPost, "/api/do/search", strings.NewReader(`{"words": "picnic"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testImportKey)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var got map[string]SearchResults
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	if len(got) != len(searchTables) || got["DOCUMENT"].Words == nil || got["PERSON"].Meaning == nil {
		t.Fatalf("the answer's tables: %s", rec.Body)
	}
	i := slices.IndexFunc(got["GROUP"].Words, func(h SearchResult) bool { return h.ID == "grp00000000040" })
	if i < 0 || got["GROUP"].Words[i].Name != "Fall Picnic" || got["GROUP"].Words[i].Href == "" || got["GROUP"].Words[i].Summary == "" {
		t.Fatalf("the picnic's hit: %s", rec.Body)
	}
}

func TestEachTableHasItsOwnLimit(t *testing.T) {
	if limits, err := SearchLimits(nil); err != nil || limits["GROUP"] != SearchLimit || limits["PERSON"] != SearchLimit || limits["DOCUMENT"] != SearchLimit {
		t.Fatalf("the default limits are %v, %v", limits, err)
	}
	for _, bad := range []map[string]int{{"MEMBER": 3}, {"GROUP": -1}} {
		if _, err := SearchLimits(bad); err == nil {
			t.Errorf("limits %v were taken", bad)
		}
	}
	s, _, _, x := searcher(t)
	makeAll(t, s, x)
	env := Env{Viewer: parent, Now: testNow}
	if all := x.Words(s.Model(), env, "picnic", testLimits)["GROUP"]; len(all) < 2 {
		t.Fatalf("the picnic search found %d groups; the test needs two", len(all))
	}
	one, _ := SearchLimits(map[string]int{"GROUP": 1, "PERSON": 0})
	got := x.Words(s.Model(), env, "picnic", one)
	if len(got["GROUP"]) != 1 || len(got["PERSON"]) != 0 {
		t.Fatalf("limited to one group and no people, the search found %d and %d", len(got["GROUP"]), len(got["PERSON"]))
	}
	mux := http.NewServeMux()
	RegisterSearch(mux, s, x, []byte(testImportKey), func() time.Time { return testNow })
	ask := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/do/search", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testImportKey)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	rec := ask(`{"words": "picnic", "limits": {"GROUP": 1}}`)
	var answer map[string]SearchResults
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil || rec.Code != http.StatusOK || len(answer["GROUP"].Words) != 1 {
		t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	if rec := ask(`{"words": "picnic", "limits": {"FAMILY": 1}}`); rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown table's limit answered %d", rec.Code)
	}
}

func TestOnlyTheImportKeyAsksForEntries(t *testing.T) {
	s, _, _, x := searcher(t)
	waitRows(t, s, x)
	mux := http.NewServeMux()
	RegisterSearch(mux, s, x, []byte(testImportKey), func() time.Time { return testNow })
	ask := func(bearer bool) int {
		req := httptest.NewRequest(http.MethodPost, "/api/do/search/make", strings.NewReader(`{"id": "grp00000000040"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		if bearer {
			req.Header.Set("Authorization", "Bearer "+testImportKey)
			mux.ServeHTTP(rec, req)
			return rec.Code
		}
		auth.Fixed("rowan@example.com", mux).ServeHTTP(rec, req)
		return rec.Code
	}
	if code := ask(false); code != http.StatusForbidden {
		t.Errorf("a signed-in person asking for an entry got %d", code)
	}
	if code := ask(true); code != http.StatusOK {
		t.Fatalf("the import key asking for an entry got %d", code)
	}
	waitIndexed(t, x, map[string]SearchRow{"grp00000000040": {Object: s.Model().SearchInputs(nil)["grp00000000040"].Object}})
}
