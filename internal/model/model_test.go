package model

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
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

func sampleDeps(key []byte) Deps {
	return Deps{IDKey: key, Static: testkit.None, Parties: testkit.All, Activities: testkit.All, Home: testkit.All}
}

func sampleStore(t *testing.T, dir *data.Dir, queue *store.Queue, deps Deps) *Store {
	t.Helper()
	s, err := NewStore(dir, dir, queue, deps)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const outsider = "outsider@example.org"

func outsideSuperAdmin(t *testing.T, dir *data.Dir) {
	t.Helper()
	if err := dir.Update(ConfigApp, superAdminsTab, map[string]string{configEmailColumn: jordan}, map[string]string{configEmailColumn: outsider}); err != nil {
		t.Fatal(err)
	}
}

func sampleDirectory(t *testing.T, dir *data.Dir, queue *store.Queue) *Store {
	t.Helper()
	deps := sampleDeps(sampleKey)
	deps.Photos = testkit.All
	return sampleStore(t, dir, queue, deps)
}

func typedRegistry(s *Store, queue *store.Queue, types ...[]api.Type[*Model]) *api.Registry[*Model] {
	reg := api.New(api.Config[*Model]{
		Actor:  func(r *http.Request, m *Model) access.Actor { return m.Directory.Actor(r, m.Held) },
		Held:   s.Held,
		Now:    func() time.Time { return now() },
		Queue:  queue,
		Staged: s.In,
		Scope:  (*Model).at,
	})
	for _, t := range slices.Concat(types...) {
		reg.Add(t)
	}
	queue.OnSwap(func() { reg.Publish(s.Model()) })
	return reg
}

func loadDirectory(t *testing.T, dir *data.Dir, photos, static blob.Checker, key []byte) *Directory {
	t.Helper()
	deps := sampleDeps(key)
	deps.Photos = photos
	deps.Static = static
	return sampleStore(t, dir, store.NewQueue(), deps).Model().Directory
}
