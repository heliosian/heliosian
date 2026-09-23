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
	"testing"

	"heliosian/internal/blob"
)

func TestStockFetchedOnce(t *testing.T) {
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewGray(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	fetches := map[string]int{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches[r.URL.Path]++
		w.Header().Set("Content-Type", "image/png")
		w.Write(picture.Bytes())
	}))
	defer provider.Close()
	s := Search{Pexels: "key", UserAgent: "test", Store: blob.NewMemory()}
	id := key("Pexels", "42")
	rec, err := json.Marshal(record{Source: "Pexels", SourceID: "42", Thumb: provider.URL + "/thumb", URL: provider.URL + "/full"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Write(context.Background(), recordName(id), "application/json", rec); err != nil {
		t.Fatal(err)
	}

	thumb := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.ServeThumb(w, httptest.NewRequest(http.MethodGet, "/api/team/images/thumb?"+query, nil))
		return w
	}
	for range 2 {
		w := thumb("id=" + id)
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), picture.Bytes()) {
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
	if ok, err := s.Store.Has(first); err != nil || !ok {
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
	Search{Store: blob.NewMemory()}.ServeSearch(w, httptest.NewRequest(http.MethodGet, "/api/team/images/search?q=soccer", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("search with no library: %d, want 400", w.Code)
	}
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
