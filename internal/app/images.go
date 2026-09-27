package app

import (
	"context"
	"strings"

	"heliosian/internal/blob"
	"heliosian/internal/static"
)

func prefetchUploaded(ctx context.Context, store *blob.Store, folder string, names []string) error {
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
		return h.store.Has(key)
	}
	return static.Bundled([]string{"web/home", "web/public/home"}, key), nil
}

func (h homeImages) Prefetch(ctx context.Context, names []string) error {
	return prefetchUploaded(ctx, h.store, "link-images/", names)
}

type teamImages struct {
	store *blob.Store
}

func (e teamImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "activity-images/") {
		return e.store.Has(key)
	}
	return static.Bundled([]string{"web/team", "web/public/team"}, key), nil
}

func (e teamImages) Prefetch(ctx context.Context, names []string) error {
	return prefetchUploaded(ctx, e.store, "activity-images/", names)
}

type celebrateImages struct {
	store *blob.Store
}

func (c celebrateImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "party-images/") {
		return c.store.Has(key)
	}
	return static.Bundled([]string{"web/celebrate", "web/public/celebrate"}, key), nil
}

func (c celebrateImages) Prefetch(ctx context.Context, names []string) error {
	return prefetchUploaded(ctx, c.store, "party-images/", names)
}

type calendarImages struct {
	store *blob.Store
}

func (c calendarImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "category-images/") {
		return c.store.Has(key)
	}
	return static.Bundled([]string{"web/when", "web/public/when"}, key), nil
}

func (c calendarImages) Prefetch(ctx context.Context, names []string) error {
	return prefetchUploaded(ctx, c.store, "category-images/", names)
}
