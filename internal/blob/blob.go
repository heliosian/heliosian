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
	"path"
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

	"heliosian/internal/lru"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	storage "google.golang.org/api/storage/v1"
)

const (
	MediaBucket  = "heliosian-media"
	MailBucket   = "heliosian-mail"
	thumbWidth   = 480
	thumbSuffix  = "-thumb"
	thumbExt     = ".jpg"
	thumbMime    = "image/jpeg"
	thumbVersion = "1"
	fetchWorkers = 32
	maxPixels    = 40_000_000
	dataBudget   = 256 << 20

	reencodeSide    = 2048
	reencodeQuality = 85
)

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

type gcs struct {
	service *storage.Service
	name    string
}

func (b gcs) get(ctx context.Context, name string) (object, error) {
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

func (b gcs) put(ctx context.Context, name, mimeType string, content []byte) error {
	_, err := b.service.Objects.Insert(b.name, &storage.Object{Name: name, ContentType: mimeType}).
		Media(bytes.NewReader(content), googleapi.ContentType(mimeType), googleapi.ChunkSize(0)).
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func (b gcs) exists(ctx context.Context, name string) (bool, error) {
	_, err := b.service.Objects.Get(b.name, name).Fields("name").Context(ctx).Do()
	if notFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", name, err)
	}
	return true, nil
}

func (b gcs) remove(ctx context.Context, name string) error {
	err := b.service.Objects.Delete(b.name, name).Context(ctx).Do()
	if notFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete %s: %w", name, err)
	}
	return nil
}

type Bucket struct {
	objects objects
}

func Open(name string) (*Bucket, error) {
	service, err := storage.NewService(context.Background(),
		option.WithScopes(storage.DevstorageReadWriteScope))
	if err != nil {
		return nil, fmt.Errorf("storage client: %w", err)
	}
	return &Bucket{objects: gcs{service: service, name: name}}, nil
}

func NewMemoryBucket() *Bucket {
	return &Bucket{objects: &memory{objects: map[string]object{}}}
}

func (b *Bucket) Get(ctx context.Context, name string) ([]byte, string, error) {
	o, err := b.objects.get(ctx, name)
	if err != nil {
		return nil, "", err
	}
	return o.data, o.mimeType, nil
}

func (b *Bucket) Put(ctx context.Context, name, mimeType string, content []byte) error {
	return b.objects.put(ctx, name, mimeType, content)
}

func (b *Bucket) PutMedia(ctx context.Context, name, mimeType string, content []byte) error {
	if !strings.HasPrefix(mimeType, "image/") {
		return b.objects.put(ctx, name, mimeType, content)
	}
	thumb, err := Thumbnail(content)
	if err != nil {
		return fmt.Errorf("thumbnail %s: %w", name, err)
	}
	if err := b.objects.put(ctx, thumbName(name), thumbMime, thumb); err != nil {
		return err
	}
	return b.objects.put(ctx, name, mimeType, content)
}

func (b *Bucket) Exists(ctx context.Context, name string) (bool, error) {
	return b.objects.exists(ctx, name)
}

func (b *Bucket) Remove(ctx context.Context, name string) error {
	return b.objects.remove(ctx, name)
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
	thumb      []byte
}

type Store struct {
	bucket  *Bucket
	mu      sync.RWMutex
	entries map[string]*entry
	named   map[string]bool
	data    *lru.Cache[string, []byte]
}

func New(bucket *Bucket) *Store {
	return &Store{bucket: bucket, entries: map[string]*entry{}, data: lru.New[string, []byte](dataBudget, func(b []byte) int { return len(b) })}
}

func Register(mux *http.ServeMux, s *Store, folders ...string) {
	for _, folder := range folders {
		mux.HandleFunc("GET /"+folder+"/{name}", s.serve)
	}
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

func (s *Store) Bytes(name string) ([]byte, string, bool) {
	key := trimExt(name)
	e, ok := s.held(key)
	if !ok {
		return nil, "", false
	}
	data, err := s.dataOf(context.Background(), key, e)
	if err != nil {
		slog.Error("blob store: read", "name", e.name, "error", err)
		return nil, "", false
	}
	return data, e.mimeType, true
}

func (s *Store) dataOf(ctx context.Context, key string, e *entry) ([]byte, error) {
	if data, ok := s.data.Get(key); ok {
		return data, nil
	}
	o, err := s.bucket.objects.get(ctx, e.name)
	if err != nil {
		return nil, err
	}
	s.data.Put(key, o.data)
	return o.data, nil
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
	e, data, err := s.download(ctx, name)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	s.data.Put(key, data)
	s.mu.Lock()
	s.entries[key] = e
	if s.named != nil {
		s.named[key] = true
	}
	held := len(s.entries)
	s.mu.Unlock()
	slog.Info("blob store: fetched", "name", name, "bytes", len(data)+len(e.thumb), "held", held, "took", time.Since(start).Round(time.Millisecond))
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

func (s *Store) download(ctx context.Context, name string) (*entry, []byte, error) {
	o, err := s.bucket.objects.get(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	e := &entry{name: name, generation: o.generation, mimeType: o.mimeType}
	if !strings.HasPrefix(o.mimeType, "image/") {
		return e, o.data, nil
	}
	thumb, err := s.bucket.objects.get(ctx, thumbName(name))
	if errors.Is(err, ErrNotFound) {
		return nil, nil, fmt.Errorf("no thumbnail stored for %s", name)
	}
	if err != nil {
		return nil, nil, err
	}
	e.thumb = thumb.data
	return e, o.data, nil
}

func (s *Store) Put(folder, name, mimeType string, content []byte) error {
	full := folder + "/" + name
	if _, ok := s.keep(trimExt(full)); ok {
		return nil
	}
	if err := s.bucket.PutMedia(context.Background(), full, mimeType, content); err != nil {
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
	data, err := s.dataOf(r.Context(), key, e)
	if err != nil {
		slog.ErrorContext(r.Context(), "blob store: read", "name", e.name, "error", err)
		http.Error(w, "could not read the object", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", e.mimeType)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

func Check(src []byte) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxPixels/cfg.Height {
		return fmt.Errorf("image declares %dx%d pixels, over the limit of %d", cfg.Width, cfg.Height, maxPixels)
	}
	return nil
}

func Decode(src []byte) (image.Image, error) {
	if err := Check(src); err != nil {
		return nil, err
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

func Reencode(src []byte) ([]byte, error) {
	img, err := Decode(src)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	if long := max(b.Dx(), b.Dy()); long > reencodeSide {
		scaled := image.NewRGBA(image.Rect(0, 0, b.Dx()*reencodeSide/long, b.Dy()*reencodeSide/long))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, b, draw.Over, nil)
		img = scaled
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, reorient(img, orientation(src)), &jpeg.Options{Quality: reencodeQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Crop(src []byte, r image.Rectangle) ([]byte, error) {
	img, err := Decode(src)
	if err != nil {
		return nil, err
	}
	if !r.In(img.Bounds()) || r.Empty() {
		return nil, fmt.Errorf("crop %v is not inside the picture's %v", r, img.Bounds())
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: reencodeQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
