package db

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font/opentype"

	"heliosian/internal/blob"
)

const googleFontsCSS = "https://fonts.googleapis.com/css2"

var fontFileURL = regexp.MustCompile(`url\((https://[^)\s]+)\)`)

type fontFace struct {
	family string
	weight int
	italic bool
}

func (f fontFace) object() string {
	name := strings.ToLower(strings.ReplaceAll(f.family, " ", "-")) + "-" + strconv.Itoa(f.weight)
	if f.italic {
		name += "-italic"
	}
	return "fonts/" + name + ".ttf"
}

type themeFonts struct {
	bucket *blob.Bucket
	mu     sync.Mutex
	held   map[fontFace]*opentype.Font
}

func newThemeFonts(bucket *blob.Bucket) *themeFonts {
	return &themeFonts{bucket: bucket, held: map[fontFace]*opentype.Font{}}
}

func loadedFaces(spec string) (string, []fontFace) {
	family, axes, _ := strings.Cut(spec, ":")
	family = strings.TrimSpace(family)
	tags, values, found := strings.Cut(axes, "@")
	if !found {
		return family, []fontFace{{family: family, weight: 400}}
	}
	names := strings.Split(tags, ",")
	faces := []fontFace{}
	for _, tuple := range strings.Split(values, ";") {
		f := fontFace{family: family, weight: 400}
		for i, v := range strings.Split(tuple, ",") {
			if i >= len(names) {
				break
			}
			switch names[i] {
			case "ital":
				f.italic = v == "1"
			case "wght":
				f.weight, _ = strconv.Atoi(v)
			}
		}
		faces = append(faces, f)
	}
	return family, faces
}

func themeFace(design map[string]any, role string) (fontFace, bool) {
	stack := designValue(design, "fonts."+designValue(design, "text."+role+".font"))
	weight, _ := strconv.Atoi(designValue(design, "text."+role+".weight"))
	italic := designValue(design, "text."+role+".style") == "italic"
	loads, _ := designAt(design, "fonts.load").([]any)
	for _, name := range strings.Split(stack, ",") {
		name = strings.Trim(strings.TrimSpace(name), `'"`)
		for _, l := range loads {
			spec, _ := l.(string)
			family, faces := loadedFaces(spec)
			if family != name {
				continue
			}
			distance := func(f fontFace) int {
				d := abs(f.weight - weight)
				if f.italic != italic {
					d += 1000
				}
				return d
			}
			best := faces[0]
			for _, f := range faces[1:] {
				if distance(f) < distance(best) {
					best = f
				}
			}
			return best, true
		}
	}
	return fontFace{}, false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (t *themeFonts) font(ctx context.Context, f fontFace) (*opentype.Font, error) {
	t.mu.Lock()
	held, ok := t.held[f]
	t.mu.Unlock()
	if ok {
		return held, nil
	}
	data, _, err := t.bucket.Get(ctx, f.object())
	if errors.Is(err, blob.ErrNotFound) {
		data, err = fetchFont(ctx, f)
		if err != nil {
			return nil, err
		}
		if _, err := t.bucket.Put(ctx, f.object(), "font/ttf", data, 0); err != nil && !errors.Is(err, blob.ErrChanged) {
			return nil, err
		}
		slog.InfoContext(ctx, "fonts: fetched from google fonts", "family", f.family, "weight", f.weight, "italic", f.italic, "bytes", len(data))
	}
	if err != nil {
		return nil, err
	}
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("font %s: %w", f.object(), err)
	}
	t.mu.Lock()
	t.held[f] = parsed
	t.mu.Unlock()
	return parsed, nil
}

func fetchFont(ctx context.Context, f fontFace) ([]byte, error) {
	ital := "0"
	if f.italic {
		ital = "1"
	}
	q := url.Values{"family": {f.family + ":ital,wght@" + ital + "," + strconv.Itoa(f.weight)}}
	css, err := fetchFontPart(ctx, googleFontsCSS+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	found := fontFileURL.FindSubmatch(css)
	if found == nil {
		return nil, fmt.Errorf("google fonts named no file for %s %d: %s", f.family, f.weight, css)
	}
	return fetchFontPart(ctx, string(found[1]))
}

func fetchFontPart(ctx context.Context, address string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "heliosian")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google fonts answered %d for %s: %s", res.StatusCode, address, body)
	}
	return body, nil
}
