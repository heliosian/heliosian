package imagesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"heliosian/internal/blob"
)

func picture(t *testing.T) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestStockFetchedOnce(t *testing.T) {
	pic := picture(t)
	fetches := map[string]int{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches[r.URL.Path]++
		w.Header().Set("Content-Type", "image/png")
		w.Write(pic)
	}))
	defer provider.Close()
	s := Search{Pexels: "key", UserAgent: "test", Stock: NewStock(blob.NewMemory())}
	id := key("Pexels", "42")
	rec, err := json.Marshal(record{Source: "Pexels", SourceID: "42", Thumb: provider.URL + "/thumb", URL: provider.URL + "/full"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Stock.store.Write(context.Background(), recordName(id), "application/json", rec); err != nil {
		t.Fatal(err)
	}

	thumb := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.ServeThumb(w, httptest.NewRequest(http.MethodGet, "/api/team/images/thumb?"+query, nil))
		return w
	}
	for range 2 {
		w := thumb("id=" + id)
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), pic) {
			t.Fatalf("thumb: %d %q", w.Code, w.Header().Get("Content-Type"))
		}
		if w.Header().Get("Cache-Control") != "private, max-age=31536000, immutable" {
			t.Errorf("thumb cache-control: %q", w.Header().Get("Cache-Control"))
		}
	}
	if fetches["/thumb"] != 1 {
		t.Errorf("thumbnail fetched %d times, want once", fetches["/thumb"])
	}
	if w := thumb("id=" + key("Pexels", "43")); w.Code != http.StatusNotFound {
		t.Errorf("unknown id: %d, want 404", w.Code)
	}
	if w := thumb("id=../42"); w.Code != http.StatusNotFound {
		t.Errorf("malformed id: %d, want 404", w.Code)
	}
	if w := thumb("source=Pexels&id=42"); w.Code != http.StatusNotFound {
		t.Errorf("a library's own id: %d, want 404", w.Code)
	}

	imported := func(body string) (int, string) {
		w := httptest.NewRecorder()
		s.ServeImport(w, httptest.NewRequest(http.MethodPost, "/api/team/images/import", strings.NewReader(body)), "activity-images", 8<<20)
		var answer struct {
			Name string `json:"name"`
		}
		json.Unmarshal(w.Body.Bytes(), &answer)
		return w.Code, answer.Name
	}
	code, first := imported(`{"id":"` + id + `"}`)
	if code != http.StatusOK || !strings.HasPrefix(first, "activity-images/") || !strings.HasSuffix(first, ".png") {
		t.Fatalf("import: %d %q", code, first)
	}
	if code, again := imported(`{"id":"` + id + `"}`); code != http.StatusOK || again != first {
		t.Errorf("second import: %d %q, want %q", code, again, first)
	}
	if fetches["/full"] != 1 {
		t.Errorf("full image fetched %d times, want once", fetches["/full"])
	}
	if ok, err := s.Stock.store.Has(first); err != nil || !ok {
		t.Errorf("imported image in the store: %v %v", ok, err)
	}
	if code, _ := imported(`{"id":"` + key("Pexels", "43") + `"}`); code != http.StatusNotFound {
		t.Errorf("import of an unknown id: %d, want 404", code)
	}
	if code, _ := imported(`{"url":"http://169.254.169.254/"}`); code != http.StatusNotFound {
		t.Errorf("import by address: %d, want 404", code)
	}
	if code, _ := imported(`{"source":"Pexels","id":"42"}`); code != http.StatusNotFound {
		t.Errorf("import by a library's own id: %d, want 404", code)
	}

	w := httptest.NewRecorder()
	Search{Stock: NewStock(blob.NewMemory())}.ServeSearch(w, httptest.NewRequest(http.MethodGet, "/api/team/images/search?q=soccer", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("search with no library: %d, want 400", w.Code)
	}
}

func TestThumbnailFetchedOnceUnderLoad(t *testing.T) {
	pic := picture(t)
	var fetches atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		w.Write(pic)
	}))
	defer provider.Close()
	s := Search{Pixabay: "key", UserAgent: "test", Stock: NewStock(blob.NewMemory())}
	results := []result{}
	for i := range 30 {
		sourceID := letters(i)
		results = append(results, result{hit: Hit{ID: key("Pixabay", sourceID)}, rec: record{Source: "Pixabay", SourceID: sourceID, Thumb: provider.URL + "/" + sourceID}})
	}
	s.Stock.remember(results)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Stock.prefetch(results, "test")
	}()
	codes := make([]int, len(results)*3)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			s.ServeThumb(w, httptest.NewRequest(http.MethodGet, "/api/team/images/thumb?id="+results[i%len(results)].hit.ID, nil))
			codes[i] = w.Code
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusOK {
			t.Errorf("request %d: %d, want 200", i, code)
		}
	}
	if got := fetches.Load(); got != int32(len(results)) {
		t.Errorf("fetched %d thumbnails from the library, want %d", got, len(results))
	}
	for _, res := range results {
		if held, err := s.Stock.store.Exists(context.Background(), thumbName(res.hit.ID)); err != nil || !held {
			t.Errorf("thumbnail of %s in the bucket: %v %v", res.rec.SourceID, held, err)
		}
	}
}

func letters(i int) string {
	return string(rune('a'+i%26)) + string(rune('a'+i/26))
}

func TestInterleave(t *testing.T) {
	a := []result{{hit: Hit{ID: "a1"}}, {hit: Hit{ID: "a2"}}, {hit: Hit{ID: "a3"}}}
	b := []result{{hit: Hit{ID: "b1"}}}
	c := []result{{hit: Hit{ID: "c1"}}, {hit: Hit{ID: "c2"}}}
	got := []string{}
	for _, res := range interleave([][]result{a, b, c}) {
		got = append(got, res.hit.ID)
	}
	if strings.Join(got, " ") != "a1 b1 c1 a2 c2 a3" {
		t.Errorf("interleaved: %v", got)
	}
}
