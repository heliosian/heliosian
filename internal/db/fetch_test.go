package db

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/intercept"
)

func TestOnlyALastingFailureIsUnreachable(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		gone bool
	}{
		{"no such host", &url.Error{Op: "Get", URL: "http://r560896.example.org/c", Err: &net.OpError{Op: "dial", Err: &net.DNSError{Err: "no such host", Name: "r560896.example.org", IsNotFound: true}}}, true},
		{"a lame referral", &url.Error{Op: "Get", URL: "http://example.org/", Err: &net.OpError{Op: "dial", Err: &net.DNSError{Err: "lame referral", Name: "example.org"}}}, true},
		{"a dns server that timed out", &url.Error{Op: "Get", URL: "http://example.org/", Err: &net.DNSError{Err: "i/o timeout", Name: "example.org", IsTimeout: true}}, false},
		{"a dns server that failed", &url.Error{Op: "Get", URL: "http://example.org/", Err: &net.DNSError{Err: "server misbehaving", Name: "example.org", IsTemporary: true}}, false},
		{"an untrusted certificate", &url.Error{Op: "Get", URL: "https://example.org/", Err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}}, true},
		{"a refused connection", &url.Error{Op: "Get", URL: "http://example.org/", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}, false},
		{"no error", nil, false},
	} {
		if got := Unreachable(c.err); got != c.gone {
			t.Errorf("%s: gone %v, want %v", c.name, got, c.gone)
		}
	}
}

func TestOneFetchFillsEveryDocumentOfAnAddress(t *testing.T) {
	logo := pngOf(t, 3)
	asked := atomic.Int32{}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Write(logo)
	}))
	defer site.Close()
	s, queue := sampleWithQueue(t)
	ids := apply(t, s, queue, []Edit{
		{Insert: "DOCUMENT", As: "this", Row: map[string]any{"kind": "mail", "name": "Clubs this week"}},
		{Insert: "DOCUMENT", As: "next", Row: map[string]any{"kind": "mail", "name": "Clubs next week"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": "@this", "url": site.URL + "/logo.png"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": "@next", "url": site.URL + "/logo.png"}},
	})
	StartFetcher(s, queue, blob.NewMemoryBucket())
	deadline := time.Now().Add(5 * time.Second)
	for {
		a, _ := s.Model().Table("DOCUMENT").Get(ids[2])
		b, _ := s.Model().Table("DOCUMENT").Get(ids[3])
		if a["content"] != "" && b["content"] != "" {
			if a["content"] != b["content"] {
				t.Fatalf("the two documents hold different content: %v %v", a, b)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the documents were not filled: %v %v", a, b)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := asked.Load(); n != 1 {
		t.Fatalf("the address was fetched %d times", n)
	}
}

func TestALinkedPageWithNoTextIsRefused(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><div>Could not load network resources</div><button>Retry</button></body></html>`))
	}))
	defer site.Close()
	s, queue := sampleWithQueue(t)
	linked := apply(t, s, queue, []Edit{
		{Insert: "DOCUMENT", As: "mail", Row: map[string]any{"kind": "mail", "name": "Fund a need"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@mail", "url": site.URL + "/dl/b33482"}},
	})[1]
	StartFetcher(s, queue, blob.NewMemoryBucket())
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		row, _ := s.Model().Table("DOCUMENT").Get(linked)
		if row["fetch"] == "refused" && row["content"] == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the shell page was not refused: %v", row)
		}
	}
}

func TestALinkedPageSayingItsSignUpIsGoneIsRefused(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><nav>Create a Sign Up Find a Sign Up Plans Pricing Resources Help Sign In Register</nav><h1>The Sign Up Was Not Found</h1><p>The web address you entered is not a valid sign up. It may have been wrapped by your email program, or the sign up may have been deleted by its creator.</p><footer>About Us Careers Press Contact Blog Terms Privacy</footer></body></html>`))
	}))
	defer site.Close()
	s, queue := sampleWithQueue(t)
	linked := apply(t, s, queue, []Edit{
		{Insert: "DOCUMENT", As: "mail", Row: map[string]any{"kind": "mail", "name": "Summer camps"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@mail", "url": site.URL + "/go/camps"}},
	})[1]
	StartFetcher(s, queue, blob.NewMemoryBucket())
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		row, _ := s.Model().Table("DOCUMENT").Get(linked)
		if row["fetch"] == "refused" && row["content"] == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the not-found page was not refused: %v", row)
		}
	}
}

func TestALinkLandingOnAGoogleEditorFetchesItsExport(t *testing.T) {
	deck := []byte("%PDF-1.4\n% the condors deck\n")
	intercept.Install("drive.google.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://docs.google.com/presentation/d/"+r.URL.Query().Get("id")+"/edit?usp=drive_open", http.StatusTemporaryRedirect)
	}))
	intercept.Install("docs.google.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/presentation/d/deck1/export" && r.URL.Query().Get("format") == "pdf" {
			w.Write(deck)
			return
		}
		w.Write([]byte(`<html><body>This browser version is no longer supported. Slideshow Share Sign in</body></html>`))
	}))
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	linked := apply(t, s, queue, []Edit{
		{Insert: "DOCUMENT", As: "mail", Row: map[string]any{"kind": "mail", "name": "Weekly Update"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@mail", "url": "https://drive.google.com/open?id=deck1"}},
	})[1]
	StartFetcher(s, queue, bucket)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		row, _ := s.Model().Table("DOCUMENT").Get(linked)
		if row["content"] != "" {
			if got := bytesOf(t, s, bucket, row); got != string(deck) {
				t.Fatalf("the link holds %q", got)
			}
			return
		}
		if row["fetch"] != "" || time.Now().After(deadline) {
			t.Fatalf("the link was not filled with the deck's export: %v", row)
		}
	}
}

func TestASignedInFetchFillsALinkedDocument(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, queue, bucket)
	ids := apply(t, s, queue, []Edit{
		{Insert: "CONTENT", As: "mail", Row: map[string]any{"hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}},
		{Insert: "CONTENT", As: "html", Row: map[string]any{"hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}},
		{Insert: "DOCUMENT", As: "clubs", Row: map[string]any{"kind": "mail", "content": "@mail", "name": "Clubs this week"}},
		{Insert: "DOCUMENT", As: "part", Row: map[string]any{"relation": "part", "parent": "@clubs", "content": "@html"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": "@part", "url": "https://lh6.example.org/schedule", "fetch": "sign_in"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": "@part", "url": "https://lh6.example.org/gone", "fetch": "sign_in"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": "@part", "url": "https://lh6.example.org/page", "fetch": "sign_in"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@part", "url": "https://docs.example.org/handbook", "fetch": "sign_in"}},
	})
	part, scheduleDoc, goneDoc, pageDoc, handbook := ids[3], ids[4], ids[5], ids[6], ids[7]
	post := func(as string, fields map[string]string, body []byte) (int, fetchedAnswer) {
		t.Helper()
		file := ""
		if body != nil {
			file = "body"
		}
		rec := postFile(t, s, queue, pics, as, "fetched", fields, file, body)
		var answer fetchedAnswer
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
				t.Fatal(err)
			}
		}
		return rec.Code, answer
	}
	schedule := pngOf(t, 5)
	if code, _ := post("rowan.ashdown@example.org", map[string]string{"document": scheduleDoc}, schedule); code != http.StatusForbidden {
		t.Fatalf("a person filled a linked document: %d", code)
	}
	code, answer := post(importing, map[string]string{"document": scheduleDoc}, schedule)
	if code != http.StatusOK || answer.Hash == "" {
		t.Fatalf("the import could not fill a linked document: %d %+v", code, answer)
	}
	filled, _ := s.Model().Table("DOCUMENT").Get(scheduleDoc)
	content, _ := s.Model().Table("CONTENT").Get(filled["content"])
	if filled["fetch"] != "" || content["mime"] != "image/png" || bytesOf(t, s, bucket, filled) != string(schedule) {
		t.Fatalf("the filled document: %v, its content %v", filled, content)
	}
	if code, _ := post(importing, map[string]string{"document": scheduleDoc}, schedule); code != http.StatusBadRequest {
		t.Fatalf("a filled document was filled again: %d", code)
	}
	if code, _ := post(importing, map[string]string{"document": goneDoc, "stop": "gone"}, nil); code != http.StatusBadRequest {
		t.Fatalf("a post with no body was taken: %d", code)
	}
	if code, answer := post(importing, map[string]string{"document": pageDoc}, []byte("<html><body>sign in</body></html>")); code != http.StatusOK || answer.Fetch != "refused" {
		t.Fatalf("a page was kept as an image: %d %+v", code, answer)
	}
	if code, answer := post(importing, map[string]string{"document": handbook}, []byte("<html><body>"+strings.Repeat("The handbook says what families need to know. ", 6)+"</body></html>")); code != http.StatusOK || answer.Hash == "" {
		t.Fatalf("a link's page was not kept: %d %+v", code, answer)
	}
	if page, _ := s.Model().Table("DOCUMENT").Get(handbook); page["content"] == "" || page["fetch"] != "" {
		t.Fatalf("the link's page: %v", page)
	}
	for id, want := range map[string]string{goneDoc: "sign_in", pageDoc: "refused"} {
		if row, _ := s.Model().Table("DOCUMENT").Get(id); row["fetch"] != want || row["content"] != "" {
			t.Errorf("%s: %v, want fetch %s", id, row, want)
		}
	}
	if code, _ := post(importing, map[string]string{"document": part}, schedule); code != http.StatusBadRequest {
		t.Fatalf("a part was filled as a linked document: %d", code)
	}
}

func TestSignedInFetchesOfTheSameBytesShareContent(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := NewPictures(s, queue, blob.NewMemoryBucket())
	inserts := []Edit{
		{Insert: "CONTENT", As: "mail", Row: map[string]any{"hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}},
		{Insert: "DOCUMENT", As: "clubs", Row: map[string]any{"kind": "mail", "content": "@mail", "name": "Clubs this week"}},
	}
	for _, name := range []string{"one", "two", "three", "four"} {
		inserts = append(inserts, Edit{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": "@clubs", "url": "https://lh6.example.org/" + name, "fetch": "sign_in"}})
	}
	ids := apply(t, s, queue, inserts)[2:]
	picture := pngOf(t, 5)
	codes := make([]int, len(ids))
	wg := sync.WaitGroup{}
	for i, id := range ids {
		wg.Go(func() {
			codes[i] = postFile(t, s, queue, pics, importing, "fetched", map[string]string{"document": id}, "body", picture).Code
		})
	}
	wg.Wait()
	contents := map[string]bool{}
	for i, id := range ids {
		row, _ := s.Model().Table("DOCUMENT").Get(id)
		if codes[i] != http.StatusOK || row["content"] == "" {
			t.Fatalf("%s: %d %v", id, codes[i], row)
		}
		contents[row["content"]] = true
	}
	if len(contents) != 1 {
		t.Fatalf("the same bytes were kept as %d contents", len(contents))
	}
}
