package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"heliosian/internal/static"
)

type Checker interface {
	Has(key string) (bool, error)
	Prefetch(ctx context.Context, names []string) error
}

var imageFolders = map[string]string{
	"home":      "link-images",
	"team":      "activity-images",
	"celebrate": "party-images",
	"when":      "category-images",
	"wiki":      "wiki-images",
}

var folders = append([]string{"photos", "pronunciation"}, slices.Collect(maps.Values(imageFolders))...)

var ImageExtensions = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/gif":  "gif",
	"image/webp": "webp",
}

func Name(content []byte, ext string) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]) + "." + ext
}

func ImageFolder(app string) (string, bool) {
	folder, ok := imageFolders[app]
	return folder, ok
}

type Images struct {
	store   *Store
	folders []string
	dirs    []string
}

func NewImages(s *Store, apps ...string) Images {
	im := Images{store: s}
	for _, app := range apps {
		folder, ok := imageFolders[app]
		if !ok {
			panic(fmt.Sprintf("no image folder for %s", app))
		}
		im.folders = append(im.folders, folder)
		im.dirs = append(im.dirs, path.Join("web", app), path.Join("web/public", app))
	}
	return im
}

func (im Images) Folder() string {
	return im.folders[0]
}

func (im Images) stored(key string) bool {
	folder, _, found := strings.Cut(key, "/")
	return found && slices.Contains(im.folders, folder)
}

func (im Images) Has(key string) (bool, error) {
	if im.stored(key) {
		return im.store.Has(key)
	}
	return static.Bundled(im.dirs, key), nil
}

func (im Images) Prefetch(ctx context.Context, names []string) error {
	keys := []string{}
	for _, name := range names {
		if im.stored(name) {
			keys = append(keys, name)
		}
	}
	return im.store.Prefetch(ctx, keys)
}

func (im Images) Read(key string) []byte {
	if key == "" {
		return nil
	}
	if im.stored(key) {
		data, _, _ := im.store.Bytes(key)
		return data
	}
	for _, dir := range im.dirs {
		if data, err := os.ReadFile(path.Join(dir, key)); err == nil {
			return data
		}
	}
	return nil
}
