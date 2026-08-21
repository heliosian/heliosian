// Package blob serves directory media from cloud storage, held fully in memory with stored thumbnails.
package blob

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	_ "image/gif"
	_ "image/png"

	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	storage "google.golang.org/api/storage/v1"
)

const (
	Bucket          = "heliosian-media"
	refreshInterval = 5 * time.Minute
	thumbWidth      = 480
	thumbSuffix     = "-thumb"
	thumbExt        = ".jpg"
	thumbMime       = "image/jpeg"
	fetchWorkers    = 32
)

var folders = []string{"people", "families"}

type entry struct {
	name       string
	generation int64
	mimeType   string
	data       []byte
	thumb      []byte
}

type object struct {
	name       string
	generation int64
	mimeType   string
}

type Store struct {
	service *storage.Service
	mu      sync.RWMutex
	entries map[string]*entry
}

func New() (*Store, error) {
	service, err := storage.NewService(context.Background(),
		option.WithScopes(storage.DevstorageReadWriteScope))
	if err != nil {
		return nil, fmt.Errorf("storage client: %w", err)
	}
	s := &Store{service: service, entries: map[string]*entry{}}
	if err := s.refresh(); err != nil {
		return nil, err
	}
	go s.refreshLoop()
	return s, nil
}

func Register(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /blob/{folder}/{name}", s.serve)
}

func (s *Store) refreshLoop() {
	for range time.Tick(refreshInterval) {
		if err := s.refresh(); err != nil {
			log.Printf("[ERROR] blob refresh: %v", err)
		}
	}
}

func (s *Store) refresh() error {
	start := time.Now()
	ctx := context.Background()
	primaries := map[string]object{}
	thumbs := map[string]object{}
	for _, folder := range folders {
		token := ""
		for {
			call := s.service.Objects.List(Bucket).Prefix(folder+"/").
				Fields("nextPageToken", "items(name,generation,contentType)").MaxResults(1000)
			if token != "" {
				call = call.PageToken(token)
			}
			list, err := call.Context(ctx).Do()
			if err != nil {
				return fmt.Errorf("list %s: %w", folder, err)
			}
			for _, item := range list.Items {
				base := strings.TrimSuffix(path.Base(item.Name), path.Ext(item.Name))
				o := object{name: item.Name, generation: item.Generation, mimeType: item.ContentType}
				if strings.HasSuffix(base, thumbSuffix) {
					thumbs[folder+"/"+strings.TrimSuffix(base, thumbSuffix)] = o
					continue
				}
				key := folder + "/" + base
				if existing, ok := primaries[key]; ok {
					return fmt.Errorf("duplicate media for %s: %s and %s", key, existing.name, item.Name)
				}
				primaries[key] = o
			}
			if list.NextPageToken == "" {
				break
			}
			token = list.NextPageToken
		}
	}

	s.mu.RLock()
	missing := []string{}
	for key, o := range primaries {
		if cached, ok := s.entries[key]; !ok || cached.generation != o.generation {
			missing = append(missing, key)
		}
	}
	s.mu.RUnlock()

	fetched := map[string]*entry{}
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	slots := make(chan struct{}, fetchWorkers)
	for _, key := range missing {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			e, err := s.load(ctx, primaries[key], thumbs[key])
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			fetched[key] = e
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}

	next := make(map[string]*entry, len(primaries))
	var totalBytes int64
	s.mu.Lock()
	for key, o := range primaries {
		if e, ok := fetched[key]; ok {
			next[key] = e
		} else if cached, ok := s.entries[key]; ok && cached.generation == o.generation {
			next[key] = cached
		}
	}
	s.entries = next
	for _, e := range next {
		totalBytes += int64(len(e.data) + len(e.thumb))
	}
	s.mu.Unlock()
	log.Printf("blob store: %d files, %d fetched, %.1f MB in memory in %s",
		len(next), len(fetched), float64(totalBytes)/1e6, time.Since(start).Round(time.Millisecond))
	return nil
}

func (s *Store) load(ctx context.Context, primary, thumb object) (*entry, error) {
	data, err := s.read(ctx, primary.name)
	if err != nil {
		return nil, err
	}
	e := &entry{name: primary.name, generation: primary.generation, mimeType: primary.mimeType, data: data}
	if !strings.HasPrefix(primary.mimeType, "image/") {
		return e, nil
	}
	if thumb.name == "" {
		return nil, fmt.Errorf("no thumbnail stored for %s", primary.name)
	}
	e.thumb, err = s.read(ctx, thumb.name)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Store) read(ctx context.Context, name string) ([]byte, error) {
	resp, err := s.service.Objects.Get(Bucket, name).Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return content, nil
}

func (s *Store) write(ctx context.Context, name, mimeType string, content []byte) error {
	_, err := s.service.Objects.Insert(Bucket, &storage.Object{Name: name, ContentType: mimeType}).
		Media(bytes.NewReader(content), googleapi.ContentType(mimeType)).
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func (s *Store) Refresh() error {
	return s.refresh()
}

func (s *Store) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.entries[key]
	return ok
}

func (s *Store) Upload(folder, base, ext, mimeType string, content []byte) (string, error) {
	ctx := context.Background()
	key := folder + "/" + base
	name := key + "." + ext
	s.mu.RLock()
	existing, exists := s.entries[key]
	s.mu.RUnlock()
	if err := s.write(ctx, name, mimeType, content); err != nil {
		return "", err
	}
	if strings.HasPrefix(mimeType, "image/") {
		thumb, err := Thumbnail(content)
		if err != nil {
			return "", fmt.Errorf("thumbnail %s: %w", name, err)
		}
		if err := s.write(ctx, key+thumbSuffix+thumbExt, thumbMime, thumb); err != nil {
			return "", err
		}
	}
	superseded := ""
	if exists {
		superseded = fmt.Sprint(existing.generation)
		if existing.name != name {
			if err := s.service.Objects.Delete(Bucket, existing.name).Context(ctx).Do(); err != nil {
				return "", fmt.Errorf("delete superseded %s: %w", existing.name, err)
			}
		}
	}
	if err := s.refresh(); err != nil {
		return "", fmt.Errorf("refresh after upload: %w", err)
	}
	return superseded, nil
}

func (s *Store) serve(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("folder") + "/" + r.PathValue("name")
	s.mu.RLock()
	e, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("thumb") == "1" {
		if e.thumb == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", thumbMime)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(e.thumb))
		return
	}
	w.Header().Set("Content-Type", e.mimeType)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(e.data))
}

func Thumbnail(src []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	o := orientation(src)
	bounds := img.Bounds()
	displayWidth := bounds.Dx()
	if o >= 5 {
		displayWidth = bounds.Dy()
	}
	if displayWidth > thumbWidth {
		w := bounds.Dx() * thumbWidth / displayWidth
		h := bounds.Dy() * thumbWidth / displayWidth
		scaled := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, bounds, draw.Over, nil)
		img = scaled
	}
	img = reorient(img, o)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func orientation(src []byte) (o int) {
	o = 1
	defer func() { recover() }()
	parsed, err := exif.Decode(bytes.NewReader(src))
	if err != nil {
		return
	}
	tag, err := parsed.Get(exif.Orientation)
	if err != nil {
		return
	}
	value, err := tag.Int(0)
	if err != nil || value < 1 || value > 8 {
		return
	}
	return value
}

func reorient(img image.Image, o int) image.Image {
	if o == 1 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			out.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}
