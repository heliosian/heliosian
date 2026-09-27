package sample

import (
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

func Calendar(t *testing.T, dir *data.Dir, queue *store.Queue, directory *who.Model) *when.Cache {
	t.Helper()
	cache, err := when.NewCache(dir, dir, func() when.Roster { return when.RosterOf(directory) }, nil, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}
