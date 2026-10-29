package lease

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"heliosian/internal/blob"
)

func TestASecondWriterWaitsUntilTheFirstReleases(t *testing.T) {
	ctx := context.Background()
	bucket := blob.NewMemoryBucket()
	first, err := Acquire(ctx, bucket)
	if err != nil {
		t.Fatal(err)
	}
	taken := make(chan *Lease)
	go func() {
		second, err := Acquire(ctx, bucket)
		if err != nil {
			t.Error(err)
		}
		taken <- second
	}()
	select {
	case <-taken:
		t.Fatal("a second writer took a lease that was still held")
	case <-time.After(3 * poll):
	}
	if err := first.Release(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case second := <-taken:
		if second.holder == first.holder {
			t.Fatal("the second writer reused the first holder")
		}
		if err := second.Release(ctx); err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * poll):
		t.Fatal("the second writer never took the released lease")
	}
}

func TestAnExpiredLeaseIsTakenOver(t *testing.T) {
	ctx := context.Background()
	bucket := blob.NewMemoryBucket()
	raw, err := json.Marshal(record{Holder: "gone", Until: time.Now().Add(-time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bucket.Put(ctx, object, "application/json", raw, 0); err != nil {
		t.Fatal(err)
	}
	done := make(chan *Lease)
	go func() {
		l, err := Acquire(ctx, bucket)
		if err != nil {
			t.Error(err)
		}
		done <- l
	}()
	select {
	case l := <-done:
		if err := l.Release(ctx); err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * poll):
		t.Fatal("an expired lease was not taken over")
	}
}

func TestWaitingStopsWithItsContext(t *testing.T) {
	bucket := blob.NewMemoryBucket()
	held, err := Acquire(context.Background(), bucket)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), poll/2)
	defer cancel()
	if _, err := Acquire(ctx, bucket); err == nil {
		t.Fatal("waiting for a held lease ignored its context")
	}
}
