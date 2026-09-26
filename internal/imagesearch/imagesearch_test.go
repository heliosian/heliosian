package imagesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"heliosian/internal/auth"
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
		s.serveThumb(w, httptest.NewRequest(http.MethodGet, "/api/team/images/thumb?"+query, nil))
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
		s.serveImport(w, httptest.NewRequest(http.MethodPost, "/api/team/images/import", strings.NewReader(body)), "activity-images")
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
	Search{Stock: NewStock(blob.NewMemory())}.serveSearch(w, httptest.NewRequest(http.MethodGet, "/api/team/images/search?q=soccer", nil))
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
			s.serveThumb(w, httptest.NewRequest(http.MethodGet, "/api/team/images/thumb?id="+results[i%len(results)].hit.ID, nil))
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

func upload(t *testing.T, mux *http.ServeMux, as, path string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("image", "picture.png")
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	form.Close()
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	auth.Fixed(as, mux).ServeHTTP(rec, req)
	return rec
}

func TestUploadThroughRegister(t *testing.T) {
	s := Search{Stock: NewStock(blob.NewMemory()), Limits: NewLimits()}
	mux := http.NewServeMux()
	s.Register(mux, "/api/team", "activity-images", Members)
	refused := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "admin access required", http.StatusForbidden)
		}
	}
	s.Register(mux, "/api/apps", "link-images", refused)

	rec := upload(t, mux, "robin.whitfield@heliosschool.org", "/api/team/image", picture(t))
	var answer struct {
		Name string `json:"name"`
	}
	json.Unmarshal(rec.Body.Bytes(), &answer)
	if rec.Code != http.StatusOK || !strings.HasPrefix(answer.Name, "activity-images/") || !strings.HasSuffix(answer.Name, ".png") {
		t.Fatalf("member upload: %d %s", rec.Code, rec.Body)
	}
	if ok, err := s.Stock.store.Has(answer.Name); err != nil || !ok {
		t.Errorf("uploaded image in the store: %v %v", ok, err)
	}
	if rec := upload(t, mux, "robin.whitfield@heliosschool.org", "/api/team/image", []byte("<svg></svg>")); rec.Code != http.StatusBadRequest {
		t.Errorf("non-image upload: %d, want 400", rec.Code)
	}
	for _, path := range []string{"/api/apps/image", "/api/apps/images/import"} {
		if rec := upload(t, mux, "robin.whitfield@heliosschool.org", path, picture(t)); rec.Code != http.StatusForbidden {
			t.Errorf("%s past a refusing gate: %d, want 403", path, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	auth.Fixed("robin.whitfield@heliosschool.org", mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/apps/images/search?q=soccer", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("search past a refusing gate: %d, want 403", rec.Code)
	}
}

func TestUploadsAreLimited(t *testing.T) {
	s := Search{Stock: NewStock(blob.NewMemory()), Limits: NewLimits()}
	mux := http.NewServeMux()
	s.Register(mux, "/api/team", "activity-images", Members)
	for i := range storesPerHour {
		if rec := upload(t, mux, "robin.whitfield@heliosschool.org", "/api/team/image", picture(t)); rec.Code != http.StatusOK {
			t.Fatalf("upload %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	rec := upload(t, mux, "robin.whitfield@heliosschool.org", "/api/team/image", picture(t))
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), storeRefusal) {
		t.Errorf("upload over the limit: %d %s", rec.Code, rec.Body)
	}
	if rec := upload(t, mux, "Robin.Whitfield@heliosschool.org", "/api/team/image", picture(t)); rec.Code != http.StatusTooManyRequests {
		t.Errorf("same person in capitals: %d, want 429", rec.Code)
	}
	if rec := upload(t, mux, "mina.park@heliosschool.org", "/api/team/image", picture(t)); rec.Code != http.StatusOK {
		t.Errorf("someone else: %d, want 200", rec.Code)
	}
	search := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		auth.Fixed("robin.whitfield@heliosschool.org", mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/team/images/search?q=soccer", nil))
		return rec
	}
	for i := range searchesPerHour {
		if rec := search(); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("search %d refused: %s", i, rec.Body)
		}
	}
	if rec := search(); rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), searchRefusal) {
		t.Errorf("search over the limit: %d %s", rec.Code, rec.Body)
	}
}

func TestSearchSkipsAFailedLibrary(t *testing.T) {
	ok := func(id string) func(context.Context, string) ([]result, error) {
		return func(context.Context, string) ([]result, error) {
			return []result{{hit: Hit{ID: id}}}, nil
		}
	}
	failed := func(context.Context, string) ([]result, error) {
		return nil, errors.New("Unsplash refused the search: Rate Limit Exceeded")
	}
	results, err := gather(context.Background(), "soccer", []func(context.Context, string) ([]result, error){failed, ok("p1"), ok("x1")})
	if err != nil || len(results) != 2 || results[0].hit.ID != "p1" || results[1].hit.ID != "x1" {
		t.Errorf("one library failed: %v %v", results, err)
	}
	if _, err := gather(context.Background(), "soccer", []func(context.Context, string) ([]result, error){failed, failed}); err == nil {
		t.Error("every library failed: no error")
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
