package imagesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/hosts"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

func picture(t *testing.T) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sampleStock(t *testing.T) (*db.Stock, *db.Store) {
	t.Helper()
	stock, s, _ := sampleStockQueue(t)
	return stock, s
}

func sampleStockQueue(t *testing.T) (*db.Stock, *db.Store, *store.Queue) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	s, err := db.NewStore(dir, dir, queue, db.NewSearchIndex(), hosts.Origin("heliosian.com"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { <-queue.Drain() })
	return db.NewStock(s, queue, blob.NewMemoryBucket()), s, queue
}

func waitPreview(t *testing.T, stock *db.Stock, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		row, _ := stock.Row(id)
		if row["preview"] != "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s has no persisted preview: %v", id, row)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestStockFetchedOnce(t *testing.T) {
	pic := picture(t)
	fetches := map[string]int{}
	var mu sync.Mutex
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fetches[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "image/png")
		w.Write(pic)
	}))
	defer provider.Close()
	stock, s := sampleStock(t)
	search := Search{Pexels: "key", UserAgent: "test", Stock: stock}
	ids := stock.Remember([]db.StockHit{{Source: "Pexels", SourceID: "42", PreviewURL: provider.URL + "/thumb", URL: provider.URL + "/full"}})
	id := ids[0]
	if again := stock.Remember([]db.StockHit{{Source: "Pexels", SourceID: "42", PreviewURL: provider.URL + "/thumb", URL: provider.URL + "/full"}}); again[0] != id {
		t.Fatalf("the same hit remembered again: %v, want %s", again, id)
	}

	thumb := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		search.serveThumb(w, httptest.NewRequest(http.MethodGet, "/api/team/images/thumb?"+query, nil))
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
	waitPreview(t, stock, id)
	if w := thumb("id=stk00000000000"); w.Code != http.StatusNotFound {
		t.Errorf("unknown id: %d, want 404", w.Code)
	}
	if w := thumb("id=../42"); w.Code != http.StatusNotFound {
		t.Errorf("malformed id: %d, want 404", w.Code)
	}
	if w := thumb("id=cnt00000030001"); w.Code != http.StatusNotFound {
		t.Errorf("another table's id: %d, want 404", w.Code)
	}

	imported := func(body string) (int, string) {
		w := httptest.NewRecorder()
		serve.JSON(search.importImage)(w, httptest.NewRequest(http.MethodPost, "/api/team/images/import", strings.NewReader(body)))
		var answer struct {
			Stock string `json:"stock"`
		}
		json.Unmarshal(w.Body.Bytes(), &answer)
		return w.Code, answer.Stock
	}
	for range 2 {
		if code, got := imported(`{"id":"` + id + `"}`); code != http.StatusOK || got != id {
			t.Fatalf("import: %d %q", code, got)
		}
	}
	if fetches["/full"] != 1 {
		t.Errorf("full image fetched %d times, want once", fetches["/full"])
	}
	row, _ := stock.Row(id)
	if content, ok := s.Model().Table("CONTENT").Get(row["picture"]); !ok || content["hash"] != blob.Hash(pic) {
		t.Errorf("the picture is not kept as content: %v", row)
	}
	if code, _ := imported(`{"id":"stk00000000000"}`); code != http.StatusNotFound {
		t.Errorf("import of an unknown id: %d, want 404", code)
	}
	if code, _ := imported(`{"url":"http://169.254.169.254/"}`); code != http.StatusNotFound {
		t.Errorf("import by address: %d, want 404", code)
	}

	w := httptest.NewRecorder()
	serve.JSON(Search{Stock: stock}.search)(w, httptest.NewRequest(http.MethodGet, "/api/team/images/search?q=soccer", nil))
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
	stock, _ := sampleStock(t)
	search := Search{Pixabay: "key", UserAgent: "test", Stock: stock}
	hits := []db.StockHit{}
	for i := range 30 {
		sourceID := letters(i)
		hits = append(hits, db.StockHit{Source: "Pixabay", SourceID: sourceID, PreviewURL: provider.URL + "/" + sourceID, URL: provider.URL + "/full/" + sourceID})
	}
	ids := stock.Remember(hits)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		search.prefetch(ids)
	}()
	codes := make([]int, len(ids)*3)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			search.serveThumb(w, httptest.NewRequest(http.MethodGet, "/api/team/images/thumb?id="+ids[i%len(ids)], nil))
			codes[i] = w.Code
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusOK {
			t.Errorf("request %d: %d, want 200", i, code)
		}
	}
	if got := fetches.Load(); got != int32(len(ids)) {
		t.Errorf("fetched %d thumbnails from the library, want %d", got, len(ids))
	}
	for _, id := range ids {
		waitPreview(t, stock, id)
	}
}

func letters(i int) string {
	return string(rune('a'+i%26)) + string(rune('a'+i/26))
}

func TestRegisterGatesEveryRoute(t *testing.T) {
	stock, _ := sampleStock(t)
	s := Search{Stock: stock, Limits: NewLimits()}
	mux := http.NewServeMux()
	refused := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "admin access required", http.StatusForbidden)
		}
	}
	s.Register(mux, "/api/apps", refused)
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/apps/images/search?q=soccer", nil),
		httptest.NewRequest(http.MethodGet, "/api/apps/images/thumb?id=stk00000000000", nil),
		httptest.NewRequest(http.MethodPost, "/api/apps/images/import", strings.NewReader(`{"id":"stk00000000000"}`)),
	} {
		rec := httptest.NewRecorder()
		auth.Fixed("robin.whitfield@heliosschool.org", mux).ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s past a refusing gate: %d, want 403", req.URL.Path, rec.Code)
		}
	}
}

func TestImportsAndSearchesAreLimited(t *testing.T) {
	stock, _ := sampleStock(t)
	s := Search{Stock: stock, Limits: NewLimits()}
	mux := http.NewServeMux()
	s.Register(mux, "/api/team", Members)
	request := func(as, method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		auth.Fixed(as, mux).ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	importing := func(as string) *httptest.ResponseRecorder {
		return request(as, http.MethodPost, "/api/team/images/import", `{"id":"stk00000000000"}`)
	}
	for i := range importsPerHour {
		if rec := importing("robin.whitfield@heliosschool.org"); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("import %d refused: %s", i, rec.Body)
		}
	}
	if rec := importing("robin.whitfield@heliosschool.org"); rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), importRefusal) {
		t.Errorf("import over the limit: %d %s", rec.Code, rec.Body)
	}
	if rec := importing("Robin.Whitfield@heliosschool.org"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("same person in capitals: %d, want 429", rec.Code)
	}
	if rec := importing("mina.park@heliosschool.org"); rec.Code == http.StatusTooManyRequests {
		t.Errorf("someone else: %d", rec.Code)
	}
	for i := range searchesPerHour {
		if rec := request("robin.whitfield@heliosschool.org", http.MethodGet, "/api/team/images/search?q=soccer", ""); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("search %d refused: %s", i, rec.Body)
		}
	}
	if rec := request("robin.whitfield@heliosschool.org", http.MethodGet, "/api/team/images/search?q=soccer", ""); rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), searchRefusal) {
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
