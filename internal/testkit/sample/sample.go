package sample

import (
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/model"
	"heliosian/internal/store"
	"heliosian/internal/when"
)

func Calendar(t *testing.T, dir *data.Dir, queue *store.Queue, directory *model.Directory) *when.Cache {
	t.Helper()
	cache, err := when.NewCache(dir, dir, func() when.Roster { return when.RosterOf(directory) }, nil, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}
