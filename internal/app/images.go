package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"heliosian/internal/blob"
)

type StaticFiles struct {
	Root string
}

func (s StaticFiles) Has(key string) (bool, error) {
	info, err := os.Stat(filepath.Join(s.Root, filepath.FromSlash(key)))
	return err == nil && info.Mode().IsRegular(), nil
}

func (StaticFiles) Prefetch(context.Context, []string) error { return nil }

func Bundled(roots []string, key string) bool {
	for _, root := range roots {
		if found, _ := (StaticFiles{Root: root}).Has(key); found {
			return true
		}
	}
	return false
}

func uploaded(store *blob.Store, key string) (bool, error) {
	if store == nil {
		return false, nil
	}
	return store.Has(key)
}

func prefetchUploaded(ctx context.Context, store *blob.Store, folder string, names []string) error {
	if store == nil {
		return nil
	}
	keys := []string{}
	for _, name := range names {
		if strings.HasPrefix(name, folder) {
			keys = append(keys, name)
		}
	}
	return store.Prefetch(ctx, keys)
}

type homeImages struct {
	store *blob.Store
}

func (h homeImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "link-images/") {
		return uploaded(h.store, key)
	}
	return Bundled([]string{"web/home", "web/public/home"}, key), nil
}

func (h homeImages) Prefetch(ctx context.Context, names []string) error {
	return prefetchUploaded(ctx, h.store, "link-images/", names)
}

type teamImages struct {
	store *blob.Store
}

func (e teamImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "activity-images/") {
		return uploaded(e.store, key)
	}
	return Bundled([]string{"web/team", "web/public/team"}, key), nil
}

func (e teamImages) Prefetch(ctx context.Context, names []string) error {
	return prefetchUploaded(ctx, e.store, "activity-images/", names)
}

type celebrateImages struct {
	store *blob.Store
}

func (c celebrateImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "party-images/") {
		return uploaded(c.store, key)
	}
	return Bundled([]string{"web/celebrate", "web/public/celebrate"}, key), nil
}

func (c celebrateImages) Prefetch(ctx context.Context, names []string) error {
	return prefetchUploaded(ctx, c.store, "party-images/", names)
}

type calendarImages struct {
	store *blob.Store
}

func (c calendarImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "category-images/") {
		return uploaded(c.store, key)
	}
	return Bundled([]string{"web/when", "web/public/when"}, key), nil
}

func (c calendarImages) Prefetch(ctx context.Context, names []string) error {
	return prefetchUploaded(ctx, c.store, "category-images/", names)
}
