package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"io/fs"
	"net/http"
	"os"
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
	MediaBucket = "heliosian-media"
	thumbWidth  = 480
	maxPixels   = 40_000_000

	reencodeSide    = 2048
	reencodeQuality = 85
)

var (
	ErrNotFound = errors.New("no such object")
	ErrChanged  = errors.New("object is not at the expected generation")
)

type object struct {
	mimeType   string
	generation int64
	data       []byte
	written    time.Time
}

type objects interface {
	get(ctx context.Context, name string) (object, error)
	put(ctx context.Context, name, mimeType string, content []byte, generation int64) (int64, error)
	stat(ctx context.Context, name string) (int64, error)
	remove(ctx context.Context, name string, generation int64) error
	list(ctx context.Context, prefix string) ([]listed, error)
	usage(ctx context.Context) (map[string]Usage, error)
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

func (b gcs) put(ctx context.Context, name, mimeType string, content []byte, generation int64) (int64, error) {
	o, err := b.service.Objects.Insert(b.name, &storage.Object{Name: name, ContentType: mimeType}).
		Media(bytes.NewReader(content), googleapi.ContentType(mimeType), googleapi.ChunkSize(0)).
		IfGenerationMatch(generation).Fields("generation").Context(ctx).Do()
	if preconditionFailed(err) {
		return 0, fmt.Errorf("write %s at generation %d: %w", name, generation, ErrChanged)
	}
	if err != nil {
		return 0, fmt.Errorf("write %s: %w", name, err)
	}
	return o.Generation, nil
}

func (b gcs) stat(ctx context.Context, name string) (int64, error) {
	o, err := b.service.Objects.Get(b.name, name).Fields("generation").Context(ctx).Do()
	if notFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", name, err)
	}
	return o.Generation, nil
}

func (b gcs) remove(ctx context.Context, name string, generation int64) error {
	err := b.service.Objects.Delete(b.name, name).IfGenerationMatch(generation).Context(ctx).Do()
	if notFound(err) {
		return fmt.Errorf("delete %s: %w", name, ErrNotFound)
	}
	if preconditionFailed(err) {
		return fmt.Errorf("delete %s at generation %d: %w", name, generation, ErrChanged)
	}
	if err != nil {
		return fmt.Errorf("delete %s: %w", name, err)
	}
	return nil
}

func (b gcs) list(ctx context.Context, prefix string) ([]listed, error) {
	out := []listed{}
	err := b.service.Objects.List(b.name).Prefix(prefix).Fields("nextPageToken", "items/name", "items/generation", "items/updated").Pages(ctx, func(page *storage.Objects) error {
		for _, o := range page.Items {
			written, err := time.Parse(time.RFC3339, o.Updated)
			if err != nil {
				return fmt.Errorf("list %s: %s was written at %q: %w", prefix, o.Name, o.Updated, err)
			}
			out = append(out, listed{Version: Version{Name: o.Name, Generation: o.Generation}, written: written})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", prefix, err)
	}
	return out, nil
}

func (b gcs) usage(ctx context.Context) (map[string]Usage, error) {
	out := map[string]Usage{}
	err := b.service.Objects.List(b.name).Fields("nextPageToken", "items/name", "items/size").Pages(ctx, func(page *storage.Objects) error {
		for _, o := range page.Items {
			addUsage(out, o.Name, int64(o.Size))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("measure %s: %w", b.name, err)
	}
	return out, nil
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

func (b *Bucket) FillFrom(root string) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		_, err = b.Put(context.Background(), filepath.ToSlash(rel), http.DetectContentType(content), content, 0)
		return err
	})
}

func (b *Bucket) Get(ctx context.Context, name string) ([]byte, string, error) {
	o, err := b.objects.get(ctx, name)
	if err != nil {
		return nil, "", err
	}
	return o.data, o.mimeType, nil
}

func (b *Bucket) GetVersion(ctx context.Context, name string) ([]byte, int64, error) {
	o, err := b.objects.get(ctx, name)
	if err != nil {
		return nil, 0, err
	}
	return o.data, o.generation, nil
}

func (b *Bucket) Put(ctx context.Context, name, mimeType string, content []byte, generation int64) (int64, error) {
	return b.objects.put(ctx, name, mimeType, content, generation)
}

func (b *Bucket) Generation(ctx context.Context, name string) (int64, error) {
	return b.objects.stat(ctx, name)
}

func (b *Bucket) Exists(ctx context.Context, name string) (bool, error) {
	generation, err := b.objects.stat(ctx, name)
	return generation != 0, err
}

func (b *Bucket) Remove(ctx context.Context, name string, generation int64) error {
	return b.objects.remove(ctx, name, generation)
}

func (b *Bucket) List(ctx context.Context, prefix string) ([]Version, error) {
	return b.Older(ctx, prefix, time.Time{})
}

func (b *Bucket) Older(ctx context.Context, prefix string, before time.Time) ([]Version, error) {
	all, err := b.objects.list(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := []Version{}
	for _, o := range all {
		if before.IsZero() || o.written.Before(before) {
			out = append(out, o.Version)
		}
	}
	return out, nil
}

type Version struct {
	Name       string
	Generation int64
}

type listed struct {
	Version
	written time.Time
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

func (m *memory) put(_ context.Context, name, mimeType string, content []byte, generation int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.objects[name].generation != generation {
		return 0, fmt.Errorf("write %s at generation %d: %w", name, generation, ErrChanged)
	}
	return m.set(name, mimeType, content), nil
}

func (m *memory) set(name, mimeType string, content []byte) int64 {
	m.generation++
	m.objects[name] = object{mimeType: mimeType, generation: m.generation, data: content, written: time.Now()}
	return m.generation
}

func (m *memory) stat(_ context.Context, name string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.objects[name].generation, nil
}

func (m *memory) remove(_ context.Context, name string, generation int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objects[name]
	if !ok {
		return fmt.Errorf("delete %s: %w", name, ErrNotFound)
	}
	if o.generation != generation {
		return fmt.Errorf("delete %s at generation %d: %w", name, generation, ErrChanged)
	}
	delete(m.objects, name)
	return nil
}

func (m *memory) list(_ context.Context, prefix string) ([]listed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []listed{}
	for name, o := range m.objects {
		if strings.HasPrefix(name, prefix) {
			out = append(out, listed{Version: Version{Name: name, Generation: o.generation}, written: o.written})
		}
	}
	slices.SortFunc(out, func(a, b listed) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (m *memory) usage(_ context.Context) (map[string]Usage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]Usage{}
	for name, o := range m.objects {
		addUsage(out, name, int64(len(o.data)))
	}
	return out, nil
}

type Usage struct {
	Objects int   `json:"objects"`
	Bytes   int64 `json:"bytes"`
}

func addUsage(out map[string]Usage, name string, size int64) {
	folder, _, _ := strings.Cut(name, "/")
	u := out[folder]
	u.Objects++
	u.Bytes += size
	out[folder] = u
}

func (b *Bucket) Usage(ctx context.Context) (map[string]Usage, error) {
	return b.objects.usage(ctx)
}

func notFound(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound
}

func preconditionFailed(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusPreconditionFailed
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
