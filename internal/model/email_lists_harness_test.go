package model

import (
	"context"
	"net/http"
	"testing"
	"time"

	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

type harness struct {
	t       *testing.T
	mux     *http.ServeMux
	dir     *data.Dir
	store   *Store
	sources func() AudienceSources
	queue   *store.Queue
}

func (h *harness) lists() *EmailLists {
	return h.store.Model().EmailLists
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return loopHarness(t, nil)
}

func loopHarness(t *testing.T, team store.Tables) *harness {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	sheet, queue = dir, store.NewQueue()
	outsideSuperAdmin(t, dir)
	linkRows(t, nil, team)
	objects := blob.NewMemoryBucket()
	embedder := vertex(t)
	deps := sampleDeps(sampleKey)
	deps.Static = testkit.Files("../../web/who")
	deps.Objects = objects
	deps.Embedder = embedder
	s := sampleStore(t, dir, queue, deps)
	sources := func() AudienceSources {
		return s.Model().Audience(now())
	}
	filer := RegisterDocuments(http.NewServeMux(), s, embedder, queue, artifacts.Inbox{SigningKey: "key", Bucket: objects}, func(context.Context, []byte) error { return nil })
	h := &harness{t: t, mux: http.NewServeMux(), dir: dir, store: s, sources: sources, queue: queue}
	typedRegistry(s, queue, DirectoryResources(), EmailListResources(s, filer), MagicTagResources()).Register(h.mux)
	RegisterEmailLists(h.mux, EmailListsDeps{
		Store: s,
		About: EmailListsAbout(func() string { return "Helios Loop" }, func() string { return "Email lists drawn from the directory" }),
	})
	return h
}

func (h *harness) rows(tab string) []map[string]string {
	_, rows, err := h.dir.Table(emailListsAppName, tab)
	if err != nil {
		h.t.Fatal(err)
	}
	return rows
}

func (h *harness) waitFor(what string, ok func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("waited in vain for %s", what)
}

func (h *harness) members(name string) []string {
	return h.lists().Named(name).Members(h.sources())
}
