package db

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"heliosian/internal/blob"
)

func TestOverlappingSearchesRememberEachHitOnce(t *testing.T) {
	s, queue := sampleWithQueue(t)
	st := NewStock(s, queue, blob.NewMemoryBucket())
	hit := func(id string) StockHit {
		return StockHit{Source: "Pexels", SourceID: id, PreviewURL: "https://example.org/" + id + "/small", URL: "https://example.org/" + id}
	}
	searches := [][]StockHit{}
	for n := range 8 {
		search := []StockHit{hit("shared"), hit(fmt.Sprint(n)), hit("shared")}
		for i := range stockBatch {
			search = append(search, hit(fmt.Sprint("many", i)))
		}
		searches = append(searches, search)
	}
	answers := make([][]string, len(searches))
	errs := make([]error, len(searches))
	var wg sync.WaitGroup
	for i, search := range searches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			answers[i] = st.Remember(search)
			for _, id := range answers[i] {
				if err := st.wait(context.Background(), id); err != nil {
					errs[i] = err
				}
			}
		}()
	}
	wg.Wait()
	shared := ""
	for i, ids := range answers {
		if errs[i] != nil {
			t.Fatalf("search %d: %v", i, errs[i])
		}
		if ids[0] == "" || ids[0] != ids[2] {
			t.Fatalf("search %d answered %v for the hit it holds twice", i, ids)
		}
		if shared == "" {
			shared = ids[0]
		}
		if ids[0] != shared {
			t.Fatalf("search %d remembered the shared hit as %s, another as %s", i, ids[0], shared)
		}
	}
	if n := s.Model().Table("STOCK").Len(); n != 1+len(searches)+stockBatch {
		t.Fatalf("%d STOCK rows, want one for each distinct hit", n)
	}
}
