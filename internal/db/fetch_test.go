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
	"sync/atomic"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/store"
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
	if err := commit(s, DocumentsSheet,
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "newsletter", "name": "Clubs this week"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "kind": "newsletter", "name": "Clubs next week"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000012", "relation": "image", "parent": "doc00000000010", "url": site.URL + "/logo.png"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000013", "relation": "image", "parent": "doc00000000011", "url": site.URL + "/logo.png"}),
	); err != nil {
		t.Fatal(err)
	}
	StartFetcher(s, queue, blob.NewMemoryBucket())
	deadline := time.Now().Add(5 * time.Second)
	for {
		a, _ := s.Model().Table("DOCUMENT").Get("doc00000000012")
		b, _ := s.Model().Table("DOCUMENT").Get("doc00000000013")
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

func TestASignedInFetchFillsALinkedDocument(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, queue, bucket)
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "newsletter", "content": "cnt00000000001", "name": "Clubs this week"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "part", "parent": "doc00000000010", "content": "cnt00000000002"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000012", "relation": "image", "parent": "doc00000000011", "url": "https://lh6.example.org/schedule", "fetch": "sign_in"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000013", "relation": "image", "parent": "doc00000000011", "url": "https://lh6.example.org/gone", "fetch": "sign_in"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000014", "relation": "image", "parent": "doc00000000011", "url": "https://lh6.example.org/page", "fetch": "sign_in"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000015", "relation": "linked", "parent": "doc00000000011", "url": "https://docs.example.org/handbook", "fetch": "sign_in"}),
	); err != nil {
		t.Fatal(err)
	}
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
	if code, _ := post("maya.lindqvist@example.org", map[string]string{"document": "doc00000000012"}, schedule); code != http.StatusForbidden {
		t.Fatalf("a person filled a linked document: %d", code)
	}
	code, answer := post("bearer:"+testImportKey, map[string]string{"document": "doc00000000012"}, schedule)
	if code != http.StatusOK || answer.Hash == "" {
		t.Fatalf("the import could not fill a linked document: %d %+v", code, answer)
	}
	filled, _ := s.Model().Table("DOCUMENT").Get("doc00000000012")
	content, _ := s.Model().Table("CONTENT").Get(filled["content"])
	if filled["fetch"] != "" || content["mime"] != "image/png" || bytesOf(t, s, bucket, filled) != string(schedule) {
		t.Fatalf("the filled document: %v, its content %v", filled, content)
	}
	if code, _ := post("bearer:"+testImportKey, map[string]string{"document": "doc00000000012"}, schedule); code != http.StatusBadRequest {
		t.Fatalf("a filled document was filled again: %d", code)
	}
	if code, answer := post("bearer:"+testImportKey, map[string]string{"document": "doc00000000013", "stop": "gone"}, nil); code != http.StatusOK || answer.Fetch != "gone" {
		t.Fatalf("the import could not stop a fetch: %d %+v", code, answer)
	}
	if code, answer := post("bearer:"+testImportKey, map[string]string{"document": "doc00000000014"}, []byte("<html><body>sign in</body></html>")); code != http.StatusOK || answer.Fetch != "refused" {
		t.Fatalf("a page was kept as an image: %d %+v", code, answer)
	}
	if code, answer := post("bearer:"+testImportKey, map[string]string{"document": "doc00000000015"}, []byte("<html><body>the handbook</body></html>")); code != http.StatusOK || answer.Hash == "" {
		t.Fatalf("a link's page was not kept: %d %+v", code, answer)
	}
	if page, _ := s.Model().Table("DOCUMENT").Get("doc00000000015"); page["content"] == "" || page["fetch"] != "" {
		t.Fatalf("the link's page: %v", page)
	}
	for id, want := range map[string]string{"doc00000000013": "gone", "doc00000000014": "refused"} {
		if row, _ := s.Model().Table("DOCUMENT").Get(id); row["fetch"] != want || row["content"] != "" {
			t.Errorf("%s: %v, want fetch %s", id, row, want)
		}
	}
	if code, _ := post("bearer:"+testImportKey, map[string]string{"document": "doc00000000011"}, schedule); code != http.StatusBadRequest {
		t.Fatalf("a part was filled as a linked document: %d", code)
	}
}
