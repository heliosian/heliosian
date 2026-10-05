package db

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"heliosian/internal/cells"
)

const (
	imageTimeout = 30 * time.Second
	imageLimit   = 25 << 20
	imageFetches = 8
)

var imageClient = &http.Client{Timeout: imageTimeout}

type Image struct {
	Body      []byte
	Mime, URL string
}

func htmlImages(ctx context.Context, raw []byte) ([]extracted, error) {
	srcs, err := ImageSources(raw)
	if err != nil {
		return nil, err
	}
	found := make([]*Image, len(srcs))
	slots := make(chan struct{}, imageFetches)
	wg := sync.WaitGroup{}
	for i, src := range srcs {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			got, err := FetchImage(ctx, src)
			if err != nil {
				slog.Warn("extract: skipped image", "src", ShortSource(src), "error", err)
				return
			}
			found[i] = &got
		})
	}
	wg.Wait()
	out := []extracted{}
	for _, got := range found {
		if got != nil {
			out = append(out, extracted{relation: "linked", body: got.Body, mime: got.Mime, url: got.URL})
		}
	}
	return out, nil
}

func ImageSources(raw []byte) ([]string, error) {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	out := []string{}
	seen := map[string]bool{}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data != "img" {
			continue
		}
		src := ""
		pixel := false
		for _, attr := range n.Attr {
			switch attr.Key {
			case "src":
				src = strings.TrimSpace(attr.Val)
			case "width", "height":
				size, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(attr.Val), "px"))
				pixel = pixel || (err == nil && size <= 1)
			}
		}
		lower := strings.ToLower(src)
		if pixel || seen[src] || !(strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")) {
			continue
		}
		seen[src] = true
		out = append(out, src)
	}
	return out, nil
}

func FetchImage(ctx context.Context, src string) (Image, error) {
	body, address, err := imageBytes(ctx, src)
	if err != nil {
		return Image{}, err
	}
	mimeType := http.DetectContentType(body)
	if !strings.HasPrefix(mimeType, "image/") {
		return Image{}, fmt.Errorf("%s is not an image", mimeType)
	}
	if config, _, err := image.DecodeConfig(bytes.NewReader(body)); err == nil && (config.Width <= 1 || config.Height <= 1) {
		return Image{}, fmt.Errorf("a %dx%d tracking pixel", config.Width, config.Height)
	}
	return Image{Body: body, Mime: mimeType, URL: address}, nil
}

func imageBytes(ctx context.Context, src string) ([]byte, string, error) {
	if strings.HasPrefix(strings.ToLower(src), "data:") {
		body, err := dataBytes(src[len("data:"):])
		return body, "", err
	}
	if err := cells.URL(src, false); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := imageClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, imageLimit+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > imageLimit {
		return nil, "", fmt.Errorf("larger than %d bytes", imageLimit)
	}
	return body, src, nil
}

func dataBytes(data string) ([]byte, error) {
	meta, payload, ok := strings.Cut(data, ",")
	if !ok {
		return nil, errors.New("a data url with no comma")
	}
	unescaped, err := url.PathUnescape(payload)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(strings.ToLower(meta), ";base64") {
		return []byte(unescaped), nil
	}
	return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(unescaped), ""))
}

func ShortSource(src string) string {
	if len(src) > 200 {
		return src[:200] + "…"
	}
	return src
}
