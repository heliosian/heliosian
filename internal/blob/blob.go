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
	Bucket        = "heliosian-media"
	sweepInterval = 5 * time.Minute
	maxIdle       = 15 * time.Minute
	thumbWidth    = 480
	thumbSuffix   = "-thumb"
	thumbExt      = ".jpg"
	thumbMime     = "image/jpeg"
	thumbVersion  = "1"
	fetchWorkers  = 32
	maxPixels     = 40_000_000
)

var folders = []string{"photos", "pronunciation", "classroom-images", "grade-images", "link-images", "activity-images", "category-images"}

// named folders hold objects replaced in place under a fixed name with no
// extension; every other folder is content addressed, so its URLs never change
// meaning.
var named = map[string]bool{"classroom-images": true, "grade-images": true}

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
}

func newBucket() (bucket, error) {
	service, err := storage.NewService(context.Background(),
		option.WithScopes(storage.DevstorageReadWriteScope))
	if err != nil {
		return bucket{}, fmt.Errorf("storage client: %w", err)
	}
	return bucket{service: service}, nil
}

func (b bucket) get(ctx context.Context, name string) (object, error) {
	resp, err := b.service.Objects.Get(Bucket, name).Context(ctx).Download()
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
	_, err := b.service.Objects.Insert(Bucket, &storage.Object{Name: name, ContentType: mimeType}).
		Media(bytes.NewReader(content), googleapi.ContentType(mimeType)).
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func (b bucket) exists(ctx context.Context, name string) (bool, error) {
	_, err := b.service.Objects.Get(Bucket, name).Fields("name").Context(ctx).Do()
	if notFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", name, err)
	}
	return true, nil
}

func (b bucket) remove(ctx context.Context, name string) error {
	err := b.service.Objects.Delete(Bucket, name).Context(ctx).Do()
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
// something keeps asking, and leaves once nothing has for a while. With a
// cache directory, every content-addressed object fetched is kept on disk
// too, and read from there ahead of the bucket on the next start.
type Store struct {
	objects  objects
	cacheDir string
	mu       sync.RWMutex
	entries  map[string]*entry
}

// New is a store over the bucket, keeping a copy of what it fetches under
// cacheDir when one is given; "" caches nothing.
func New(cacheDir string) (*Store, error) {
	b, err := newBucket()
	if err != nil {
		return nil, err
	}
	s := &Store{objects: b, cacheDir: cacheDir, entries: map[string]*entry{}}
	if cacheDir != "" {
		slog.Info("blob store: caching fetched objects on disk", "dir", cacheDir)
	}
	go s.sweepLoop()
	return s, nil
}

func NewMemory() *Store {
	s := &Store{objects: &memory{objects: map[string]object{}}, entries: map[string]*entry{}}
	go s.sweepLoop()
	return s
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

// cacheable is whether an object may be read from and written to the disk
// cache: only content-addressed folders, whose bytes never change under a
// name. The named folders are replaced in place, so those always go to the
// bucket.
func (s *Store) cacheable(name string) bool {
	folder, _, _ := strings.Cut(name, "/")
	return s.cacheDir != "" && !named[folder]
}

func (s *Store) cachePath(name string) string {
	return filepath.Join(s.cacheDir, filepath.FromSlash(name))
}

// cached is an object from the disk cache: its bytes, its thumbnail when it
// is an image, and its generation and type from the meta file beside it.
// An object with no meta file is not cached; anything else wrong with the
// files is an error, since every write lands whole or not at all.
func (s *Store) cached(name string) (*entry, error) {
	if !s.cacheable(name) {
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
	e := &entry{name: name, generation: generation, mimeType: mimeType, data: data, used: time.Now()}
	if strings.HasPrefix(mimeType, "image/") {
		if e.thumb, err = os.ReadFile(s.cachePath(thumbName(name))); err != nil {
			return nil, fmt.Errorf("read the cache of %s: %w", name, err)
		}
	}
	return e, nil
}

// cache writes an entry to the disk cache, the meta file last, so a reader
// that finds the meta file finds the whole object.
func (s *Store) cache(e *entry) error {
	if !s.cacheable(e.name) {
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

// writeFile lands a file whole: written to a temp file of its own beside its
// name, then renamed over it. Two writers of the same object - a prefetch
// asks for a name once per row that carries it - each land the same bytes.
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
// person's own avatar - the same pair of needs RegisterTeam covers.
func RegisterHome(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /link-images/{name}", s.serve)
	mux.HandleFunc("GET /photos/{name}", s.serve)
}

// RegisterTeam serves what the volunteer portal shows: the activity and role
// images, content addressed, and the directory's photos of the volunteers.
func RegisterTeam(mux *http.ServeMux, s *Store) {
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

// RegisterCalendar serves what Helios When shows: the directory's photo
// of the viewer, in the toolbar, and the category images, content addressed.
func RegisterCalendar(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
	mux.HandleFunc("GET /category-images/{name}", s.serve)
}

// RegisterLoop serves what Helios Loop shows: the directory's photos of
// the viewer, the managers and the members.
func RegisterLoop(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
}

// RegisterAsk serves what Helios Ask shows: the directory's photo of the
// viewer, in the toolbar.
func RegisterAsk(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /photos/{name}", s.serve)
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

// Get is one object's bytes, handed straight back rather than held: for what a
// caller parses into a model of its own and has no use for twice
// (`docs/ask/artifacts.md`). It reads through the disk cache like every other
// fetch, so a tool or a dev server run over thousands of them pays for the
// download once.
func (s *Store) Get(name string) ([]byte, error) {
	e, err := s.fetch(context.Background(), name)
	if err != nil {
		return nil, err
	}
	return e.data, nil
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
	if errors.Is(err, ErrNotFound) {
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

// fetch is an object and its thumbnail from the disk cache, else downloaded
// and cached.
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
	e := &entry{name: name, generation: o.generation, mimeType: o.mimeType, data: o.data, used: time.Now()}
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

// Uploader fills the bucket for the tools that load media in bulk, checking each
// name on its own rather than holding anything in memory.
type Uploader struct {
	objects objects
}

func NewUploader() (*Uploader, error) {
	b, err := newBucket()
	if err != nil {
		return nil, err
	}
	return &Uploader{objects: b}, nil
}

// Has reports whether the bucket holds an object, so a tool can resolve the names the
// sheet records without downloading anything.
func (u *Uploader) Has(name string) (bool, error) {
	return u.objects.exists(context.Background(), name)
}

// Remove deletes an object a tool has just replaced under another name, so a
// re-import leaves nothing behind. An object already gone is not an error.
func (u *Uploader) Remove(name string) error {
	return u.objects.remove(context.Background(), name)
}

// Put writes a content-addressed object and its thumbnail, and reports whether it had
// to. The name already being present means the same bytes by construction, but the
// thumbnail is checked separately: a primary written without one is an object the
// serving store refuses to load, and skipping on the primary alone leaves it that way.
func (u *Uploader) Put(folder, name, mimeType string, content []byte) (bool, error) {
	ctx := context.Background()
	full := folder + "/" + name
	var thumb []byte
	if strings.HasPrefix(mimeType, "image/") {
		hasThumb, err := u.objects.exists(ctx, thumbName(full))
		if err != nil {
			return false, err
		}
		if !hasThumb {
			if thumb, err = Thumbnail(content); err != nil {
				return false, fmt.Errorf("thumbnail %s: %w", full, err)
			}
		}
	}
	wrote := false
	present, err := u.objects.exists(ctx, full)
	if err != nil {
		return false, err
	}
	if !present {
		if err := u.objects.put(ctx, full, mimeType, content); err != nil {
			return false, err
		}
		wrote = true
	}
	if thumb == nil {
		return wrote, nil
	}
	if err := u.objects.put(ctx, thumbName(full), thumbMime, thumb); err != nil {
		return false, err
	}
	return true, nil
}

// writeWithThumbnail makes the thumbnail before it writes anything, so a picture
// the decoder refuses - one declaring more pixels than Decode allows - is never
// stored, and so never reaches the readers that decode it again.
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

// Put writes a content-addressed object and its thumbnail and takes it into memory. A
// name already held is byte-identical by construction, so the write is skipped.
func (s *Store) Put(folder, name, mimeType string, content []byte) error {
	full := folder + "/" + name
	if _, ok := s.touch(trimExt(full)); ok {
		return nil
	}
	if err := writeWithThumbnail(context.Background(), s.objects, full, mimeType, content); err != nil {
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
	if err := writeWithThumbnail(context.Background(), s.objects, full, mimeType, content); err != nil {
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

// Decode reads an image, refusing one whose header declares more pixels than a
// decoder should be asked for. A decoder allocates the whole frame from the
// header before it reads a pixel, so a few hundred bytes can ask for gigabytes.
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
