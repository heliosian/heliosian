package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

type harness struct {
	t     *testing.T
	mux   *http.ServeMux
	store *Store
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
	deps := sampleDeps(sampleKey)
	deps.Static = testkit.Files("../../web/who")
	deps.Objects = blob.NewMemoryBucket()
	deps.Embedder = vertex(t)
	s := sampleStore(t, dir, queue, deps)
	h := &harness{t: t, mux: http.NewServeMux(), store: s}
	typedRegistry(s, queue, DirectoryResources(), MagicTagResources()).Register(h.mux)
	return h
}

func (h *harness) as(email, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	auth.Fixed(email, h.mux).ServeHTTP(rec, req)
	return rec
}

type listReply struct {
	Result    json.RawMessage                      `json:"result"`
	Resources map[string]map[string]map[string]any `json:"resources"`
}

func (h *harness) get(as, path string) listReply {
	h.t.Helper()
	rec := h.as(as, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		h.t.Fatalf("GET %s as %s: %d %s", path, as, rec.Code, rec.Body)
	}
	var out listReply
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		h.t.Fatalf("GET %s: %v", path, err)
	}
	return out
}

func (h *harness) want(as, method, path, body string, code int) {
	h.t.Helper()
	if rec := h.as(as, method, path, body); rec.Code != code {
		h.t.Fatalf("%s %s as %s: %d %s, want %d", method, path, as, rec.Code, rec.Body, code)
	}
}

func (r listReply) ids(t *testing.T) []string {
	t.Helper()
	out := []string{}
	if err := json.Unmarshal(r.Result, &out); err != nil {
		t.Fatalf("result %s: %v", r.Result, err)
	}
	return out
}

func (r listReply) id(t *testing.T) string {
	t.Helper()
	var out string
	if err := json.Unmarshal(r.Result, &out); err != nil {
		t.Fatalf("result %s: %v", r.Result, err)
	}
	return out
}

func strings2(v any) []string {
	out := []string{}
	for _, s := range v.([]any) {
		out = append(out, s.(string))
	}
	return out
}
