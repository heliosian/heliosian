package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"heliosian/internal/qclient"
)

func TestUploadsWaitForThumbnailsAndBothSearchStages(t *testing.T) {
	pages, polls, phase := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/queues" {
			polls++
			if phase > 0 && phase <= 3 {
				name := []string{"thumbnails", "search indexing", "search processing"}[phase-1]
				fmt.Fprintf(w, `{"queues":[{"name":"thumbnails"},{"name":"search indexing"},{"name":"search processing"},{"name":"pending writes"},{"name":%q,"pending":1}]}`, name)
				phase++
				return
			}
			phase = 0
			writeDrained(w)
			return
		}
		if finishUpload(w, r, t) {
			return
		}
		if r.URL.Path != "/api/do/pdf-page" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		if phase != 0 {
			t.Error("another upload arrived before processing drained")
		}
		pages++
		if pages == 1 {
			phase = 1
		}
		io.WriteString(w, `{}`)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "page.png")
	if err := os.WriteFile(path, []byte("page pixels"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := upload(&qclient.Client{Base: server.URL}, "doc00000000001", "cnt00000000001", []string{path, path}); err != nil {
		t.Fatal(err)
	}
	if pages != 2 || polls != 6 {
		t.Fatalf("pages=%d polls=%d", pages, polls)
	}
}

func TestMissingQueueStatusStopsBeforeAnyUpload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/queues" {
			t.Errorf("request before processing status was known: %s", r.URL.Path)
		}
		io.WriteString(w, `{"queues":[]}`)
	}))
	defer server.Close()
	if err := draw(&qclient.Client{Base: server.URL}); err == nil {
		t.Fatal("missing queue status was accepted")
	}
}

func TestRateLimitStopsWithoutRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "rate exceeded", http.StatusTooManyRequests)
	}))
	defer server.Close()
	if err := send(&qclient.Client{Base: server.URL}, "/api/do/pdf-page", "doc00000000001", "cnt00000000001", 1, 1, []byte("pixels")); err == nil {
		t.Fatal("rate limit was ignored")
	}
	if requests != 1 {
		t.Fatalf("sent %d requests", requests)
	}
}

func TestReuseWaitsAfterCopyingBothImages(t *testing.T) {
	written, polled := false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/queues" {
			if !written {
				t.Error("expected processing check after the batch")
			}
			polled = true
			writeDrained(w)
			return
		}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			if !bytes.Contains(body, []byte(`"thumbnail":"cnt00000000003"`)) || !bytes.Contains(body, []byte(`"thumbnailed":"ready"`)) {
				t.Errorf("copy lacks the server thumbnail: %s", body)
			}
			written = true
			io.WriteString(w, `{}`)
			return
		}
		json.NewEncoder(w).Encode(qclient.Answer{Resources: map[string]map[string]map[string]string{"DOCUMENT": {
			"doc00000000001": {"content": "cnt00000000001", "thumbnail": "cnt00000000003", "rendered": "ready", "page_count": "1"},
			"doc00000000002": {"content": "cnt00000000001"},
			"doc00000000101": {"parent": "doc00000000001", "relation": "page", "content": "cnt00000000002", "first_page": "1", "last_page": "1", "thumbnail": "cnt00000000003", "thumbnailed": "ready"},
		}}})
	}))
	defer server.Close()
	if err := reuse(&qclient.Client{Base: server.URL}, "doc00000000002", "cnt00000000001", "doc00000000001"); err != nil {
		t.Fatal(err)
	}
	if !written || !polled {
		t.Fatal("reuse did not wait after its write")
	}
}
