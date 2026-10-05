package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

const (
	fetchActor   = "fetch"
	fetchTimeout = 30 * time.Second
	fetchLimit   = 25 << 20
	fetchHosts   = 8
	fetchWaits   = 4
	fetchTries   = 3
	fetchWait    = time.Minute
	fetchLongest = 15 * time.Minute
	googleSignIn = "accounts.google.com"
)

var fetchClient = &http.Client{Timeout: fetchTimeout}

var keptPages = []string{"application/pdf", "text/html"}

var fetchedRelations = []string{"image", "linked"}

var googleExports = []string{"document", "presentation", "spreadsheets"}

type Fetcher struct {
	s      *Store
	queue  *store.Queue
	bucket *blob.Bucket
	mu     sync.Mutex
	tries  map[string]int
	poke   chan struct{}
}

type fetched struct {
	body      []byte
	mime      string
	stop, why string
}

type busy struct {
	wait time.Duration
}

func (b busy) Error() string {
	return fmt.Sprintf("answered 429, asking for a wait of %s", b.wait)
}

func StartFetcher(s *Store, queue *store.Queue, bucket *blob.Bucket) {
	f := &Fetcher{s: s, queue: queue, bucket: bucket, tries: map[string]int{}, poke: make(chan struct{}, 1)}
	go f.run()
	queue.OnSwap(func() {
		select {
		case f.poke <- struct{}{}:
		default:
		}
	})
}

func (f *Fetcher) run() {
	for range f.poke {
		byHost := map[string][]store.Row{}
		for _, doc := range f.pending() {
			u, err := url.Parse(doc["url"])
			if err != nil {
				slog.Error("fetch: address", "document", doc["id"], "error", err)
				continue
			}
			byHost[u.Host] = append(byHost[u.Host], doc)
		}
		slots := make(chan struct{}, fetchHosts)
		wg := sync.WaitGroup{}
		for _, docs := range byHost {
			wg.Go(func() {
				slots <- struct{}{}
				defer func() { <-slots }()
				for _, doc := range docs {
					f.fetch(doc)
				}
			})
		}
		wg.Wait()
	}
}

func (f *Fetcher) pending() []store.Row {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []store.Row{}
	for _, row := range f.s.Model().Table("DOCUMENT").All() {
		if slices.Contains(fetchedRelations, row["relation"]) && row["url"] != "" && row["content"] == "" && row["fetch"] == "" && f.tries[row["id"]] < fetchTries {
			out = append(out, row)
		}
	}
	return out
}

func (f *Fetcher) fetch(doc store.Row) {
	ctx := context.Background()
	start := time.Now()
	got, err := fetchURL(ctx, doc)
	for waits := 0; err != nil && waits < fetchWaits; waits++ {
		var b busy
		if !errors.As(err, &b) {
			break
		}
		slog.Info("fetch: waiting", "document", doc["id"], "url", ShortSource(doc["url"]), "for", b.wait)
		time.Sleep(b.wait)
		got, err = fetchURL(ctx, doc)
	}
	if err != nil {
		f.mu.Lock()
		f.tries[doc["id"]]++
		f.mu.Unlock()
		slog.Warn("fetch: failed", "document", doc["id"], "url", ShortSource(doc["url"]), "error", err)
		return
	}
	if got.stop != "" {
		slog.Info("fetch: stopped", "document", doc["id"], "url", ShortSource(doc["url"]), "fetch", got.stop, "why", got.why)
		if err := f.stop(ctx, doc["id"], got.stop); err != nil {
			slog.Error("fetch: write", "document", doc["id"], "error", err)
		}
		return
	}
	if err := f.keep(ctx, doc["id"], got); err != nil {
		slog.Error("fetch: write", "document", doc["id"], "error", err)
		return
	}
	slog.Info("fetch: done", "document", doc["id"], "url", ShortSource(doc["url"]), "mime", got.mime, "bytes", len(got.body), "took", time.Since(start).Round(time.Millisecond))
}

func fetchURL(ctx context.Context, doc store.Row) (fetched, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ExportURL(doc["url"]), nil)
	if err != nil {
		return fetched{stop: "refused", why: err.Error()}, nil
	}
	resp, err := fetchClient.Do(req)
	if err != nil {
		return fetched{}, err
	}
	defer resp.Body.Close()
	code := resp.StatusCode
	switch {
	case resp.Request.URL.Host == googleSignIn:
		return fetched{stop: "sign_in", why: "sent to google sign-in"}, nil
	case code == http.StatusTooManyRequests:
		return fetched{}, busy{wait: retryAfter(resp.Header.Get("Retry-After"))}
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return fetched{stop: "sign_in", why: resp.Status}, nil
	case code == http.StatusRequestTimeout:
		return fetched{}, fmt.Errorf("answered %s", resp.Status)
	case code >= 400 && code < 500:
		return fetched{stop: "gone", why: resp.Status}, nil
	case code != http.StatusOK:
		return fetched{}, fmt.Errorf("answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchLimit+1))
	if err != nil {
		return fetched{}, err
	}
	if len(body) > fetchLimit {
		return fetched{stop: "refused", why: fmt.Sprintf("larger than %d bytes", fetchLimit)}, nil
	}
	mimeType, err := keptBody(doc["relation"], body)
	if err != nil {
		return fetched{stop: "refused", why: err.Error()}, nil
	}
	return fetched{body: body, mime: mimeType}, nil
}

func keptBody(relation string, body []byte) (string, error) {
	mimeType := http.DetectContentType(body)
	if relation == "image" || strings.HasPrefix(mimeType, "image/") {
		return keptImage(body)
	}
	if !slices.Contains(keptPages, baseType(mimeType)) {
		return "", fmt.Errorf("%s is not an image, a pdf or a page", mimeType)
	}
	return mimeType, nil
}

func ExportURL(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return address
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	at := slices.Index(parts, "d")
	if at < 1 || at+1 >= len(parts) {
		return address
	}
	switch {
	case u.Host == "docs.google.com" && slices.Contains(googleExports, parts[0]):
		return "https://docs.google.com/" + parts[0] + "/d/" + parts[at+1] + "/export?format=pdf"
	case u.Host == "drive.google.com" && parts[0] == "file":
		return "https://drive.google.com/uc?id=" + parts[at+1] + "&export=download"
	}
	return address
}

func retryAfter(header string) time.Duration {
	wait := fetchWait
	if seconds, err := strconv.Atoi(header); err == nil {
		wait = time.Duration(seconds) * time.Second
	} else if at, err := http.ParseTime(header); err == nil {
		wait = time.Until(at)
	}
	return min(max(wait, time.Second), fetchLongest)
}

func (f *Fetcher) still(tx *store.Tx, id string) bool {
	now, ok := f.s.In(tx).Table("DOCUMENT").Get(id)
	return ok && now["content"] == "" && now["fetch"] == ""
}

func (f *Fetcher) stop(ctx context.Context, id, why string) error {
	_, err := f.queue.Transact(ctx, access.System(fetchActor), func(tx *store.Tx) error {
		if !f.still(tx, id) {
			return nil
		}
		_, err := stager(f.s, tx, fetchActor)(Edit{Set: id, Cells: map[string]any{"fetch": why}})
		return err
	})
	return err
}

func (f *Fetcher) keep(ctx context.Context, id string, got fetched) error {
	sum := sha256.Sum256(got.body)
	hash := hex.EncodeToString(sum[:])
	name := contentFolder + "/" + hash
	stored := false
	if _, held := f.s.Model().Table("CONTENT").Find(hash, got.mime); !held {
		if err := f.bucket.Put(ctx, name, got.mime, got.body); err != nil {
			return err
		}
		stored = true
	}
	committed := false
	_, err := f.queue.Transact(ctx, access.System(fetchActor), func(tx *store.Tx) error {
		if !f.still(tx, id) {
			return nil
		}
		stage := stager(f.s, tx, fetchActor)
		content, err := stageContent(f.s, tx, stage, map[string]string{}, hash, got.mime, len(got.body))
		if err != nil {
			return err
		}
		_, err = stage(Edit{Set: id, Cells: map[string]any{"content": content}})
		committed = err == nil
		return err
	})
	if stored && (err != nil || !committed) {
		dropUnheld(f.s, f.bucket, []string{name})
	}
	return err
}
