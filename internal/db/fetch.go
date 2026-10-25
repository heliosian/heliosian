package db

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
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
	fetchActor    = "fetch"
	fetchTimeout  = 30 * time.Second
	exportTimeout = 5 * time.Minute
	fetchLimit    = 100 << 20
	fetchHosts    = 8
	fetchWaits    = 4
	fetchTries    = 3
	fetchWait     = time.Minute
	fetchLongest  = 15 * time.Minute
	pageLeast     = 40
	googleSignIn  = "accounts.google.com"
)

var fetchClient = &http.Client{}

var keptPages = []string{"application/pdf", "text/html"}

var fetchedRelations = []string{"image", "linked"}

var deadPages = []string{"the sign up was not found"}

var googleExports = []string{"document", "presentation", "spreadsheets"}

type Fetcher struct {
	s      *Store
	bucket *blob.Bucket
	mu     sync.Mutex
	tries  map[string]int
	in     *inbox
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

func StartFetcher(s *Store, bucket *blob.Bucket) {
	f := &Fetcher{s: s, bucket: bucket, tries: map[string]int{}, in: s.Model().services.inbox(fetchInbox)}
	go f.run()
	s.OnTick(f.in.wake.poke)
	f.in.wake.poke()
}

func (f *Fetcher) run() {
	for range f.in.wake {
		f.in.take()
		byHost := map[string][][]store.Row{}
		same := map[[2]string]int{}
		for _, doc := range f.pending() {
			u, err := url.Parse(doc["url"])
			if err != nil {
				slog.Error("fetch: address", "document", doc["id"], "error", err)
				continue
			}
			key := [2]string{doc["relation"], doc["url"]}
			if i, ok := same[key]; ok {
				byHost[u.Host][i] = append(byHost[u.Host][i], doc)
				continue
			}
			same[key] = len(byHost[u.Host])
			byHost[u.Host] = append(byHost[u.Host], []store.Row{doc})
		}
		slots := make(chan struct{}, fetchHosts)
		wg := sync.WaitGroup{}
		for _, groups := range byHost {
			wg.Go(func() {
				slots <- struct{}{}
				defer func() { <-slots }()
				for _, docs := range groups {
					f.fetch(docs)
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

func (f *Fetcher) fetch(docs []store.Row) {
	ctx := context.Background()
	start := time.Now()
	doc := docs[0]
	ids := []string{}
	for _, d := range docs {
		ids = append(ids, d["id"])
	}
	got, err := fetchURL(ctx, doc)
	for waits := 0; err != nil && waits < fetchWaits; waits++ {
		var b busy
		if !errors.As(err, &b) {
			break
		}
		slog.Info("fetch: waiting", "documents", ids, "url", ShortSource(doc["url"]), "for", b.wait)
		time.Sleep(b.wait)
		got, err = fetchURL(ctx, doc)
	}
	if err != nil {
		f.mu.Lock()
		tries := 0
		for _, id := range ids {
			f.tries[id]++
			tries = max(tries, f.tries[id])
		}
		f.mu.Unlock()
		slog.Warn("fetch: failed", "documents", ids, "url", ShortSource(doc["url"]), "tries", tries, "error", err)
		if tries < fetchTries {
			return
		}
		if err := f.stop(ctx, ids, "sign_in"); err != nil {
			slog.Error("fetch: write", "documents", ids, "error", err)
		}
		return
	}
	if got.stop != "" {
		slog.Info("fetch: stopped", "documents", ids, "url", ShortSource(doc["url"]), "fetch", got.stop, "why", got.why)
		if err := f.stop(ctx, ids, got.stop); err != nil {
			slog.Error("fetch: write", "documents", ids, "error", err)
		}
		return
	}
	if err := f.keep(ctx, ids, got); err != nil {
		slog.Error("fetch: write", "documents", ids, "error", err)
		return
	}
	slog.Info("fetch: done", "documents", ids, "url", ShortSource(doc["url"]), "mime", got.mime, "bytes", len(got.body), "took", time.Since(start).Round(time.Millisecond))
}

func fetchURL(ctx context.Context, doc store.Row) (fetched, error) {
	return fetchFrom(ctx, doc, ExportURL(doc["url"]), true)
}

func fetchFrom(ctx context.Context, doc store.Row, address string, follow bool) (fetched, error) {
	timeout := fetchTimeout
	if address != doc["url"] {
		timeout = exportTimeout
	}
	within, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(within, http.MethodGet, address, nil)
	if err != nil {
		return fetched{stop: "refused", why: err.Error()}, nil
	}
	resp, err := fetchClient.Do(req)
	if Unreachable(err) {
		return fetched{stop: "gone", why: err.Error()}, nil
	}
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
	case code >= 500 && code < 600:
		return fetched{}, fmt.Errorf("answered %s", resp.Status)
	case code != http.StatusOK:
		return fetched{stop: "gone", why: resp.Status}, nil
	}
	if final := resp.Request.URL.String(); follow && ExportURL(final) != final {
		return fetchFrom(ctx, doc, ExportURL(final), false)
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
	if baseType(mimeType) != "text/html" {
		return mimeType, nil
	}
	text, err := pageMarkdown(body)
	if err != nil {
		return "", err
	}
	if words := len(strings.Fields(text)); words < pageLeast {
		return "", fmt.Errorf("page text is %d words", words)
	}
	for _, dead := range deadPages {
		if strings.Contains(strings.ToLower(text), dead) {
			return "", fmt.Errorf("the page says %q", dead)
		}
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

func Unreachable(err error) bool {
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return dns.IsNotFound || !dns.IsTemporary && !dns.IsTimeout
	}
	var cert *tls.CertificateVerificationError
	return errors.As(err, &cert)
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

func (f *Fetcher) still(tx *store.Tx[Model], ids []string) []string {
	out := []string{}
	for _, id := range ids {
		if now, ok := f.s.In(tx).Table("DOCUMENT").Get(id); ok && now["content"] == "" && now["fetch"] == "" {
			out = append(out, id)
		}
	}
	return out
}

func (f *Fetcher) stop(ctx context.Context, ids []string, why string) error {
	_, err := f.s.Transact(ctx, access.System(fetchActor), func(tx *store.Tx[Model]) error {
		w := &pendingWrites{}
		for _, id := range f.still(tx, ids) {
			w.add(Edit{Set: id, Cells: map[string]any{"fetch": why}})
		}
		_, err := stageEdits(f.s, tx, Env{System: fetchActor}, w.edits, fetchActor, true)
		return err
	})
	return err
}

func (f *Fetcher) keep(ctx context.Context, ids []string, got fetched) error {
	kept, err := keepBytes(ctx, f.s.Model(), f.bucket, got.mime, got.body)
	if err != nil {
		return err
	}
	_, err = f.s.Transact(ctx, access.System(fetchActor), func(tx *store.Tx[Model]) error {
		open := f.still(tx, ids)
		if len(open) == 0 {
			return nil
		}
		w := &pendingWrites{}
		content, err := stageContent(f.s.In(tx), w, map[string]string{}, kept)
		if err != nil {
			return err
		}
		for _, id := range open {
			w.add(Edit{Set: id, Cells: map[string]any{"content": content}})
		}
		_, err = stageEdits(f.s, tx, Env{System: fetchActor}, w.edits, fetchActor, true)
		return err
	})
	return err
}
