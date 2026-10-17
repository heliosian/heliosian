package model

import (
	"testing"

	"heliosian/internal/blob"
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

var sampleKey = []byte("sample")

func sampleDeps(key []byte) Deps {
	return Deps{IDKey: key, Static: testkit.None}
}

func sampleStore(t *testing.T, dir *data.Dir, queue *store.Queue, deps Deps) *Store {
	t.Helper()
	s, err := NewStore(dir, dir, queue, deps)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func sampleDirectory(t *testing.T, dir *data.Dir, queue *store.Queue) *Store {
	t.Helper()
	deps := sampleDeps(sampleKey)
	deps.Photos = testkit.All
	return sampleStore(t, dir, queue, deps)
}

func loadDirectory(t *testing.T, dir *data.Dir, photos, static blob.Checker, key []byte) *Directory {
	t.Helper()
	deps := sampleDeps(key)
	deps.Photos = photos
	deps.Static = static
	return sampleStore(t, dir, store.NewQueue(), deps).Model().Directory
}
