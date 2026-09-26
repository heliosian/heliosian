package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
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
	Bucket       = "heliosian-media"
	thumbWidth   = 480
	thumbSuffix  = "-thumb"
	thumbExt     = ".jpg"
	thumbMime    = "image/jpeg"
	thumbVersion = "1"
	fetchWorkers = 32
	maxPixels    = 40_000_000
)

var folders = []string{"photos", "pronunciation", "link-images", "activity-images", "category-images"}

var ErrNotFound = errors.New("no such object")

type object struct {
	mimeType   string
	generation int64
	data       []byte
}

type objects interface {
	get(ctx context.Context, name string) (object, error)
	put(ctx context.Context, name, mimeType string, content []byte) error
	exists(ctx context.Context, name string) (bool, error)
	remove(ctx context.Context, name string) error
}

type bucket struct {
	service *storage.Service
	name    string
}

func newBucket(name string) (bucket, error) {
	service, err := storage.NewService(context.Background(),
		option.WithScopes(storage.DevstorageReadWriteScope))
	if err != nil {
		return bucket{}, fmt.Errorf("storage client: %w", err)
	}
	return bucket{service: service, name: name}, nil
}

func (b bucket) get(ctx context.Context, name string) (object, error) {
	resp, err := b.service.Objects.Get(b.name, name).Context(ctx).Download()
	if notFound(err) {
		return object{}, ErrNotFound
	}
	if err != nil {
		return object{}, fmt.Errorf("read %s: %w", name, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return object{}, fmt.Errorf("read %s: %w", name, err)
	}
	generation, err := strconv.ParseInt(resp.Header.Get("x-goog-generation"), 10, 64)
	if err != nil {
		return object{}, fmt.Errorf("read %s: no generation on the response: %w", name, err)
	}
	return object{mimeType: resp.Header.Get("Content-Type"), generation: generation, data: data}, nil
}

func (b bucket) put(ctx context.Context, name, mimeType string, content []byte) error {
	// A chunk size of zero sends the object in one request; the default of
	// sixteen megabytes is allocated whole for every upload, however small.
	_, err := b.service.Objects.Insert(b.name, &storage.Object{Name: name, ContentType: mimeType}).
		Media(bytes.NewReader(content), googleapi.ContentType(mimeType), googleapi.ChunkSize(0)).
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func (b bucket) exists(ctx context.Context, name string) (bool, error) {
	_, err := b.service.Objects.Get(b.name, name).Fields("name").Context(ctx).Do()
	if notFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", name, err)
	}
	return true, nil
}

func (b bucket) remove(ctx context.Context, name string) error {
	err := b.service.Objects.Delete(b.name, name).Context(ctx).Do()
	if notFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete %s: %w", name, err)
	}
	return nil
}

type memory struct {
	mu         sync.Mutex
	objects    map[string]object
	generation int64
}

func (m *memory) get(_ context.Context, name string) (object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objects[name]
	if !ok {
		return object{}, ErrNotFound
	}
	return o, nil
}

func (m *memory) put(_ context.Context, name, mimeType string, content []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.generation++
	m.objects[name] = object{mimeType: mimeType, generation: m.generation, data: content}
	return nil
}

func (m *memory) exists(_ context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objects[name]
	return ok, nil
}

func (m *memory) remove(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, name)
	return nil
}

func Media(path string) bool {
	folder, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	return slices.Contains(folders, folder)
}

func trimExt(name string) string {
	return strings.TrimSuffix(name, path.Ext(name))
}

func thumbName(name string) string {
	return trimExt(name) + thumbSuffix + thumbExt
}

type entry struct {
	name       string
	generation int64
	mimeType   string
	data       []byte
	thumb      []byte
}

type Store struct {
	objects  objects
	cacheDir string
	mu       sync.RWMutex
	entries  map[string]*entry
	named    map[string]bool
}

func New(cacheDir string) (*Store, error) {
	b, err := newBucket(Bucket)
	if err != nil {
		return nil, err
	}
	s := &Store{objects: b, cacheDir: cacheDir, entries: map[string]*entry{}}
	if cacheDir != "" {
		slog.Info("blob store: caching fetched objects on disk", "dir", cacheDir)
	}
	return s, nil
}

func NewMemory() *Store {
	return &Store{objects: &memory{objects: map[string]object{}}, entries: map[string]*entry{}}
}

func (s *Store) Read(ctx context.Context, name string) ([]byte, string, error) {
	o, err := s.objects.get(ctx, name)
	if err != nil {
		return nil, "", err
	}
	return o.data, o.mimeType, nil
}

func (s *Store) Write(ctx context.Context, name, mimeType string, content []byte) error {
	return s.objects.put(ctx, name, mimeType, content)
}

func (s *Store) Exists(ctx context.Context, name string) (bool, error) {
	return s.objects.exists(ctx, name)
}

func (s *Store) cachePath(name string) string {
	return filepath.Join(s.cacheDir, filepath.FromSlash(name))
}

func (s *Store) cached(name string) (*entry, error) {
	if s.cacheDir == "" {
		return nil, nil
	}
	meta, err := os.ReadFile(s.cachePath(name) + ".meta")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the cache of %s: %w", name, err)
	}
	generationText, mimeType, ok := strings.Cut(strings.TrimSpace(string(meta)), "\n")
	if !ok {
		return nil, fmt.Errorf("read the cache of %s: the meta file is not generation and type", name)
	}
	generation, err := strconv.ParseInt(generationText, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("read the cache of %s: %w", name, err)
	}
	data, err := os.ReadFile(s.cachePath(name))
	if err != nil {
		return nil, fmt.Errorf("read the cache of %s: %w", name, err)
	}
	e := &entry{name: name, generation: generation, mimeType: mimeType, data: data}
	if strings.HasPrefix(mimeType, "image/") {
		if e.thumb, err = os.ReadFile(s.cachePath(thumbName(name))); err != nil {
			return nil, fmt.Errorf("read the cache of %s: %w", name, err)
		}
	}
	return e, nil
}

func (s *Store) cache(e *entry) error {
	if s.cacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.cachePath(e.name)), 0o755); err != nil {
		return fmt.Errorf("cache %s: %w", e.name, err)
	}
	if err := writeFile(s.cachePath(e.name), e.data); err != nil {
		return fmt.Errorf("cache %s: %w", e.name, err)
	}
	if e.thumb != nil {
		if err := writeFile(s.cachePath(thumbName(e.name)), e.thumb); err != nil {
			return fmt.Errorf("cache %s: %w", e.name, err)
		}
	}
	if err := writeFile(s.cachePath(e.name)+".meta", []byte(strconv.FormatInt(e.generation, 10)+"\n"+e.mimeType+"\n")); err != nil {
		return fmt.Errorf("cache %s: %w", e.name, err)
	}
	return nil
}

func writeFile(name string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), filepath.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), name)
}

func Register(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
	mux.HandleFunc("GET /pronunciation/{name}", s.serve)
}

func RegisterHome(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /link-images/{name}", s.serve)
	mux.HandleFunc("GET /photos/{name}", s.serve)
}

func RegisterTeam(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
	mux.HandleFunc("GET /activity-images/{name}", s.serve)
}

func RegisterBirthday(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
}

func RegisterCelebrate(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
	mux.HandleFunc("GET /party-images/{name}", s.serve)
}

func RegisterCalendar(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
	mux.HandleFunc("GET /category-images/{name}", s.serve)
}

func RegisterLoop(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
}

func RegisterAsk(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
}

func (s *Store) Load(context.Context) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.named = map[string]bool{}
	return s.drop, nil
}

func (s *Store) drop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	dropped := 0
	for key := range s.entries {
		if !s.named[key] {
			delete(s.entries, key)
			dropped++
		}
	}
	s.named = nil
	if dropped > 0 {
		slog.Info("blob store: dropped what no sheet names", "dropped", dropped, "kept", len(s.entries))
	}
}

func notFound(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound
}

func (s *Store) held(key string) (*entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key]
	return e, ok
}

func (s *Store) keep(key string) (*entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if ok && s.named != nil {
		s.named[key] = true
	}
	return e, ok
}

func (s *Store) Get(name string) ([]byte, error) {
	e, err := s.fetch(context.Background(), name)
	if err != nil {
		return nil, err
	}
	return e.data, nil
}

func (s *Store) Bytes(name string) ([]byte, string, bool) {
	e, ok := s.held(trimExt(name))
	if !ok {
		return nil, "", false
	}
	return e.data, e.mimeType, true
}

func (s *Store) Has(name string) (bool, error) {
	return s.has(context.Background(), name)
}

func (s *Store) has(ctx context.Context, name string) (bool, error) {
	key := trimExt(name)
	if _, ok := s.keep(key); ok {
		return true, nil
	}
	start := time.Now()
	e, err := s.fetch(ctx, name)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	s.entries[key] = e
	if s.named != nil {
		s.named[key] = true
	}
	held := len(s.entries)
	s.mu.Unlock()
	slog.Info("blob store: fetched", "name", name, "bytes", len(e.data)+len(e.thumb), "held", held, "took", time.Since(start).Round(time.Millisecond))
	return true, nil
}

func (s *Store) Prefetch(ctx context.Context, names []string) error {
	start := time.Now()
	pending := []string{}
	for _, name := range names {
		if _, ok := s.keep(trimExt(name)); !ok {
			pending = append(pending, name)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	slog.Info("blob store: prefetching", "names", len(pending))
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	slots := make(chan struct{}, fetchWorkers)
	for _, name := range pending {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			if _, err := s.has(ctx, name); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	slog.Info("blob store: prefetched", "names", len(pending), "held", s.count(), "took", time.Since(start).Round(time.Millisecond))
	return firstErr
}

func (s *Store) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *Store) fetch(ctx context.Context, name string) (*entry, error) {
	if e, err := s.cached(name); err != nil || e != nil {
		return e, err
	}
	e, err := s.download(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := s.cache(e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Store) download(ctx context.Context, name string) (*entry, error) {
	o, err := s.objects.get(ctx, name)
	if err != nil {
		return nil, err
	}
	e := &entry{name: name, generation: o.generation, mimeType: o.mimeType, data: o.data}
	if !strings.HasPrefix(o.mimeType, "image/") {
		return e, nil
	}
	thumb, err := s.objects.get(ctx, thumbName(name))
	if errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("no thumbnail stored for %s", name)
	}
	if err != nil {
		return nil, err
	}
	e.thumb = thumb.data
	return e, nil
}

type Uploader struct {
	objects objects
}

func NewUploader() (*Uploader, error) {
	b, err := newBucket(Bucket)
	if err != nil {
		return nil, err
	}
	return &Uploader{objects: b}, nil
}

func (u *Uploader) Has(name string) (bool, error) {
	return u.objects.exists(context.Background(), name)
}

func (u *Uploader) Remove(name string) error {
	return u.objects.remove(context.Background(), name)
}

func (u *Uploader) Put(folder, name, mimeType string, content []byte) (bool, error) {
	ctx := context.Background()
	full := folder + "/" + name
	present, err := u.objects.exists(ctx, full)
	if err != nil {
		return false, err
	}
	if present && strings.HasPrefix(mimeType, "image/") {
		present, err = u.objects.exists(ctx, thumbName(full))
		if err != nil {
			return false, err
		}
	}
	if present {
		return false, nil
	}
	if err := writeWithThumbnail(ctx, u.objects, full, mimeType, content); err != nil {
		return false, err
	}
	return true, nil
}

func writeWithThumbnail(ctx context.Context, into objects, name, mimeType string, content []byte) error {
	if !strings.HasPrefix(mimeType, "image/") {
		return into.put(ctx, name, mimeType, content)
	}
	thumb, err := Thumbnail(content)
	if err != nil {
		return fmt.Errorf("thumbnail %s: %w", name, err)
	}
	if err := into.put(ctx, name, mimeType, content); err != nil {
		return err
	}
	return into.put(ctx, thumbName(name), thumbMime, thumb)
}

func (s *Store) Put(folder, name, mimeType string, content []byte) error {
	full := folder + "/" + name
	if _, ok := s.keep(trimExt(full)); ok {
		return nil
	}
	if err := writeWithThumbnail(context.Background(), s.objects, full, mimeType, content); err != nil {
		return err
	}
	return s.take(full)
}

func (s *Store) take(name string) error {
	found, err := s.Has(name)
	if err != nil {
		return fmt.Errorf("read back %s: %w", name, err)
	}
	if !found {
		return fmt.Errorf("read back %s: not there after writing it", name)
	}
	return nil
}

func (s *Store) serve(w http.ResponseWriter, r *http.Request) {
	key := trimExt(strings.TrimPrefix(r.URL.Path, "/"))
	e, ok := s.held(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, e.generation))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if thumb := r.URL.Query().Get("thumb"); thumb != "" {
		if thumb != thumbVersion || e.thumb == nil {
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

func Decode(src []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxPixels/cfg.Height {
		return nil, fmt.Errorf("image declares %dx%d pixels, over the limit of %d", cfg.Width, cfg.Height, maxPixels)
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	return img, err
}

func Thumbnail(src []byte) ([]byte, error) {
	img, err := Decode(src)
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
