package model

import (
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

const (
	abena  = "abena.osei@heliosschool.org"
	asha   = "asha.chandra@heliosschool.org"
	colin  = "colin.quinn@heliosschool.org"
	dev    = "dev.chandra@heliosschool.org"
	elena  = "elena.torres@heliosschool.org"
	jordan = "jordan.whitfield@heliosschool.org"
	marco  = "marco.torres@heliosschool.org"
	noa    = "noa.adler@heliosschool.org"
	rohan  = "rohan.chandra@heliosschool.org"
)

const (
	carpool    = "dtg0000000001"
	soccerTeam = "dtg0000000002"
	bookClub   = "dtg0000000003"
	band       = "dtg0000000010"
	choir      = "dtg0000000011"
)

var testKey = []byte("test")

func sampleDirectory(t *testing.T, dir *data.Dir, queue *store.Queue) *DirectoryCache {
	t.Helper()
	cache, err := NewDirectoryCache(dir, dir, testkit.All, testkit.None, queue, testKey, func() []string { return []string{jordan} })
	if err != nil {
		t.Fatal(err)
	}
	return cache
}
