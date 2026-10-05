package db

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/intercept"
	"heliosian/internal/store"
)

func instantClaude(w http.ResponseWriter, r *http.Request) {
	claudeReplying(func(string) string {
		return `{"summary": "a thing in the sample", "keywords": ["outing", "lunch"]}`
	})(w, r)
}

func claudeReplying(reply func(request string) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, _ := io.ReadAll(r.Body)
		claudeStream(w, reply(string(request)))
	}
}

func claudeStream(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	answer, _ := json.Marshal(text)
	for _, event := range []string{
		`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":` + string(answer) + `}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":0}}`,
		`{"type":"message_stop"}`,
	} {
		var kind struct {
			Type string `json:"type"`
		}
		json.Unmarshal([]byte(event), &kind)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind.Type, event)
	}
}

func searcher(t *testing.T) (*Store, *store.Queue, *blob.Bucket, *Searcher) {
	t.Helper()
	intercept.GoogleLogin(t.TempDir())
	intercept.Install(intercept.VertexHost, intercept.Vertex())
	intercept.Install(intercept.ClaudeHost, http.HandlerFunc(instantClaude))
	vertex, err := artifacts.NewVertex()
	if err != nil {
		t.Fatal(err)
	}
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	x := NewSearcher(s, queue, bucket, vertex)
	x.StartMaking("test")
	return s, queue, bucket, x
}

func madeAll(t *testing.T, s *Store, x *Searcher) {
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
			if x.rows[id].Object != r.Object || x.entries[r.Object] == nil {
				done = false
			}
		}
		x.mu.RUnlock()
		if done {
			return
		}
	}
	t.Fatal("the searcher never made every row's entry")
}

func hitIDs(hits []SearchHit) []string {
	out := []string{}
	for _, h := range hits {
		out = append(out, h.ID)
	}
	return out
}

func TestEveryShownRowGetsAnEntry(t *testing.T) {
	s, _, bucket, x := searcher(t)
	madeAll(t, s, x)
	m := s.Model()
	inputs := m.SearchInputs(nil)
	if _, ok := inputs["grp00000000041"]; !ok {
		t.Error("a managers group has no search input")
	}
	picnic := inputs["grp00000000040"]
	if !strings.Contains(picnic.Input, "Fall Picnic") || !strings.Contains(picnic.Input, "Under: Community") {
		t.Errorf("the picnic's input is %q", picnic.Input)
	}
	if held, err := bucket.Exists(context.Background(), picnic.Object); err != nil || !held || !strings.HasPrefix(picnic.Object, "search/") {
		t.Errorf("the picnic's entry %s is not in the bucket: %v", picnic.Object, err)
	}
}

func TestAnEmailsMarkdownIsSearchedByThoseItWasSentTo(t *testing.T) {
	s, queue, bucket, x := searcher(t)
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket)
	root := uploadMail(t, s, pics, "From: Maya Lindqvist <maya.lindqvist@example.org>\r\nDate: Thu, 12 Feb 2026 01:48:03 +0000\r\nSubject: Tide pools\r\nList-Id: <hummingbirds.parents.heliosschool.org>\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Bring <b>boots</b> for the tide pools.</p>\r\n")
	made(t, s, "DOCUMENT", root, "extracted")
	part := children(s, root)[0]
	made(t, s, "DOCUMENT", part["id"], "extracted")
	extract := children(s, part["id"])[0]["id"]
	madeAll(t, s, x)
	x.mu.RLock()
	input := x.rows[extract].Input
	x.mu.RUnlock()
	for _, want := range []string{"Email: Tide pools", "Kind: list", "From: Maya Lindqvist", "Part: text/html", "Bring **boots** for the tide pools."} {
		if !strings.Contains(input, want) {
			t.Errorf("the extract's input lacks %q:\n%s", want, input)
		}
	}
	m := s.Model()
	if got := hitIDs(x.Words(m, Env{Viewer: parent, Now: testNow}, "boots")); !slices.Contains(got, extract) {
		t.Errorf("the Hummingbirds parent's search for boots found %v", got)
	}
	if got := hitIDs(x.Words(m, Env{Viewer: student, Now: testNow}, "boots")); slices.Contains(got, extract) {
		t.Errorf("a student the email was not sent to found it: %v", got)
	}
}

func TestSearchKeepsToWhatTheCallerMayRead(t *testing.T) {
	s, _, _, x := searcher(t)
	madeAll(t, s, x)
	m := s.Model()
	if got := hitIDs(x.Words(m, Env{Viewer: parent, Now: testNow}, "picnic")); !slices.Contains(got, "grp00000000040") {
		t.Errorf("the parent's word search for picnic found %v", got)
	}
	if got := hitIDs(x.Words(m, Env{Viewer: guest, Now: testNow}, "picnic")); slices.Contains(got, "grp00000000040") {
		t.Errorf("a guest's word search found the picnic they can't see: %v", got)
	}
	meaning, err := x.Meaning(context.Background(), m, Env{Viewer: parent, Now: testNow}, "fall picnic")
	if err != nil {
		t.Fatal(err)
	}
	if got := hitIDs(meaning); !slices.Contains(got, "grp00000000040") {
		t.Errorf("the parent's search by meaning for fall picnic found %v", got)
	}
}

func TestAChangedRowIsMadeAgain(t *testing.T) {
	s, _, bucket, x := searcher(t)
	madeAll(t, s, x)
	before := s.Model().SearchInputs(nil)["grp00000000040"].Object
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000040"}, store.Row{"location": "the meadow"})); err != nil {
		t.Fatal(err)
	}
	after := s.Model().SearchInputs(nil)["grp00000000040"].Object
	if after == before {
		t.Fatal("a new location left the picnic's input unchanged")
	}
	madeAll(t, s, x)
	if held, err := bucket.Exists(context.Background(), after); err != nil || !held {
		t.Errorf("the picnic's new entry %s was never made: %v", after, err)
	}
}

func TestTheIndexIsATableForSuperAdmins(t *testing.T) {
	s, _, _, x := searcher(t)
	madeAll(t, s, x)
	q := `(from SEARCH (where (= target "grp00000000040")))`
	rows := as(t, s, staff, q)
	if len(rows) != 1 || rows[0]["made"] != "Yes" || rows[0]["summary"] == "" || !strings.Contains(rows[0]["input"], "Fall Picnic") || rows[0]["chunks"] != "1" {
		t.Fatalf("the super admin reads the picnic's entry as %v", rows)
	}
	if n := len(as(t, s, parent, q)); n != 0 {
		t.Errorf("a parent reads %d search entries", n)
	}
}

func TestSearchStreamsWordsThenMeaning(t *testing.T) {
	s, _, _, x := searcher(t)
	madeAll(t, s, x)
	mux := http.NewServeMux()
	RegisterSearch(mux, s, x, []byte(testImportKey), func() time.Time { return testNow })
	req := httptest.NewRequest(http.MethodPost, "/api/do/search", strings.NewReader(`{"words": "picnic"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testImportKey)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	words, meaning := strings.Index(body, "event: words"), strings.Index(body, "event: meaning")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" || words < 0 || meaning < words || !strings.Contains(body, "grp00000000040") {
		t.Fatalf("%d %s", rec.Code, body)
	}
}
