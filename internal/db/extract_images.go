package db

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"heliosian/internal/cells"
)

func htmlImages(raw []byte) ([]extracted, error) {
	srcs, err := ImageSources(raw)
	if err != nil {
		return nil, err
	}
	out := []extracted{}
	for _, src := range srcs {
		if !strings.HasPrefix(strings.ToLower(src), "data:") {
			if err := cells.URL(src, false); err != nil {
				slog.Warn("extract: skipped image", "src", ShortSource(src), "error", err)
				continue
			}
			out = append(out, extracted{relation: "image", url: src})
			continue
		}
		body, err := dataBytes(src[len("data:"):])
		if err != nil {
			slog.Warn("extract: skipped image", "src", ShortSource(src), "error", err)
			continue
		}
		mimeType, err := keptImage(body)
		if err != nil {
			slog.Warn("extract: skipped image", "src", ShortSource(src), "error", err)
			continue
		}
		out = append(out, extracted{relation: "image", body: body, mime: mimeType})
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

func keptImage(body []byte) (string, error) {
	mimeType := http.DetectContentType(body)
	if !strings.HasPrefix(mimeType, "image/") {
		return "", fmt.Errorf("%s is not an image", mimeType)
	}
	if config, _, err := image.DecodeConfig(bytes.NewReader(body)); err == nil && (config.Width <= 1 || config.Height <= 1) {
		return "", fmt.Errorf("a %dx%d tracking pixel", config.Width, config.Height)
	}
	return mimeType, nil
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
