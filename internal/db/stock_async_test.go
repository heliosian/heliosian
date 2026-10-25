package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

func TestRememberAnswersWhileAnotherTransactionIsBlocked(t *testing.T) {
	s := sample(t)
	st := NewStock(s, blob.NewMemoryBucket())
	blocked := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, err := s.Transact(context.Background(), access.System(stockActor), func(tx *store.Tx[Model]) error {
			close(blocked)
			<-release
			return nil
		})
		if err != nil {
			t.Error(err)
		}
	}()
	<-blocked
	hits := []StockHit{{Source: "Pexels", SourceID: "42", PreviewURL: "https://example.org/thumb", URL: "https://example.org/full"}}
	defer func() {
		close(release)
		<-finished
		if err := st.wait(context.Background(), st.Remember(hits)[0]); err != nil {
			t.Error(err)
		}
		if source, _, err := st.Keep(context.Background(), st.Remember(hits)[0], "source", nil); err != nil || source != "Pexels" {
			t.Errorf("using the pending hit after commit: %q, %v", source, err)
		}
	}()
	answered := make(chan []string, 1)
	go func() { answered <- st.Remember(hits) }()
	var ids []string
	select {
	case ids = <-answered:
	case <-time.After(time.Second):
		t.Fatal("remember waited for the blocked transaction")
	}
	if again := st.Remember(hits); again[0] != ids[0] {
		t.Fatalf("overlapping search returned %v, want %v", again, ids)
	}
	if _, ok := st.Row(ids[0]); ok {
		t.Fatal("stock row was committed while the transaction was blocked")
	}
	fetches := 0
	for range 2 {
		data, mime, err := st.Preview(context.Background(), ids[0], func(row store.Row) ([]byte, string, error) {
			fetches++
			if row["preview_url"] != hits[0].PreviewURL {
				t.Errorf("preview address: %v", row)
			}
			return []byte("preview"), "image/png", nil
		})
		if err != nil || string(data) != "preview" || mime != "image/png" {
			t.Fatalf("preview while transaction blocked: %q %q %v", data, mime, err)
		}
	}
	if fetches != 1 {
		t.Fatalf("fetched preview %d times", fetches)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := st.Keep(ctx, ids[0], "source", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting for stock row: %v, want cancellation", err)
	}
}
