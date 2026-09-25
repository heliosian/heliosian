package artifacts

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

type Resolver struct {
	Dropped int
}

func NewResolver() *Resolver {
	return &Resolver{}
}

func tracking(address string) bool {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return strings.HasSuffix(u.Host, veracrossHost) && strings.HasPrefix(u.Path, trackingPath)
}

func (r *Resolver) Resolve(address string) string {
	if !tracking(address) {
		return address
	}
	target := unwrap(address)
	if target == "" {
		r.Dropped++
	}
	return target
}

// A Veracross link's path is a deflated query whose l is the destination.
func unwrap(address string) string {
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
	return clean(query.Get("l"))
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
