package static

import (
	"context"
	"os"
	"path/filepath"
)

type Files struct {
	Root string
}

func (s Files) Has(key string) (bool, error) {
	info, err := os.Stat(filepath.Join(s.Root, filepath.FromSlash(key)))
	return err == nil && info.Mode().IsRegular(), nil
}

func (Files) Prefetch(context.Context, []string) error { return nil }

func Bundled(roots []string, key string) bool {
	for _, root := range roots {
		if found, _ := (Files{Root: root}).Has(key); found {
			return true
		}
	}
	return false
}
