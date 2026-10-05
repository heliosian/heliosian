package tomarkdown

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"io"
	"net/url"
	"strings"
)

const (
	veracrossHost = ".veracross.com"
	trackingPath  = "/c/"
)

type LinkResolver struct {
	Dropped int
}

func trackingLink(address string) bool {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return strings.HasSuffix(u.Host, veracrossHost) && strings.HasPrefix(u.Path, trackingPath)
}

func (r *LinkResolver) Links(base *url.URL) func(string) string {
	return func(href string) string {
		target, err := base.Parse(href)
		if err != nil {
			return ""
		}
		return r.Resolve(unwrapGoogle(target).String())
	}
}

func unwrapGoogle(u *url.URL) *url.URL {
	if (u.Host != "www.google.com" && u.Host != "google.com") || u.Path != "/url" {
		return u
	}
	target, err := url.Parse(u.Query().Get("q"))
	if err != nil || target.Host == "" {
		return u
	}
	return target
}

func (r *LinkResolver) Resolve(address string) string {
	if !trackingLink(address) {
		return address
	}
	target := unwrapTracking(address)
	if target == "" {
		r.Dropped++
	}
	return target
}

func unwrapTracking(address string) string {
	u, err := url.Parse(address)
	if err != nil {
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
	return cleanLink(query.Get("l"))
}

func cleanLink(address string) string {
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
