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
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/intercept"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/workload"
)

var testLimits, _ = SearchLimits(nil)

func testOrigin(app string) string {
	return "https://" + app + ".test"
}

func newSearcher(t *testing.T, claude http.Handler, bucket *blob.Bucket) (*Store, *Searcher) {
	t.Helper()
	intercept.GoogleLogin(t.TempDir())
	intercept.Install(intercept.VertexHost, intercept.Vertex())
	intercept.Install(intercept.ClaudeHost, claude)
	vertex, err := artifacts.NewVertex()
	if err != nil {
		t.Fatal(err)
	}
	s := sample(t)
	x := NewSearcher(s, bucket, vertex)
	x.StartMaking("test")
	return s, x
}

func sampleBucket(t *testing.T) *blob.Bucket {
	t.Helper()
	bucket := blob.NewMemoryBucket()
	if err := bucket.FillFrom("../../sampledata/bucket"); err != nil {
		t.Fatal(err)
	}
	return bucket
}

func searcher(t *testing.T) (*Store, *blob.Bucket, *Searcher) {
	t.Helper()
	bucket := sampleBucket(t)
	s, x := newSearcher(t, testkit.SearchClaude(), bucket)
	return s, bucket, x
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
	t.Fatal("the searcher never indexed every row")
}

func makeAll(t *testing.T, s *Store, x *Searcher) {
	t.Helper()
	waitIndexed(t, x, waitRows(t, s, x))
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

func TestReadyAfterTheStoredEntriesAreIndexed(t *testing.T) {
	s, bucket, x := searcher(t)
	rows := waitRows(t, s, x)
	waitIndexed(t, x, rows)
	vertex, err := artifacts.NewVertex()
	if err != nil {
		t.Fatal(err)
	}
	s2 := sample(t)
	started := NewSearcher(s2, bucket, vertex)
	select {
	case <-started.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("the searcher never became ready")
	}
	v := started.snapshot()
	for id, r := range rows {
		if v.entries[r.Object] == nil {
			t.Errorf("ready without the entry for %s", id)
		}
	}
	if len(v.vectors["DOCUMENT"].centroids) == 0 {
		t.Error("ready without trained cells")
	}
}

func TestEveryRowIsMadeUnasked(t *testing.T) {
	s, bucket, x := searcher(t)
	rows := waitRows(t, s, x)
	waitIndexed(t, x, rows)
	held, err := bucket.List(context.Background(), searchFolder+"/")
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]bool{}
	for _, r := range rows {
		objects[r.Object] = true
	}
	if len(held) != len(objects) {
		t.Fatalf("the bucket holds %d entries for %d inputs", len(held), len(objects))
	}
}

func TestAnEntryAnotherWriterStoredIsTakenUp(t *testing.T) {
	s, bucket, x := searcher(t)
	rows := waitRows(t, s, x)
	waitIndexed(t, x, rows)
	var hash string
	for _, r := range rows {
		hash = r.Object
		break
	}
	x.mu.Lock()
	mine := *x.entries[hash]
	theirs := mine
	mine.Summary = ""
	x.entries[hash] = &mine
	x.mu.Unlock()
	theirs.Summary = "the other revision's summary"
	raw, err := json.Marshal(&theirs)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := bucket.Put(context.Background(), hash, "application/json", raw, mine.generation)
	if err != nil {
		t.Fatal(err)
	}
	x.mu.Lock()
	x.enqueue(hash)
	x.mu.Unlock()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		x.mu.RLock()
		e := x.entries[hash]
		x.mu.RUnlock()
		if e.Summary == theirs.Summary && e.generation == stored {
			return
		}
	}
	t.Fatal("the searcher never took up the entry another writer stored")
}

func TestAnAnswerCutShortIsTriedThreeTimes(t *testing.T) {
	var mu sync.Mutex
	asked := map[string]int{}
	s, x := newSearcher(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, _ := io.ReadAll(r.Body)
		mu.Lock()
		asked[string(request)]++
		mu.Unlock()
		testkit.ClaudeStream(w, `{"summary": "cut`, "max_tokens")
	}), sampleBucket(t))
	rows := waitRows(t, s, x)
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
	s, x := newSearcher(t, testkit.ClaudeReplying(func(request string) string {
		mu.Lock()
		defer mu.Unlock()
		asked[request]++
		if asked[request] == 1 {
			return `{"summary": "x"}`
		}
		return `{"summary": "a thing in the sample, asked twice"}`
	}), sampleBucket(t))
	waitRows(t, s, x)
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

func TestDeletingASearchEntryMakesItAgain(t *testing.T) {
	s, _, x := searcher(t)
	makeAll(t, s, x)
	rows := runAs(t, s.Model(), "", `(from SEARCH (where (= target "grp00000000040")))`).Rows()
	if len(rows) != 1 {
		t.Fatalf("search rows for the event: %v", rows)
	}
	object := rows[0]["object"]
	old := x.snapshot().entries[object]
	b := Batch{Batch: []Edit{{Delete: rows[0]["id"]}}}
	if _, err := Write(context.Background(), s, access.Actor{Email: "maya@example.com"}, Env{Viewer: "per00000000002", Now: testNow}, b); err == nil {
		t.Fatal("a parent deleted a search entry")
	}
	if _, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, b); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if e := x.snapshot().entries[object]; e != nil && e != old && e.Summary != "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the deleted entry was never made again")
		}
	}
}

func TestEveryShownRowHasAnInput(t *testing.T) {
	s, bucket, x := searcher(t)
	makeAll(t, s, x)
	inputs := s.Model().SearchInputs(nil)
	if _, ok := inputs["grp00000000041"]; ok {
		t.Error("a managers group has a search input")
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
	if err := json.Unmarshal(raw, &entry); err != nil || len(entry.Chunks[0].Vector) != artifacts.Dims {
		t.Errorf("the picnic's entry has %d dimensions: %v", len(entry.Chunks[0].Vector), err)
	}
}

func found(t *testing.T, x *Searcher, m *Model, env Env, words string, limits map[string]int) map[string][]SearchResult {
	t.Helper()
	results, err := x.Search(context.Background(), m, env, words, limits)
	if err != nil {
		t.Fatal(err)
	}
	return results
}

func TestAnEmailsMarkdownIsSearchedByThoseItWasSentTo(t *testing.T) {
	s, bucket, x := searcher(t)
	pics := NewPictures(s, bucket)
	NewExtractor(s, bucket, "test")
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
	if got := extractsOf(found(t, x, m, Env{Viewer: parent, Now: testNow}, "tide pools", testLimits)["DOCUMENT"]); !slices.Contains(got, extract) {
		t.Errorf("the Hummingbirds parent's search for tide pools found %v", got)
	}
	if got := extractsOf(found(t, x, m, Env{Viewer: student, Now: testNow}, "tide pools", testLimits)["DOCUMENT"]); slices.Contains(got, extract) {
		t.Errorf("a student the email was not sent to found it: %v", got)
	}
}

func TestEveryChunkCarriesTheHeader(t *testing.T) {
	s, bucket, x := searcher(t)
	pics := NewPictures(s, bucket)
	NewExtractor(s, bucket, "test")
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

func TestFillerWordsAreNotSearched(t *testing.T) {
	if got := searchTerms("Who teaches the Jays? People who are going"); !slices.Equal(got, []string{"teaches", "jays", "people", "going"}) {
		t.Fatalf("the terms are %v", got)
	}
}

func TestListsAdminsManagersSessionsDaysAndCategoriesAreNotSearched(t *testing.T) {
	s := sample(t)
	ids := apply(t, s, []Edit{
		{Insert: "GROUP", As: "clubs", Row: map[string]any{"posting": "members", "replying": "members", "kind": "category", "name": "Clubs", "status": "open", "visible_to": "grp00000000004"}},
		{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "day_part", "name": "Pickup", "parent": "@clubs", "status": "open", "visible_to": "grp00000000004", "start": "2026-11-03 15:15:00", "end": "2026-11-03 15:30:00"}},
		{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "day", "name": "No School", "parent": "@clubs", "status": "open", "visible_to": "grp00000000004", "start": "2026-11-03"}},
		{Insert: "GROUP", As: "going", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Book Club Going", "status": "open"}},
		{Insert: "GROUP", As: "managers", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Book Club Managers", "status": "open"}},
		{Set: "@managers", Cells: map[string]any{"managed_by": "@managers"}},
		{Insert: "GROUP", As: "waitlist", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Book Club Waitlist", "status": "open"}},
		{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "admins", "name": "Book Club Admins", "status": "open"}},
		{Insert: "GROUP", As: "viewers", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Book Club Chat Viewers", "status": "open"}},
		{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Book Club Chat", "status": "open", "parent": "@clubs", "visible_to": "@viewers"}},
		{Insert: "GROUP", As: "club", Row: map[string]any{"posting": "members", "replying": "members", "kind": "event", "name": "Book Club", "parent": "@clubs", "status": "open", "visible_to": "grp00000000004", "rsvp_yes": "@going", "managed_by": "@managers", "waitlist": "@waitlist"}},
		{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "event", "name": "Book Club", "parent": "@club", "status": "open", "visible_to": "grp00000000004", "start": "2026-11-03 15:45:00"}},
	})
	clubs, pickup, noSchool, going, managers, waitlist, admins, viewers, chat, club, meeting := ids[0], ids[1], ids[2], ids[3], ids[4], ids[6], ids[7], ids[8], ids[9], ids[10], ids[11]
	rows := s.Model().SearchInputs(nil)
	for id, want := range map[string]bool{noSchool: false, clubs: false, club: true, meeting: false, going: false, managers: false, waitlist: false, admins: false, viewers: true, chat: true, pickup: false} {
		if _, got := rows[id]; got != want {
			t.Errorf("%s searched %v, want %v", id, got, want)
		}
	}
}

func TestANameOnlyPersonIsFoundByNameAlone(t *testing.T) {
	s, _, x := searcher(t)
	makeAll(t, s, x)
	v := x.snapshot()
	rowan, maya := v.rows["per00000000002"], v.rows["per00000000003"]
	if !rowan.nameOnly() || maya.nameOnly() {
		t.Fatalf("name only: Rowan %v (%q), Maya %v (%q)", rowan.nameOnly(), rowan.Input, maya.nameOnly(), maya.Input)
	}
	placed := v.vectors["PERSON"].placed
	if _, ok := placed[rowan.Object]; ok {
		t.Error("Rowan's name alone is in the vector index")
	}
	if _, ok := placed[maya.Object]; !ok {
		t.Error("Maya is not in the vector index")
	}
	got := found(t, x, s.Model(), Env{Viewer: staff, Now: testNow}, "rowan", testLimits)["PERSON"]
	if len(got) == 0 || got[0].ID != "per00000000002" || got[0].Score != 1 {
		t.Errorf("a search for rowan found %+v", got)
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
	s, bucket, x := searcher(t)
	pics := NewPictures(s, bucket)
	NewExtractor(s, bucket, "test")
	extracts := []string{}
	for _, subject := range []string{"Tide pools", "Tide pools again"} {
		extracts = append(extracts, sendMail(t, s, pics, subject, "Bring <b>boots</b> for the tide pools."))
	}
	makeAll(t, s, x)
	m := s.Model()
	env := Env{Viewer: parent, Now: testNow}
	results := found(t, x, m, env, "tide pools", testLimits)["DOCUMENT"]
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
	s, bucket, x := searcher(t)
	pics := NewPictures(s, bucket)
	NewExtractor(s, bucket, "test")
	tidePools := "Bring boots for the tide pools. We meet at the north end of the beach at nine, walk the rocks with the ranger, count the anemones, sea stars and crabs in each pool, sketch what we find in our field journals, eat lunch on the bluff above the cove and walk back to the bus by one. Pack water, a hat, sunscreen and a change of socks in case a wave comes over the rocks."
	extracts := []string{}
	for i, body := range []string{tidePools + " See you Friday.", tidePools + " See you Monday.", "Rain boots are on sale at the book fair this week, in every size from toddler to adult, with half the money going to the library."} {
		extracts = append(extracts, sendMail(t, s, pics, fmt.Sprintf("Field trip %d", i), body))
	}
	makeAll(t, s, x)
	results := found(t, x, s.Model(), Env{Viewer: parent, Now: testNow}, "field trip", testLimits)["DOCUMENT"]
	together := slices.IndexFunc(results, func(r SearchResult) bool {
		got := extractsOf([]SearchResult{r})
		return slices.Contains(got, extracts[0]) && slices.Contains(got, extracts[1])
	})
	if len(results) != 2 || together < 0 || len(results[together].Refs) != 2 || !slices.Contains(extractsOf(results), extracts[2]) {
		t.Fatalf("the near copies are not one result beside the sale: %+v", results)
	}
}

func TestAHitJoinsItsOwnEmailBeforeItsCopy(t *testing.T) {
	s, bucket, x := searcher(t)
	pics := NewPictures(s, bucket)
	NewExtractor(s, bucket, "test")
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
		for _, r := range v.pick(run, scores, nil, testLimits)["DOCUMENT"] {
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
	s, bucket, x := searcher(t)
	pics := NewPictures(s, bucket)
	NewExtractor(s, bucket, "test")
	extract := sendMail(t, s, pics, "Tide pools", "Bring <b>boots</b> for the tide pools.")
	makeAll(t, s, x)
	x.mu.RLock()
	row := x.rows[extract]
	x.mu.RUnlock()
	results := found(t, x, s.Model(), Env{Viewer: parent, Now: testNow}, "tide pools", testLimits)["DOCUMENT"]
	if len(results) != 1 || len(results[0].Refs) != 1 {
		t.Fatalf("the email's result: %+v", results)
	}
	want := SearchRef{Extract: extract, Document: row.Source, Source: "email body, text/html", Href: row.SourceHref, Terminal: row.Terminal, Name: "Tide pools", Summary: "Sample search entry for Email: Tide pools", Score: 1}
	if ref := results[0].Refs[0]; ref != want {
		t.Fatalf("the email's ref: %+v, want %+v", ref, want)
	}
	if rows := as(t, s, staff, `(from SEARCH (where (= target "`+extract+`")))`); len(rows) != 1 || rows[0]["source"] != row.Source || rows[0]["terminal"] != row.Terminal {
		t.Fatalf("the search row: %v", rows)
	}
}

func TestARefNamesThePagesItWasReadFrom(t *testing.T) {
	s, bucket, x := searcher(t)
	if _, err := bucket.Put(t.Context(), "content/m1", "text/markdown", []byte("# Egg drop\n\nEach team drops one egg from the balcony."), 0); err != nil {
		t.Fatal(err)
	}
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "whole", Row: map[string]any{"hash": "p0", "blob": "content/p0", "mime": pdfType, "size": "100"}},
		{Insert: "CONTENT", As: "run", Row: map[string]any{"hash": "p2", "blob": "content/p2", "mime": pdfType, "size": "40"}},
		{Insert: "CONTENT", As: "text", Row: map[string]any{"hash": "m1", "blob": "content/m1", "mime": "text/markdown", "size": "52"}},
		{Insert: "DOCUMENT", As: "rules", Row: map[string]any{"kind": "file", "content": "@whole", "name": "Science Olympiad Rules", "extracted": "2026-02-12 01:48:03"}},
		{Insert: "DOCUMENT", As: "pages", Row: map[string]any{"relation": "pages", "parent": "@rules", "content": "@run", "name": "pages 21–25 of 25", "first_page": "21", "last_page": "25", "extracted": "2026-02-12 01:48:03"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "extract", "parent": "@pages", "content": "@text"}},
	})
	pages, extract := ids[4], ids[5]
	makeAll(t, s, x)
	results := found(t, x, s.Model(), Env{Viewer: parent, Now: testNow}, "science olympiad rules", testLimits)["DOCUMENT"]
	if len(results) != 1 || len(results[0].Refs) != 1 {
		t.Fatalf("the rules' result: %+v", results)
	}
	if ref := results[0].Refs[0]; ref.Extract != extract || ref.Document != ids[3] || ref.FirstPage != 21 || ref.LastPage != 25 {
		t.Fatalf("the rules' ref: %+v, want %s read from pages 21 to 25 of %s", ref, extract, ids[3])
	}
	apply(t, s, []Edit{{Set: pages, Cells: map[string]any{"first_page": "22"}}})
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		x.mu.RLock()
		first := x.rows[extract].FirstPage
		x.mu.RUnlock()
		if first == 22 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the extract's row still starts at page %d after its run moved to 22", first)
		}
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
			e.Chunks = []SearchChunk{{Vector: near(1)}}
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
	run := m.newRun(Env{Viewer: staff, Now: testNow})
	limits := map[string]int{"GROUP": 1, "PERSON": 1, "DOCUMENT": 0}
	scores, scanned, _ := v.byMeaning(run, query, limits)
	got := v.pick(run, scores, nil, limits)
	if len(got["GROUP"]) != 1 || len(got["PERSON"]) != 1 {
		t.Fatalf("found %d groups and %d people", len(got["GROUP"]), len(got["PERSON"]))
	}
	if scanned["GROUP"] != searchProbe {
		t.Errorf("scanned %d group cells to fill a limit of one", scanned["GROUP"])
	}
}

func namedView(t *testing.T, m *Model, names map[string]string) (*searchView, []string) {
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
	for name, title := range names {
		r := rows[at[name]]
		r.Name = title
		rows[at[name]] = r
	}
	entries := map[string]*SearchEntry{}
	for _, r := range rows {
		entries[r.Object] = &SearchEntry{Summary: "made"}
	}
	return buildView(rows, entries, nil), dated
}

func byNameOf(v *searchView, run *run, words string) []string {
	scores, exact := v.byName(words)
	return hitIDs(v.pick(run, scores, exact, testLimits)["GROUP"])
}

func TestTheNewestOfEqualHitsComesFirst(t *testing.T) {
	m := sample(t).Model()
	v, dated := namedView(t, m, map[string]string{"oldest": "Rain Boots Sale", "newest": "Boots for the Tide Pools"})
	got := byNameOf(v, m.newRun(Env{Viewer: staff, Now: testNow}), "boots")
	if !slices.Equal(got, []string{dated[2], dated[0]}) {
		t.Fatalf("the two named boots came back %v, want newest %s then oldest %s", got, dated[2], dated[0])
	}
}

func TestANameThatIsTheWordsComesFirst(t *testing.T) {
	m := sample(t).Model()
	v, dated := namedView(t, m, map[string]string{"oldest": "The Boots", "middle": "Boots for the Tide Pools", "newest": "Rain Boots Sale"})
	got := byNameOf(v, m.newRun(Env{Viewer: staff, Now: testNow}), "Boots")
	if !slices.Equal(got, []string{dated[0], dated[2], dated[1]}) {
		t.Fatalf("boots found %v, want the one named boots %s, then the newest %s, then %s", got, dated[0], dated[2], dated[1])
	}
}

func TestANameMustHoldEveryWord(t *testing.T) {
	m := sample(t).Model()
	v, dated := namedView(t, m, map[string]string{"oldest": "Rain Boots Sale", "middle": "Kite Day", "newest": "Boots and Kites"})
	got := byNameOf(v, m.newRun(Env{Viewer: staff, Now: testNow}), "the boots kites")
	if !slices.Equal(got, []string{dated[2]}) {
		t.Fatalf("boots kites found %v, want only %s", got, dated[2])
	}
	if got, _ := v.byName("who is the"); len(got) != 0 {
		t.Fatalf("filler alone found %v", got)
	}
}

func TestAWikiPageIsSearchedAsItself(t *testing.T) {
	s, _, x := searcher(t)
	makeAll(t, s, x)
	results := found(t, x, s.Model(), Env{Viewer: parent, Now: testNow}, "drop-off and pickup", testLimits)["DOCUMENT"]
	if len(results) == 0 || len(results[0].Refs) != 1 {
		t.Fatalf("the wiki page's result: %+v", results)
	}
	ref := results[0].Refs[0]
	if ref.Extract != "doc00000000104" || ref.Document != "doc00000000104" || ref.Terminal != "doc00000000104" || ref.Source != "" || ref.Name != "Drop-off and Pickup" || ref.Href == "" || ref.Score != 1 {
		t.Fatalf("the wiki page's ref: %+v", ref)
	}
	x.mu.RLock()
	row := x.rows["doc00000000104"]
	x.mu.RUnlock()
	if !strings.HasPrefix(row.Input, "Wiki page: Drop-off and Pickup\nKind: wiki\nUpdated: 2026-09-01 09:00:00\n") {
		t.Fatalf("the wiki page's input starts %.120q", row.Input)
	}
}

func TestAResultKeepsFiveRefs(t *testing.T) {
	s, bucket, x := searcher(t)
	pics := NewPictures(s, bucket)
	NewExtractor(s, bucket, "test")
	for i := range 7 {
		sendMail(t, s, pics, fmt.Sprintf("Tide pools %d", i), "Bring <b>boots</b> for the tide pools.")
	}
	makeAll(t, s, x)
	results := found(t, x, s.Model(), Env{Viewer: parent, Now: testNow}, "tide pools", testLimits)["DOCUMENT"]
	if len(results) != 1 || len(results[0].Refs) != searchRefs {
		t.Fatalf("seven copies came back as %d results, the first with %d refs", len(results), len(results[0].Refs))
	}
}

func TestARemovedExtractLeavesTheIndex(t *testing.T) {
	s, bucket, x := searcher(t)
	pics := NewPictures(s, bucket)
	NewExtractor(s, bucket, "test")
	extract := sendMail(t, s, pics, "Tide pools", "Bring <b>boots</b> for the tide pools.")
	makeAll(t, s, x)
	x.mu.RLock()
	object := x.rows[extract].Object
	x.mu.RUnlock()
	apply(t, s, []Edit{{Delete: extract}})
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
	s, _, x := searcher(t)
	makeAll(t, s, x)
	m := s.Model()
	if got := hitIDs(found(t, x, m, Env{Viewer: parent, Now: testNow}, "picnic", testLimits)["GROUP"]); !slices.Contains(got, "grp00000000040") {
		t.Errorf("the parent's search for picnic found %v", got)
	}
	if got := hitIDs(found(t, x, m, Env{Viewer: guest, Now: testNow}, "picnic", testLimits)["GROUP"]); slices.Contains(got, "grp00000000040") {
		t.Errorf("a guest's search found the picnic they can't see: %v", got)
	}
}

func TestMeaningStopsAtTheFloor(t *testing.T) {
	s, _, x := searcher(t)
	makeAll(t, s, x)
	v := x.snapshot()
	picnic := v.rows["grp00000000040"]
	near := v.entries[picnic.Object].Chunks[0].Vector
	far := make([]float32, len(near))
	far[len(far)-1] = 1
	run := s.Model().newRun(Env{Viewer: staff, Now: testNow})
	if scores, _, _ := v.byMeaning(run, near, testLimits); scores[picnic.Object] < searchFloor {
		t.Errorf("the picnic's own vector scores %v", scores[picnic.Object])
	}
	if scores, _, _ := v.byMeaning(run, far, testLimits); len(scores) != 0 {
		t.Errorf("a vector near nothing found %d entries", len(scores))
	}
}

func TestAChangedRowLosesItsOldEntry(t *testing.T) {
	s, bucket, x := searcher(t)
	makeAll(t, s, x)
	before := s.Model().SearchInputs(nil)["grp00000000040"].Object
	apply(t, s, []Edit{{Set: "grp00000000040", Cells: map[string]any{"location": "the meadow"}}})
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
	makeAll(t, s, x)
	if held, err := bucket.Exists(context.Background(), after); err != nil || !held {
		t.Errorf("the picnic's new entry %s was never made: %v", after, err)
	}
}

func TestARenamedClassroomReachesItsPeoplesEntries(t *testing.T) {
	s, _, x := searcher(t)
	makeAll(t, s, x)
	students := s.Model().Table("PERSON").Referencing("classroom", "grp00000000010")
	if len(students) == 0 {
		t.Fatal("the sample has nobody in grp00000000010")
	}
	before := s.Model().SearchInputs(nil)[students[0]["id"]].Object
	apply(t, s, []Edit{{Set: "grp00000000010", Cells: map[string]any{"name": "Sea Otters"}}})
	rows := waitRows(t, s, x)
	if rows[students[0]["id"]].Object == before || !strings.Contains(rows[students[0]["id"]].Input, "Classroom: Sea Otters") {
		t.Fatalf("%s's input after the rename: %q", students[0]["id"], rows[students[0]["id"]].Input)
	}
}

func TestIncrementalSearchAgreesWithAFullPass(t *testing.T) {
	s, _, x := searcher(t)
	waitRows(t, s, x)
	m := s.Model()
	groups, classrooms, people, documents := []string{}, []string{}, []string{}, []string{}
	for _, g := range m.Shown("GROUP").All() {
		groups = append(groups, g["id"])
		if g["kind"] == "classroom" {
			classrooms = append(classrooms, g["id"])
		}
	}
	for _, p := range m.Shown("PERSON").All() {
		people = append(people, p["id"])
	}
	for _, d := range m.Shown("DOCUMENT").All() {
		if d["kind"] != "" {
			documents = append(documents, d["id"])
		}
	}
	rng := rand.New(rand.NewPCG(1, 2))
	pick := func(ids []string) string { return ids[rng.IntN(len(ids))] }
	for i := range 40 {
		edit := Edit{}
		switch rng.IntN(4) {
		case 0:
			edit = Edit{Set: pick(groups), Cells: map[string]any{"name": fmt.Sprintf("Renamed %d", i)}}
		case 1:
			edit = Edit{Set: pick(people), Cells: map[string]any{"classroom_override": pick(classrooms)}}
		case 2:
			edit = Edit{Set: pick(people), Cells: map[string]any{"name_long_override": fmt.Sprintf("Someone Renamed%d", i)}}
		case 3:
			edit = Edit{Set: pick(documents), Cells: map[string]any{"name": fmt.Sprintf("Retitled %d", i)}}
		}
		t.Logf("edit %d: %s %v", i, edit.Set, edit.Cells)
		apply(t, s, []Edit{edit})
		waitRows(t, s, x)
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
	s := sample(t)
	bucket := sampleBucket(t)
	stray := SearchObject("a row long gone")
	if _, err := bucket.Put(context.Background(), stray, "application/json", []byte(`{"summary":"gone"}`), 0); err != nil {
		t.Fatal(err)
	}
	x := NewSearcher(s, bucket, vertex)
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
	s, _, x := searcher(t)
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
	if n := len(s.Model().Run(t.Context(), parsed, setupEnv).IDs); n != 1 {
		t.Errorf("the import reads %d search entries, want 1", n)
	}
}

func TestSearchAnswersOnceByTable(t *testing.T) {
	s, _, x := searcher(t)
	makeAll(t, s, x)
	mux := http.NewServeMux()
	RegisterSearch(mux, s, x, testTokens)
	req := httptest.NewRequest(http.MethodPost, "/api/do/search", strings.NewReader(`{"words": "picnic"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	serveAs(mux, rec, req, importing)
	var got map[string][]SearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	if len(got) != len(searchTables) || got["DOCUMENT"] == nil || got["PERSON"] == nil {
		t.Fatalf("the answer's tables: %s", rec.Body)
	}
	i := slices.IndexFunc(got["GROUP"], func(h SearchResult) bool { return h.ID == "grp00000000040" })
	if i < 0 || got["GROUP"][i].Name != "Fall Picnic" || got["GROUP"][i].Href == "" || got["GROUP"][i].Summary == "" || got["GROUP"][i].Score != 1 {
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
	s, _, x := searcher(t)
	apply(t, s, []Edit{{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "event", "name": "Spring Picnic", "parent": "grp00000000060", "status": "open", "visible_to": "grp00000000004", "start": "2027-04-10 12:00:00"}}})
	makeAll(t, s, x)
	env := Env{Viewer: parent, Now: testNow}
	if all := found(t, x, s.Model(), env, "picnic", testLimits)["GROUP"]; len(all) < 2 {
		t.Fatalf("the picnic search found %d groups; the test needs two", len(all))
	}
	one, _ := SearchLimits(map[string]int{"GROUP": 1, "PERSON": 0})
	got := found(t, x, s.Model(), env, "picnic", one)
	if len(got["GROUP"]) != 1 || len(got["PERSON"]) != 0 {
		t.Fatalf("limited to one group and no people, the search found %d and %d", len(got["GROUP"]), len(got["PERSON"]))
	}
	mux := http.NewServeMux()
	RegisterSearch(mux, s, x, testTokens)
	ask := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/do/search", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		serveAs(mux, rec, req, importing)
		return rec
	}
	rec := ask(`{"words": "picnic", "limits": {"GROUP": 1}}`)
	var answer map[string][]SearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil || rec.Code != http.StatusOK || len(answer["GROUP"]) != 1 {
		t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	if rec := ask(`{"words": "picnic", "limits": {"FAMILY": 1}}`); rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown table's limit answered %d", rec.Code)
	}
}

func TestSearchPrefersCurrentDeckAndProtectsReplacement(t *testing.T) {
	s := sample(t)
	ids := apply(t, s, []Edit{
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "name": "Gradeband updates week one"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "name": "Gradeband updates week two", "url": "https://example.org/new.pdf"}},
	})
	extracts := apply(t, s, []Edit{
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "extract", "parent": ids[0], "content": "cnt00000000101"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "extract", "parent": ids[1], "content": "cnt00000000102"}},
	})
	apply(t, s, []Edit{{Set: ids[0], Cells: map[string]any{"superseded_by": ids[1]}}})
	rows := map[string]SearchRow{}
	entries := map[string]*SearchEntry{}
	scores := map[string]float64{}
	for i, id := range extracts {
		object := fmt.Sprintf("object%d", i)
		rows[id] = SearchRow{Table: "DOCUMENT", Object: object, Source: id, Terminal: ids[i], Name: "Gradeband updates"}
		entries[object] = &SearchEntry{Summary: "update"}
		scores[object] = 1 - float64(i)*0.1
	}
	v := buildView(rows, entries, nil)
	pick := func() []SearchResult {
		return v.pick(s.Model().newRun(Env{Viewer: parent, Now: testNow}), scores, nil, testLimits)["DOCUMENT"]
	}
	got := pick()
	if len(got) != 2 || got[0].Refs[0].Terminal != ids[1] || got[1].Refs[0].SupersededBy != ids[1] || got[1].Refs[0].ReplacementHref != "https://example.org/new.pdf" {
		t.Fatalf("replacement ranking: %+v", got)
	}
	apply(t, s, []Edit{{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": ids[1], "group": "grp00000000031", "relation": "sent_to"}}})
	got = pick()
	if len(got) != 1 || got[0].Refs[0].Terminal != ids[0] || got[0].Refs[0].SupersededBy != "" || got[0].Refs[0].ReplacementHref != "" {
		t.Fatalf("unreadable replacement: %+v", got)
	}
}

func TestMeaningScansPastSupersededMatches(t *testing.T) {
	s := sample(t)
	ids := apply(t, s, []Edit{
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "name": "Previous deck"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "name": "Current deck"}},
	})
	extracts := apply(t, s, []Edit{
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "extract", "parent": ids[0], "content": "cnt00000000101"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "extract", "parent": ids[1], "content": "cnt00000000102"}},
	})
	apply(t, s, []Edit{{Set: ids[0], Cells: map[string]any{"superseded_by": ids[1]}}})
	rows := map[string]SearchRow{}
	entries := map[string]*SearchEntry{}
	for i, id := range extracts {
		object := fmt.Sprintf("deck%d", i)
		rows[id] = SearchRow{Table: "DOCUMENT", Object: object, Source: id, Terminal: ids[i]}
		entries[object] = &SearchEntry{Summary: "update"}
	}
	index := &vectorIndex{}
	for i := range searchProbe + 1 {
		vector := []float32{1, 0}
		object := "deck0"
		if i == searchProbe {
			vector = []float32{0.9, float32(math.Sqrt(1 - 0.9*0.9))}
			object = "deck1"
		}
		index.centroids = append(index.centroids, vector)
		index.cells = append(index.cells, []chunkRef{{object: object, vector: vector}})
	}
	vectors := map[string]*vectorIndex{"GROUP": {}, "PERSON": {}, "DOCUMENT": index}
	v := buildView(rows, entries, vectors)
	limits := map[string]int{"DOCUMENT": 1}
	pick := func() ([]SearchResult, int) {
		r := s.Model().newRun(Env{Viewer: parent, Now: testNow})
		scores, scanned, _ := v.byMeaning(r, []float32{1, 0}, limits)
		return v.pick(r, scores, nil, limits)["DOCUMENT"], scanned["DOCUMENT"]
	}
	got, scanned := pick()
	if scanned != searchProbe+1 || len(got) != 1 || got[0].Refs[0].Terminal != ids[1] {
		t.Fatalf("scanned %d cells, results %+v", scanned, got)
	}
	apply(t, s, []Edit{{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": ids[1], "group": "grp00000000031", "relation": "sent_to"}}})
	got, scanned = pick()
	if scanned != searchProbe || len(got) != 1 || got[0].Refs[0].Terminal != ids[0] || got[0].Refs[0].SupersededBy != "" {
		t.Fatalf("unreadable replacement: scanned %d cells, results %+v", scanned, got)
	}
}

func TestLoadedReplacementChainResolvesToLatest(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	if err := dir.Insert(DocumentsSheet, "DOCUMENT", testkit.DeckRevisions()); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(dir, dir, NewSearchIndex(), testOrigin)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]SearchRow{"doc00000000904": {Table: "DOCUMENT", Object: "deck", Source: "doc00000000904", Terminal: "doc00000000901"}}
	v := buildView(rows, map[string]*SearchEntry{"deck": {Summary: "previous update"}}, nil)
	check := func(want string) {
		t.Helper()
		env := Env{Viewer: parent, Now: testNow}
		resolved := s.Model().DocumentReplacements(env, []string{"doc00000000901", "doc00000000902", "doc00000000904"})
		for _, id := range []string{"doc00000000901", "doc00000000902", "doc00000000904"} {
			if resolved[id] != want {
				t.Fatalf("%s resolves to %q, want %q", id, resolved[id], want)
			}
		}
		got := v.pick(s.Model().newRun(env), map[string]float64{"deck": 1}, nil, testLimits)["DOCUMENT"]
		if len(got) != 1 || got[0].Refs[0].SupersededBy != want {
			t.Fatalf("search refs: %+v", got)
		}
	}
	check("doc00000000903")
	original, _ := s.Model().Table("DOCUMENT").Get("doc00000000901")
	if original["superseded_by"] != "doc00000000902" {
		t.Fatal("reader rewrote the sheet's relationship")
	}
	apply(t, s, []Edit{{Set: staff, Cells: map[string]any{"consent": "withheld"}}})
	check("")
	apply(t, s, []Edit{{Set: staff, Cells: map[string]any{"consent": "listed"}}})
	check("doc00000000903")
	apply(t, s, []Edit{{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": "doc00000000903", "group": "grp00000000031", "relation": "sent_to"}}})
	check("")
}

func TestSearchProcessingIncludesCommittedAndUnpublishedWork(t *testing.T) {
	index := NewSearchIndex()
	m := &Model{index: index, services: newServices(index)}
	x := &Searcher{SearchIndex: index, s: &Store{Work: workload.New()}, ready: make(chan struct{})}
	if m.searchProcessing().Pending != 1 {
		t.Fatal("an unbuilt index was reported idle")
	}
	go x.index()
	index.reindex.poke()
	wait := func() {
		t.Helper()
		for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			if m.searchProcessing().Pending == 0 {
				return
			}
		}
		t.Fatal("search processing did not drain")
	}
	wait()
	in := m.services.inbox(searchInbox)
	in.deliver(delivery{})
	if m.searchProcessing().Pending != 1 {
		t.Fatal("a committed change was reported idle before the follower read it")
	}
	index.mu.Lock()
	index.following = true
	index.mu.Unlock()
	in.take()
	if m.searchProcessing().Pending != 1 {
		t.Fatal("a change being followed was reported idle")
	}
	index.mu.Lock()
	index.following = false
	index.version++
	index.mu.Unlock()
	if m.searchProcessing().Pending != 1 {
		t.Fatal("a version not yet published in the in-memory index was reported idle")
	}
	index.reindex.poke()
	wait()
	index.mu.Lock()
	index.rows = map[string]SearchRow{"row": {Object: "object"}}
	index.entries["object"] = &SearchEntry{}
	index.mu.Unlock()
	if m.searchProcessing().Pending != 1 {
		t.Fatal("an unfinished search entry was reported idle")
	}
}

func TestThumbnailWritesLeaveTheSearchIndexUnchanged(t *testing.T) {
	s := sample(t)
	bucket := sampleBucket(t)
	x := &Searcher{SearchIndex: s.Model().index, s: s, bucket: bucket, texts: map[string]string{}, ready: make(chan struct{})}
	x.rebuild()
	x.followChanges()
	go x.index()
	wait := func() {
		t.Helper()
		for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			if s.Model().searchProcessing().Pending == 0 {
				return
			}
		}
		t.Fatal("index did not finish rebuilding")
	}
	wait()
	version, view := x.current(), x.snapshot()
	document := ""
	for _, row := range view.rows {
		if row.Table == "DOCUMENT" {
			document = row.Terminal
			break
		}
	}
	if document == "" {
		t.Fatal("fixture has no searchable documents")
	}
	thumbnail, err := blob.Thumbnail(redPNG(t, 120, 180))
	if err != nil {
		t.Fatal(err)
	}
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "thumbnail", Row: storedContent(t, bucket, "image/jpeg", thumbnail)},
		{Set: document, Cells: map[string]any{"thumbnail": "@thumbnail", "thumbnailed": "2026-10-27 12:00:00"}},
	})
	if len(ids) == 0 || s.Model().searchProcessing().Pending == 0 {
		t.Fatal("thumbnail write did not reach search")
	}
	x.followChanges()
	if x.current() != version || x.snapshot() != view || len(x.reindex) != 0 {
		t.Fatal("thumbnail-only change rebuilt the search index")
	}
	x.rebuild()
	x.followChanges()
	if x.current() != version || x.snapshot() != view || len(x.reindex) != 0 {
		t.Fatal("unchanged full pass rebuilt the search index")
	}
	apply(t, s, []Edit{{Set: document, Cells: map[string]any{"name": "A changed searchable document title"}}})
	x.followChanges()
	wait()
	if x.current() == version || x.snapshot() == view {
		t.Fatal("searchable title change did not rebuild the index")
	}
	found := false
	for _, row := range x.snapshot().rows {
		if row.Terminal == document && row.Name == "A changed searchable document title" {
			found = true
		}
	}
	if !found {
		t.Fatal("rebuilt index lacks the changed document title")
	}
}
