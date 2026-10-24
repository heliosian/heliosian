package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"heliosian/internal/blob"
)

func TestStockHitIsReadyBeforeLaterBatchesFinish(t *testing.T) {
	s, queue := sampleWithQueue(t)
	st := NewStock(s, queue, blob.NewMemoryBucket())
	blocked := make(chan struct{})
	release := make(chan struct{})
	queue.OnSwap(func() {
		if s.Model().Table("STOCK").Len() > stockBatch {
			close(blocked)
			<-release
		}
	})
	hits := []StockHit{}
	for i := range stockBatch + 1 {
		hits = append(hits, StockHit{Source: "Pexels", SourceID: fmt.Sprint(i), PreviewURL: "https://example.org/thumb", URL: "https://example.org/full"})
	}
	ids := st.Remember(hits)
	defer func() {
		close(release)
		<-queue.Drain()
	}()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("later batch did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := st.wait(ctx, ids[0]); err != nil {
		t.Fatalf("first batch's hit waited for a later batch: %v", err)
	}
}
