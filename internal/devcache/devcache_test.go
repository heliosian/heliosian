package devcache

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type bucket struct {
	mu      sync.Mutex
	objects map[string]string
	calls   map[string]int
}

func (b *bucket) RoundTrip(req *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls[req.Method+" "+req.URL.Path]++
	if req.URL.Host != host || req.Header.Get("Accept-Encoding") != "" {
		return response(req, http.StatusBadRequest, http.Header{}, nil), nil
	}
	if req.Method == http.MethodDelete {
		delete(b.objects, req.URL.Path)
		return response(req, http.StatusNoContent, http.Header{}, nil), nil
	}
	body, ok := b.objects[req.URL.Path]
	if !ok {
		return response(req, http.StatusNotFound, http.Header{}, nil), nil
	}
	return response(req, http.StatusOK, http.Header{"Content-Type": {"image/jpeg"}, "X-Goog-Generation": {"7"}}, []byte(body)), nil
}

func (b *bucket) count(key string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls[key]
}

func client(t *testing.T, upstream http.RoundTripper) (*http.Client, string) {
	caKey, ca := mintCA()
	dir := t.TempDir()
	p := &proxy{
		dir:      dir,
		upstream: upstream,
		tls:      &tls.Config{Certificates: []tls.Certificate{mintLeaf(caKey, ca)}, NextProtos: []string{"http/1.1"}},
	}
	server := httptest.NewServer(p)
	t.Cleanup(server.Close)
	through, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(through), TLSClientConfig: &tls.Config{RootCAs: roots}}}, dir
}

func fetch(t *testing.T, c *http.Client, method, target string) (int, string, http.Header) {
	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body), resp.Header
}

func TestProxyCachesObjectReads(t *testing.T) {
	path := "/storage/v1/b/media/o/photos/abc.jpg"
	b := &bucket{objects: map[string]string{path: "photo"}, calls: map[string]int{}}
	c, dir := client(t, b)
	media := "https://" + host + "/storage/v1/b/media/o/photos%2Fabc.jpg?alt=media&prettyPrint=false"

	for range 3 {
		code, body, header := fetch(t, c, http.MethodGet, media)
		if code != http.StatusOK || body != "photo" || header.Get("X-Goog-Generation") != "7" || header.Get("Content-Type") != "image/jpeg" {
			t.Fatalf("read: %d %q %v", code, body, header)
		}
	}
	if n := b.count("GET " + path); n != 1 {
		t.Fatalf("the bucket was read %d times, want once", n)
	}
	for _, name := range []string{"abc.jpg", "abc.jpg.meta"} {
		if _, err := os.Stat(filepath.Join(dir, "media", "photos", name)); err != nil {
			t.Fatalf("cache file: %v", err)
		}
	}

	metadata := "https://" + host + "/storage/v1/b/media/o/photos%2Fabc.jpg?alt=json&fields=name"
	fetch(t, c, http.MethodGet, metadata)
	fetch(t, c, http.MethodGet, metadata)
	if n := b.count("GET " + path); n != 3 {
		t.Fatalf("metadata reads reached the bucket %d times in all, want every one", n)
	}

	missing := "https://" + host + "/storage/v1/b/media/o/photos%2Fnone.jpg?alt=media"
	for range 2 {
		if code, _, _ := fetch(t, c, http.MethodGet, missing); code != http.StatusNotFound {
			t.Fatalf("missing: %d", code)
		}
	}
	if n := b.count("GET /storage/v1/b/media/o/photos/none.jpg"); n != 2 {
		t.Fatalf("a missing object was read %d times, want every time", n)
	}

	if code, _, _ := fetch(t, c, http.MethodDelete, "https://"+host+"/storage/v1/b/media/o/photos%2Fabc.jpg"); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _, _ := fetch(t, c, http.MethodGet, media); code != http.StatusNotFound {
		t.Fatalf("a deleted object still read back: %d", code)
	}
}

func TestProxyConcurrentReadsLeaveNoTempFiles(t *testing.T) {
	path := "/storage/v1/b/media/o/photos/abc.jpg"
	b := &bucket{objects: map[string]string{path: "photo"}, calls: map[string]int{}}
	c, dir := client(t, b)
	media := "https://" + host + "/storage/v1/b/media/o/photos%2Fabc.jpg?alt=media"
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, media, nil)
			if err != nil {
				t.Error(err)
				return
			}
			resp, err := c.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusOK || string(body) != "photo" {
				t.Errorf("read: %d %q %v", resp.StatusCode, body, err)
			}
		}()
	}
	wg.Wait()
	leftovers, err := filepath.Glob(filepath.Join(dir, "media", "photos", "*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v %v", leftovers, err)
	}
}

func TestCachePathStaysUnderTheDirectory(t *testing.T) {
	p := &proxy{dir: "cache"}
	for _, c := range []struct {
		path string
		ok   bool
	}{
		{"/storage/v1/b/media/o/photos/abc.jpg", true},
		{"/storage/v1/b/media/o/../../../etc/passwd", false},
		{"/storage/v1/b/media/o/", false},
		{"/upload/storage/v1/b/media/o", false},
	} {
		if _, ok := p.cachePath(c.path); ok != c.ok {
			t.Errorf("%s: %v, want %v", c.path, ok, c.ok)
		}
	}
}
