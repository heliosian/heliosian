package blob

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

type overlay struct {
	base objects
	top  *memory
	mu   sync.Mutex
	gone map[string]bool
}

func Overlay(base *Bucket) *Bucket {
	// GCS generations are microsecond timestamps, so the copy's start above them.
	top := &memory{objects: map[string]object{}, generation: time.Now().UnixMicro()}
	return &Bucket{objects: &overlay{base: base.objects, top: top, gone: map[string]bool{}}}
}

func (o *overlay) removed(name string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.gone[name]
}

func (o *overlay) get(ctx context.Context, name string) (object, error) {
	if o.removed(name) {
		return object{}, ErrNotFound
	}
	got, err := o.top.get(ctx, name)
	if !errors.Is(err, ErrNotFound) {
		return got, err
	}
	return o.base.get(ctx, name)
}

func (o *overlay) put(ctx context.Context, name, mimeType string, content []byte, generation int64) (int64, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	now, err := o.statLocked(ctx, name)
	if err != nil {
		return 0, err
	}
	if now != generation {
		return 0, fmt.Errorf("write %s at generation %d: %w", name, generation, ErrChanged)
	}
	delete(o.gone, name)
	o.top.mu.Lock()
	defer o.top.mu.Unlock()
	return o.top.set(name, mimeType, content), nil
}

func (o *overlay) stat(ctx context.Context, name string) (int64, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.statLocked(ctx, name)
}

func (o *overlay) statLocked(ctx context.Context, name string) (int64, error) {
	if o.gone[name] {
		return 0, nil
	}
	if generation, _ := o.top.stat(ctx, name); generation != 0 {
		return generation, nil
	}
	return o.base.stat(ctx, name)
}

func (o *overlay) remove(ctx context.Context, name string, generation int64) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	now, err := o.statLocked(ctx, name)
	if err != nil {
		return err
	}
	if now == 0 {
		return fmt.Errorf("delete %s: %w", name, ErrNotFound)
	}
	if now != generation {
		return fmt.Errorf("delete %s at generation %d: %w", name, generation, ErrChanged)
	}
	o.gone[name] = true
	o.top.mu.Lock()
	defer o.top.mu.Unlock()
	delete(o.top.objects, name)
	return nil
}

func (o *overlay) list(ctx context.Context, prefix string) ([]listed, error) {
	base, err := o.base.list(ctx, prefix)
	if err != nil {
		return nil, err
	}
	top, _ := o.top.list(ctx, prefix)
	newest := map[string]listed{}
	for _, l := range append(base, top...) {
		if !o.removed(l.Name) && strings.HasPrefix(l.Name, prefix) {
			newest[l.Name] = l
		}
	}
	out := slices.Collect(maps.Values(newest))
	slices.SortFunc(out, func(a, b listed) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (o *overlay) usage(ctx context.Context) (map[string]Usage, error) {
	out, err := o.base.usage(ctx)
	if err != nil {
		return nil, err
	}
	top, _ := o.top.usage(ctx)
	for folder, u := range top {
		sum := out[folder]
		sum.Objects += u.Objects
		sum.Bytes += u.Bytes
		out[folder] = sum
	}
	return out, nil
}
