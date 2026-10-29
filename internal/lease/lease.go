package lease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/logging"
)

const (
	object = "lease/writer"
	term   = time.Minute
	renew  = 20 * time.Second
	poll   = 2 * time.Second
)

type record struct {
	Holder string    `json:"holder"`
	Until  time.Time `json:"until"`
}

type Lease struct {
	bucket     *blob.Bucket
	holder     string
	generation int64
	stop       chan struct{}
	stopped    chan struct{}
}

func Acquire(ctx context.Context, bucket *blob.Bucket) (*Lease, error) {
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	holder := hex.EncodeToString(id)
	waiting := ""
	for {
		raw, generation, err := bucket.GetVersion(ctx, object)
		if errors.Is(err, blob.ErrNotFound) {
			generation, err = 0, nil
		}
		if err != nil {
			slog.Warn("read the writer lease", "error", err)
			if err := sleep(ctx, poll); err != nil {
				return nil, err
			}
			continue
		}
		if generation != 0 {
			var held record
			if err := json.Unmarshal(raw, &held); err != nil {
				return nil, fmt.Errorf("the writer lease holds %q: %w", raw, err)
			}
			if time.Now().Before(held.Until) {
				if held.Holder != waiting {
					slog.Info("waiting for the writer lease", "holder", held.Holder, "until", held.Until)
					waiting = held.Holder
				}
				if err := sleep(ctx, poll); err != nil {
					return nil, err
				}
				continue
			}
		}
		taken, err := write(ctx, bucket, holder, generation)
		if errors.Is(err, blob.ErrChanged) {
			continue
		}
		if err != nil {
			slog.Warn("take the writer lease", "error", err)
			if err := sleep(ctx, poll); err != nil {
				return nil, err
			}
			continue
		}
		l := &Lease{bucket: bucket, holder: holder, generation: taken, stop: make(chan struct{}), stopped: make(chan struct{})}
		go l.keep()
		slog.Info("holding the writer lease", "holder", holder)
		return l, nil
	}
}

func (l *Lease) Release(ctx context.Context) error {
	close(l.stop)
	<-l.stopped
	if err := l.bucket.Remove(ctx, object, l.generation); err != nil {
		return err
	}
	slog.Info("released the writer lease", "holder", l.holder)
	return nil
}

func (l *Lease) keep() {
	defer close(l.stopped)
	renewed := time.Now()
	ticker := time.NewTicker(renew)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
		}
		taken, err := write(context.Background(), l.bucket, l.holder, l.generation)
		if errors.Is(err, blob.ErrChanged) {
			logging.Fatal("lost the writer lease", "holder", l.holder, "error", err)
		}
		if err != nil {
			slog.Warn("renew the writer lease", "holder", l.holder, "error", err)
			if time.Since(renewed) >= term-renew {
				logging.Fatal("could not renew the writer lease before it ran out", "holder", l.holder, "error", err)
			}
			continue
		}
		l.generation, renewed = taken, time.Now()
	}
}

func write(ctx context.Context, bucket *blob.Bucket, holder string, generation int64) (int64, error) {
	raw, err := json.Marshal(record{Holder: holder, Until: time.Now().Add(term)})
	if err != nil {
		return 0, err
	}
	return bucket.Put(ctx, object, "application/json", raw, generation)
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
