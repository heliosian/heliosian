package blob

import (
	"context"
	"errors"
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

func (o *overlay) put(ctx context.Context, name, mimeType string, content []byte) error {
	o.mu.Lock()
	delete(o.gone, name)
	o.mu.Unlock()
	return o.top.put(ctx, name, mimeType, content)
}

func (o *overlay) exists(ctx context.Context, name string) (bool, error) {
	if o.removed(name) {
		return false, nil
	}
	if held, _ := o.top.exists(ctx, name); held {
		return true, nil
	}
	return o.base.exists(ctx, name)
}

func (o *overlay) remove(ctx context.Context, name string) error {
	o.mu.Lock()
	o.gone[name] = true
	o.mu.Unlock()
	return o.top.remove(ctx, name)
}

func (o *overlay) list(ctx context.Context, prefix string) ([]string, error) {
	base, err := o.base.list(ctx, prefix)
	if err != nil {
		return nil, err
	}
	top, _ := o.top.list(ctx, prefix)
	out := []string{}
	for _, name := range append(base, top...) {
		if !o.removed(name) && strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
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
