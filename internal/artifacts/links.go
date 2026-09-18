package artifacts

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	veracrossHost = ".veracross.com"
	benchmarkHost = "bmetrack.com"
	trackingPath  = "/c/"
	// benchmarkLink is the one Benchmark redirect that goes anywhere a
	// reader would want: the others open the mail in a browser or
	// unsubscribe whoever follows them, and neither may ever be offered.
	benchmarkLink = "/c/l"
	maxHops       = 4
	workers       = 12
)

// benchmarkVanity is the mailer's redirect under the school's own name,
// r560896.heliosschool.org, which looks like the school and is not.
var benchmarkVanity = regexp.MustCompile(`^r[0-9]+\.`)

// Every link the school's two mailers put in a newsletter is a redirect
// through the mailer, and carries the address of the person it was sent to.
// None of them may be handed to a reader, so each is turned back into the
// address it points at: Veracross writes that address into the link itself,
// where it can be read with no request at all, and Benchmark keeps it, so
// its links have to be followed. A link that cannot be turned back keeps its
// words and loses its address.
type Resolver struct {
	client  *http.Client
	mu      sync.Mutex
	known   map[string]string
	Dropped int
}

func NewResolver() *Resolver {
	return &Resolver{
		client: &http.Client{
			Timeout:       20 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		known: map[string]string{},
	}
}

// tracking says whether a link is one of the mailers' redirects rather than
// an address in its own right. Anything carrying an email parameter is one
// however it is dressed: that parameter is the reader, and the whole reason
// none of these may be written into a document.
func tracking(address string) bool {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if u.Query().Has("email") {
		return true
	}
	if !strings.HasPrefix(u.Path, trackingPath) {
		return false
	}
	return strings.HasSuffix(u.Host, veracrossHost) || strings.HasSuffix(u.Host, benchmarkHost) || benchmarkVanity.MatchString(u.Host)
}

// dead says whether a redirect is one that leads nowhere worth following:
// Benchmark's own view-in-browser and unsubscribe addresses.
func dead(u *url.URL) bool {
	if strings.HasSuffix(u.Host, veracrossHost) {
		return false
	}
	return strings.HasPrefix(u.Path, trackingPath) && u.Path != benchmarkLink
}

// Warm turns back every tracking link in a page at once, so a newsletter of
// forty of them takes one round trip's time rather than forty.
func (r *Resolver) Warm(addresses []string) {
	pending := []string{}
	seen := map[string]bool{}
	r.mu.Lock()
	for _, address := range addresses {
		if _, done := r.known[address]; done || seen[address] || !tracking(address) {
			continue
		}
		seen[address] = true
		pending = append(pending, address)
	}
	r.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	var wg sync.WaitGroup
	slots := make(chan struct{}, workers)
	for _, address := range pending {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			r.Resolve(address)
		}()
	}
	wg.Wait()
}

// Resolve is the address a link really points at: the link itself when it is
// nobody's redirect, the address inside it when the mailer wrote one there,
// and what following it answers when it did not. A tracking link that cannot
// be turned back resolves to nothing, and the page keeps its words alone.
func (r *Resolver) Resolve(address string) string {
	if !tracking(address) {
		return address
	}
	r.mu.Lock()
	target, done := r.known[address]
	r.mu.Unlock()
	if done {
		return target
	}
	target = unwrap(address)
	if target == "" {
		if u, err := url.Parse(address); err != nil || dead(u) {
			target = ""
		} else {
			target = r.follow(address)
		}
	}
	if target == "" {
		r.Dropped++
	}
	r.mu.Lock()
	r.known[address] = target
	r.mu.Unlock()
	return target
}

// unwrap is the address a Veracross link carries inside it: its path is a
// deflated query, and the query's l is where the link goes.
func unwrap(address string) string {
	u, err := url.Parse(address)
	if err != nil || !strings.HasSuffix(u.Host, veracrossHost) {
		return ""
	}
	encoded := strings.TrimPrefix(u.Path, trackingPath)
	padded := strings.ReplaceAll(strings.ReplaceAll(encoded, "-", "+"), "_", "/")
	padded += strings.Repeat("=", (4-len(padded)%4)%4)
	raw, err := base64.StdEncoding.DecodeString(padded)
	if err != nil {
		return ""
	}
	reader, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	defer reader.Close()
	inflated, err := io.ReadAll(io.LimitReader(reader, 1<<16))
	if err != nil && len(inflated) == 0 {
		return ""
	}
	query, err := url.ParseQuery(string(inflated))
	if err != nil {
		return ""
	}
	return clean(query.Get("l"))
}

func (r *Resolver) follow(address string) string {
	current := address
	for hop := 0; hop < maxHops; hop++ {
		resp, err := r.client.Get(current)
		if err != nil {
			slog.Debug("tracking link not followed", "url", address, "error", err)
			return ""
		}
		resp.Body.Close()
		location := resp.Header.Get("Location")
		if resp.StatusCode < 300 || resp.StatusCode > 399 || location == "" {
			return ""
		}
		next, err := resp.Request.URL.Parse(location)
		if err != nil {
			return ""
		}
		current = next.String()
		if !tracking(current) {
			return clean(current)
		}
	}
	return ""
}

// clean is a destination without the marks the mailer added to count clicks.
func clean(address string) string {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	query := u.Query()
	for key := range query {
		if strings.HasPrefix(key, "utm_") {
			query.Del(key)
		}
	}
	u.RawQuery = query.Encode()
	return u.String()
}
