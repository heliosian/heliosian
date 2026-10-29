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
	"sync/atomic"
	"testing"

	"heliosian/internal/qclient"
)

func TestPagesUploadBackToBackThenWaitForEveryPageThumbnail(t *testing.T) {
	pages, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/do/pdf-page":
			pages++
			io.WriteString(w, `{}`)
		case r.URL.Path == "/api/q" && r.Method == http.MethodPost:
			if pages != 3 {
				t.Errorf("thumbnail copied after %d pages", pages)
			}
			io.WriteString(w, `{}`)
		case r.URL.Path == "/api/q":
			reads++
			docs := map[string]map[string]string{"doc00000000001": {"content": "cnt00000000001", "page_count": "3"}}
			for n := 1; n <= 3; n++ {
				page := map[string]string{"parent": "doc00000000001", "first_page": fmt.Sprint(n)}
				if reads > n {
					page["thumbnail"], page["thumbnailed"] = fmt.Sprintf("cnt0000000001%d", n), "2026-10-28 20:00:00"
				}
				docs[fmt.Sprintf("doc0000000010%d", n)] = page
			}
			json.NewEncoder(w).Encode(qclient.Answer{Resources: map[string]map[string]map[string]string{"DOCUMENT": docs}})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "page.png")
	if err := os.WriteFile(path, []byte("page pixels"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := upload(&qclient.Client{Base: server.URL}, "doc00000000001", "cnt00000000001", []string{path, path, path}); err != nil {
		t.Fatal(err)
	}
	if pages != 3 || reads != 4 {
		t.Fatalf("pages=%d reads=%d", pages, reads)
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

func TestServerRestartsStartThePassOverAndRefusalsStop(t *testing.T) {
	var code atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if code.Load() != 0 {
			http.Error(w, "answer", int(code.Load()))
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n{\"result\":")
		conn.Close()
	}))
	for _, c := range []struct {
		code int
		over bool
	}{{0, true}, {http.StatusBadGateway, true}, {http.StatusInternalServerError, true}, {http.StatusTooManyRequests, false}, {http.StatusForbidden, false}} {
		code.Store(int64(c.code))
		err := draw(&qclient.Client{Base: server.URL})
		if err == nil || unavailable(err) != c.over {
			t.Errorf("%d: unavailable=%v, error %v", c.code, unavailable(err), err)
		}
	}
	server.Close()
	if err := draw(&qclient.Client{Base: server.URL}); err == nil || !unavailable(err) {
		t.Errorf("an unreachable server: %v", err)
	}
}

func TestReuseWaitsForTheSourcePageThumbnails(t *testing.T) {
	reads, written := 0, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/q" {
			t.Errorf("unexpected request %s", r.URL.Path)
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
		reads++
		second := map[string]string{"parent": "doc00000000001", "relation": "page", "content": "cnt00000000004", "first_page": "2", "last_page": "2"}
		if reads > 1 {
			second["thumbnail"], second["thumbnailed"] = "cnt00000000005", "ready"
		}
		json.NewEncoder(w).Encode(qclient.Answer{Resources: map[string]map[string]map[string]string{"DOCUMENT": {
			"doc00000000001": {"content": "cnt00000000001", "thumbnail": "cnt00000000003", "rendered": "ready", "page_count": "2"},
			"doc00000000002": {"content": "cnt00000000001"},
			"doc00000000101": {"parent": "doc00000000001", "relation": "page", "content": "cnt00000000002", "first_page": "1", "last_page": "1", "thumbnail": "cnt00000000003", "thumbnailed": "ready"},
			"doc00000000102": second,
		}}})
	}))
	defer server.Close()
	if err := reuse(&qclient.Client{Base: server.URL}, "doc00000000002", "cnt00000000001", "doc00000000001"); err != nil {
		t.Fatal(err)
	}
	if !written || reads != 2 {
		t.Fatalf("written=%v reads=%d", written, reads)
	}
}
