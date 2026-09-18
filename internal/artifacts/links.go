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
	benchmarkLink = "/c/l"
	maxHops       = 4
	workers       = 12
)

// Benchmark's redirect under the school's own name (r560896.heliosschool.org).
var benchmarkVanity = regexp.MustCompile(`^r[0-9]+\.`)

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

func dead(u *url.URL) bool {
	if strings.HasSuffix(u.Host, veracrossHost) {
		return false
	}
	return strings.HasPrefix(u.Path, trackingPath) && u.Path != benchmarkLink
}

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

// A Veracross link's path is a deflated query whose l is the destination.
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
