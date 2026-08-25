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

var folders = []string{"photos", "pronunciation"}

// Recorded names carry an extension and entries are keyed without one, so an object
// and its thumbnail share a key.
func trimExt(name string) string {
	return strings.TrimSuffix(name, path.Ext(name))
}

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
	// One route per kind, and the object name is a content hash, so nothing about
	// where a blob is stored or who it belongs to reaches the client.
	mux.HandleFunc("GET /photos/{name}", s.serve)
	mux.HandleFunc("GET /pronunciation/{name}", s.serve)
}

func (s *Store) refreshLoop() {
	for range time.Tick(refreshInterval) {
		if err := s.refresh(); err != nil {
			log.Printf("[ERROR] blob refresh: %v", err)
		}
	}
}

func list(ctx context.Context, service *storage.Service, folder string) ([]object, error) {
	objects := []object{}
	token := ""
	for {
		call := service.Objects.List(Bucket).Prefix(folder+"/").
			Fields("nextPageToken", "items(name,generation,contentType)").MaxResults(1000)
		if token != "" {
			call = call.PageToken(token)
		}
		page, err := call.Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", folder, err)
		}
		for _, item := range page.Items {
			objects = append(objects, object{
				name: item.Name, generation: item.Generation, mimeType: item.ContentType,
			})
		}
		if page.NextPageToken == "" {
			return objects, nil
		}
		token = page.NextPageToken
	}
}

func (s *Store) refresh() error {
	start := time.Now()
	ctx := context.Background()
	primaries := map[string]object{}
	thumbs := map[string]object{}
	for _, folder := range folders {
		objects, err := list(ctx, s.service, folder)
		if err != nil {
			return err
		}
		for _, o := range objects {
			base := trimExt(path.Base(o.name))
			if strings.HasSuffix(base, thumbSuffix) {
				thumbs[folder+"/"+strings.TrimSuffix(base, thumbSuffix)] = o
				continue
			}
			key := folder + "/" + base
			if existing, ok := primaries[key]; ok {
				return fmt.Errorf("duplicate media for %s: %s and %s", key, existing.name, o.name)
			}
			primaries[key] = o
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
			log.Printf("blob store: fetched %d/%d %s, %d bytes", len(fetched), len(missing), key, len(e.data)+len(e.thumb))
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

// Uploader fills the bucket for the tools that load media in bulk. It lists names
// rather than holding every object in memory the way the serving Store does.
type Uploader struct {
	service *storage.Service
	present map[string]bool
}

func NewUploader() (*Uploader, error) {
	service, err := storage.NewService(context.Background(),
		option.WithScopes(storage.DevstorageReadWriteScope))
	if err != nil {
		return nil, fmt.Errorf("storage client: %w", err)
	}
	u := &Uploader{service: service, present: map[string]bool{}}
	for _, folder := range folders {
		objects, err := list(context.Background(), service, folder)
		if err != nil {
			return nil, err
		}
		for _, o := range objects {
			u.present[trimExt(o.name)] = true
		}
	}
	return u, nil
}

// Has reports whether the bucket holds an object, so a tool can resolve the names the
// sheet records without downloading anything.
func (u *Uploader) Has(key string) bool {
	return u.present[trimExt(key)]
}

// Put writes a content-addressed object and its thumbnail, and reports whether it had
// to. The name already being present means the same bytes by construction, but the
// thumbnail is checked separately: a primary written without one is an object the
// serving store refuses to load, and skipping on the primary alone leaves it that way.
func (u *Uploader) Put(folder, name, mimeType string, content []byte) (bool, error) {
	ctx := context.Background()
	key := folder + "/" + trimExt(name)
	wrote := false
	if !u.present[key] {
		if err := write(ctx, u.service, folder+"/"+name, mimeType, content); err != nil {
			return false, err
		}
		u.present[key] = true
		wrote = true
	}
	if !strings.HasPrefix(mimeType, "image/") || u.present[key+thumbSuffix] {
		return wrote, nil
	}
	if err := writeThumbnail(ctx, u.service, folder, name, content); err != nil {
		return false, err
	}
	u.present[key+thumbSuffix] = true
	return true, nil
}

// Repair writes the thumbnails that objects already in the bucket are missing. The
// serving store treats an image without one as fatal, so a primary written on its own
// is not a degraded object but a bucket that will not load at all.
func (u *Uploader) Repair() (int, error) {
	ctx := context.Background()
	repaired := 0
	for _, folder := range folders {
		objects, err := list(ctx, u.service, folder)
		if err != nil {
			return repaired, err
		}
		for _, o := range objects {
			base := trimExt(path.Base(o.name))
			if strings.HasSuffix(base, thumbSuffix) || !strings.HasPrefix(o.mimeType, "image/") {
				continue
			}
			if u.present[folder+"/"+base+thumbSuffix] {
				continue
			}
			content, err := read(ctx, u.service, o.name)
			if err != nil {
				return repaired, err
			}
			if err := writeThumbnail(ctx, u.service, folder, path.Base(o.name), content); err != nil {
				return repaired, err
			}
			u.present[folder+"/"+base+thumbSuffix] = true
			log.Printf("blob repair: wrote the missing thumbnail for %s", o.name)
			repaired++
		}
	}
	return repaired, nil
}

func read(ctx context.Context, service *storage.Service, name string) ([]byte, error) {
	resp, err := service.Objects.Get(Bucket, name).Context(ctx).Download()
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

func (s *Store) read(ctx context.Context, name string) ([]byte, error) {
	return read(ctx, s.service, name)
}

func write(ctx context.Context, service *storage.Service, name, mimeType string, content []byte) error {
	_, err := service.Objects.Insert(Bucket, &storage.Object{Name: name, ContentType: mimeType}).
		Media(bytes.NewReader(content), googleapi.ContentType(mimeType)).
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func writeThumbnail(ctx context.Context, service *storage.Service, folder, name string, content []byte) error {
	thumb, err := Thumbnail(content)
	if err != nil {
		return fmt.Errorf("thumbnail %s: %w", name, err)
	}
	return write(ctx, service, folder+"/"+trimExt(name)+thumbSuffix+thumbExt, thumbMime, thumb)
}

func writeWithThumbnail(ctx context.Context, service *storage.Service, folder, name, mimeType string, content []byte) error {
	if err := write(ctx, service, folder+"/"+name, mimeType, content); err != nil {
		return err
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return nil
	}
	return writeThumbnail(ctx, service, folder, name, content)
}

func (s *Store) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.entries[trimExt(key)]
	return ok
}

// Put writes a content-addressed object and its thumbnail. A name already present is
// byte-identical by construction, so the write is skipped rather than repeated.
func (s *Store) Put(folder, name, mimeType string, content []byte) error {
	key := folder + "/" + trimExt(name)
	s.mu.RLock()
	_, exists := s.entries[key]
	s.mu.RUnlock()
	if exists {
		return nil
	}
	if err := writeWithThumbnail(context.Background(), s.service, folder, name, mimeType, content); err != nil {
		return err
	}
	if err := s.refresh(); err != nil {
		return fmt.Errorf("refresh after upload: %w", err)
	}
	return nil
}

func (s *Store) serve(w http.ResponseWriter, r *http.Request) {
	key := trimExt(strings.TrimPrefix(r.URL.Path, "/"))
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
