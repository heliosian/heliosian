package sample

import (
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/model"
	"heliosian/internal/store"
)

func Calendar(t *testing.T, dir *data.Dir, queue *store.Queue, directory *model.Directory) *model.CalendarCache {
	t.Helper()
	cache, err := model.NewCalendarCache(dir, dir, func() model.Roster { return directory.Roster() }, nil, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}
