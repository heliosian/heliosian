// Package blob serves media from cloud storage: every object a sheet names, fetched by that name and held in memory with its stored thumbnail.
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

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	storage "google.golang.org/api/storage/v1"
)

const (
	Bucket        = "heliosian-media"
	sweepInterval = 5 * time.Minute
	maxIdle       = 15 * time.Minute
	thumbWidth    = 480
	thumbSuffix   = "-thumb"
	thumbExt      = ".jpg"
	thumbMime     = "image/jpeg"
	thumbVersion  = "1"
	fetchWorkers  = 32
)

var folders = []string{"photos", "pronunciation", "classroom-images", "grade-images", "link-images", "activity-images"}

// named folders hold objects replaced in place under a fixed name with no
// extension; every other folder is content addressed, so its URLs never change
// meaning.
var named = map[string]bool{"classroom-images": true, "grade-images": true}

var errNotFound = errors.New("no such object")

// Media reports whether a request path is one of the media routes served here.
func Media(path string) bool {
	folder, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	return slices.Contains(folders, folder)
}

// Recorded names carry an extension and entries are keyed without one, so an object
// and its thumbnail share a key.
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
	used       time.Time
}

// Store holds what the sheets name. Nothing here ever lists the bucket: an
// object enters memory when a loader asks for it by name, stays while
// something keeps asking, and leaves once nothing has for a while.
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
	go s.sweepLoop()
	return s, nil
}

func Register(mux *http.ServeMux, s *Store) {
	// One route per kind, and the object name is a content hash, so nothing about
	// where a blob is stored or who it belongs to reaches the client.
	mux.HandleFunc("GET /photos/{name}", s.serve)
	mux.HandleFunc("GET /pronunciation/{name}", s.serve)
	// Classroom and grade images are named for what they depict, not their bytes, since
	// an admin replaces one in place rather than adding a new one alongside it.
	mux.HandleFunc("GET /classroom-images/{name}", s.serve)
	mux.HandleFunc("GET /grade-images/{name}", s.serve)
}

// RegisterHome serves what the link portal shows: its link and category
// images, content addressed, and the directory's photos, for the signed-in
// person's own avatar - the same pair of needs RegisterEvents covers.
func RegisterHome(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /link-images/{name}", s.serve)
	mux.HandleFunc("GET /photos/{name}", s.serve)
}

// RegisterEvents serves what the volunteer portal shows: the activity and role
// images, content addressed, and the directory's photos of the volunteers.
func RegisterEvents(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
	mux.HandleFunc("GET /activity-images/{name}", s.serve)
}

// RegisterBirthday serves what the birthday team shows: the directory's photos
// of the staff and of the team.
func RegisterBirthday(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
}

// RegisterCelebrate serves what Helios Celebrate shows: the party
// images, content addressed, and the directory's photos of who is coming.
func RegisterCelebrate(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
	mux.HandleFunc("GET /party-images/{name}", s.serve)
}

func (s *Store) sweepLoop() {
	for range time.Tick(sweepInterval) {
		s.sweep(time.Now())
	}
}

// sweep drops what nothing has asked for lately. Every loader re-asks for
// every name it holds on its own refresh, so an object a sheet stopped naming
// is exactly one nobody asks for.
func (s *Store) sweep(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dropped := 0
	for key, e := range s.entries {
		if now.Sub(e.used) > maxIdle {
			delete(s.entries, key)
			dropped++
		}
	}
	if dropped > 0 {
		slog.Info("blob store: swept", "dropped", dropped, "kept", len(s.entries))
	}
}

func notFound(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound
}

func (s *Store) touch(key string) (*entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if ok {
		e.used = time.Now()
	}
	return e, ok
}

// Bytes is an object the store already holds, with its media type, for a
// caller that composes rather than serves - the share card draws an event's
// image into itself. Nothing is fetched: what the sheets name is prefetched.
func (s *Store) Bytes(name string) ([]byte, string, bool) {
	e, ok := s.touch(trimExt(name))
	if !ok {
		return nil, "", false
	}
	return e.data, e.mimeType, true
}

// Has fetches the named object into memory on first sight and reports whether
// the bucket holds it. Any trouble other than the object not existing is an
// error, so a network fault never reads as a missing photo.
func (s *Store) Has(name string) (bool, error) {
	if _, ok := s.touch(trimExt(name)); ok {
		return true, nil
	}
	start := time.Now()
	e, err := s.fetch(context.Background(), name)
	if errors.Is(err, errNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	s.entries[trimExt(name)] = e
	held := len(s.entries)
	s.mu.Unlock()
	slog.Info("blob store: fetched", "name", name, "bytes", len(e.data)+len(e.thumb), "held", held, "took", time.Since(start).Round(time.Millisecond))
	return true, nil
}

// Prefetch fetches many names at once, for a loader that is about to ask for
// each of them; a name the bucket lacks is left for Has to report.
func (s *Store) Prefetch(names []string) error {
	start := time.Now()
	pending := []string{}
	for _, name := range names {
		if _, ok := s.touch(trimExt(name)); !ok {
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
			if _, err := s.Has(name); err != nil {
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

// fetch downloads an object and its thumbnail. The download response already
// carries the generation and content type, so there is no separate stat.
func (s *Store) fetch(ctx context.Context, name string) (*entry, error) {
	resp, err := s.service.Objects.Get(Bucket, name).Context(ctx).Download()
	if notFound(err) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	generation, err := strconv.ParseInt(resp.Header.Get("x-goog-generation"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("read %s: no generation on the response: %w", name, err)
	}
	mimeType := resp.Header.Get("Content-Type")
	e := &entry{name: name, generation: generation, mimeType: mimeType, data: data, used: time.Now()}
	if !strings.HasPrefix(mimeType, "image/") {
		return e, nil
	}
	e.thumb, err = read(ctx, s.service, thumbName(name))
	if errors.Is(err, errNotFound) {
		return nil, fmt.Errorf("no thumbnail stored for %s", name)
	}
	if err != nil {
		return nil, err
	}
	return e, nil
}

// Uploader fills the bucket for the tools that load media in bulk, checking each
// name on its own rather than holding anything in memory.
type Uploader struct {
	service *storage.Service
}

func NewUploader() (*Uploader, error) {
	service, err := storage.NewService(context.Background(),
		option.WithScopes(storage.DevstorageReadWriteScope))
	if err != nil {
		return nil, fmt.Errorf("storage client: %w", err)
	}
	return &Uploader{service: service}, nil
}

func exists(ctx context.Context, service *storage.Service, name string) (bool, error) {
	_, err := service.Objects.Get(Bucket, name).Fields("name").Context(ctx).Do()
	if notFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", name, err)
	}
	return true, nil
}

// Has reports whether the bucket holds an object, so a tool can resolve the names the
// sheet records without downloading anything.
func (u *Uploader) Has(name string) (bool, error) {
	return exists(context.Background(), u.service, name)
}

// Put writes a content-addressed object and its thumbnail, and reports whether it had
// to. The name already being present means the same bytes by construction, but the
// thumbnail is checked separately: a primary written without one is an object the
// serving store refuses to load, and skipping on the primary alone leaves it that way.
func (u *Uploader) Put(folder, name, mimeType string, content []byte) (bool, error) {
	ctx := context.Background()
	full := folder + "/" + name
	wrote := false
	present, err := exists(ctx, u.service, full)
	if err != nil {
		return false, err
	}
	if !present {
		if err := write(ctx, u.service, full, mimeType, content); err != nil {
			return false, err
		}
		wrote = true
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return wrote, nil
	}
	hasThumb, err := exists(ctx, u.service, thumbName(full))
	if err != nil || hasThumb {
		return wrote, err
	}
	if err := writeThumbnail(ctx, u.service, full, content); err != nil {
		return false, err
	}
	return true, nil
}

func read(ctx context.Context, service *storage.Service, name string) ([]byte, error) {
	resp, err := service.Objects.Get(Bucket, name).Context(ctx).Download()
	if err != nil {
		if notFound(err) {
			return nil, errNotFound
		}
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return content, nil
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

func writeThumbnail(ctx context.Context, service *storage.Service, name string, content []byte) error {
	thumb, err := Thumbnail(content)
	if err != nil {
		return fmt.Errorf("thumbnail %s: %w", name, err)
	}
	return write(ctx, service, thumbName(name), thumbMime, thumb)
}

func writeWithThumbnail(ctx context.Context, service *storage.Service, name, mimeType string, content []byte) error {
	if err := write(ctx, service, name, mimeType, content); err != nil {
		return err
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return nil
	}
	return writeThumbnail(ctx, service, name, content)
}

// Put writes a content-addressed object and its thumbnail and takes it into memory. A
// name already held is byte-identical by construction, so the write is skipped.
func (s *Store) Put(folder, name, mimeType string, content []byte) error {
	full := folder + "/" + name
	if _, ok := s.touch(trimExt(full)); ok {
		return nil
	}
	if err := writeWithThumbnail(context.Background(), s.service, full, mimeType, content); err != nil {
		return err
	}
	return s.take(full)
}

// PutNamed writes an object at a fixed, human-chosen name with no extension,
// replacing whatever was there before — the opposite assumption from Put, for the
// handful of slots (a classroom's logo, a grade's tile) that are named for what they
// are rather than their bytes. Object versioning on the bucket keeps the replaced
// generation recoverable.
func (s *Store) PutNamed(folder, name, mimeType string, content []byte) error {
	full := folder + "/" + name
	if err := writeWithThumbnail(context.Background(), s.service, full, mimeType, content); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.entries, full)
	s.mu.Unlock()
	return s.take(full)
}

// take reads a just-written object back, so what is served is exactly what the
// bucket holds, generation included.
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
	e, ok := s.touch(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	folder, _, _ := strings.Cut(key, "/")
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, e.generation))
	if named[folder] {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	}
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
