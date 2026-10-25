package imagesearch

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/db"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

func TestThumbnailsAndImageDownloadsDoNotWaitForStockWrites(t *testing.T) {
	pic := picture(t)
	downloaded := make(chan struct{}, 1)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/full" {
			downloaded <- struct{}{}
		}
		w.Write(pic)
	}))
	defer provider.Close()
	stock, s := sampleStockQueue(t)
	search := Search{Pexels: "key", Stock: stock}
	blocked := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	go func() {
		_, err := s.Transact(context.Background(), access.System("imagesearch"), func(tx *store.Tx[db.Model]) error {
			close(blocked)
			<-release
			return nil
		})
		if err != nil {
			t.Error(err)
		}
	}()
	<-blocked
	id := stock.Remember([]db.StockHit{{Source: "Pexels", SourceID: "42", PreviewURL: provider.URL + "/thumb", URL: provider.URL + "/full"}})[0]
	thumbs := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		search.serveThumb(w, httptest.NewRequest(http.MethodGet, "/images/thumb?id="+id, nil))
		thumbs <- w
	}()
	select {
	case w := <-thumbs:
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), pic) {
			t.Fatalf("thumbnail: %d %s", w.Code, w.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("thumbnail waited for the blocked stock write")
	}
	imports := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		serve.JSON(search.importImage)(w, httptest.NewRequest(http.MethodPost, "/images/import", strings.NewReader("{\"id\":\""+id+"\"}")))
		imports <- w
	}()
	select {
	case <-downloaded:
	case <-time.After(time.Second):
		t.Fatal("image download waited for the blocked stock write")
	}
	if _, ok := stock.Row(id); ok {
		t.Fatal("stock row exists while commits are blocked")
	}
	unblock()
	select {
	case w := <-imports:
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), id) {
			t.Fatalf("import: %d %s", w.Code, w.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("import did not finish after releasing the stock write")
	}
}
