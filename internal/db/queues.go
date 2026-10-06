package db

import (
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/serve"
	"heliosian/internal/store"
)

type queueCount struct {
	Name    string `json:"name"`
	About   string `json:"about"`
	Waiting int    `json:"waiting"`
	Done    int    `json:"done,omitempty"`
	Total   int    `json:"total,omitempty"`
	Query   string `json:"query,omitempty"`
}

type queueReport struct {
	Queues      []queueCount `json:"queues"`
	LastRefresh time.Time    `json:"lastRefresh"`
}

func RegisterQueues(mux *http.ServeMux, s *Store, queue *store.Queue, importKey []byte, now func() time.Time) {
	mux.HandleFunc("GET /api/queues", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, _, ok := caller(w, r, m, importKey, now())
		if !ok {
			return
		}
		if !m.superAdmin(env.Viewer) {
			http.Error(w, "super admins only", http.StatusForbidden)
			return
		}
		status := queue.Status()
		report := queueReport{Queues: m.queueCounts(), LastRefresh: status.LastRefresh}
		report.Queues = append(report.Queues, queueCount{Name: "pending writes", About: "commits and refreshes waiting on the write queue, plus work held open", Waiting: status.Waiting + status.Held})
		serve.Write(w, r, http.StatusOK, report)
	})
}

func (m *Model) superAdmin(viewer string) bool {
	if viewer == "" {
		return false
	}
	for _, g := range m.Table("GROUP").All() {
		if g["slug"] != "super-admins" {
			continue
		}
		return slices.ContainsFunc(m.effectiveRows(g["id"]), func(row store.Row) bool { return row["person"] == viewer })
	}
	return false
}

func quoted(values []string) string {
	out := []string{}
	for _, v := range values {
		out = append(out, strconv.Quote(v))
	}
	return strings.Join(out, " ")
}

func (m *Model) queueCounts() []queueCount {
	out := []queueCount{}
	contents := m.Table("CONTENT")
	mimes := slices.Collect(maps.Keys(extractors))
	for _, c := range contents.All() {
		if extractable(baseType(c["mime"])) && !slices.Contains(mimes, c["mime"]) {
			mimes = append(mimes, c["mime"])
		}
	}
	slices.Sort(mimes)
	extraction := queueCount{Name: "extraction", About: "documents whose content the extractor reads, not yet read", Query: `(from DOCUMENT (where (blank extracted) (in content.mime ` + quoted(mimes) + `)))`}
	fetching := queueCount{Name: "fetching", About: "images and links an email shows, not yet fetched", Query: `(from DOCUMENT (where (in relation "image" "linked") (blank content) (blank fetch)))`}
	classifying := queueCount{Name: "classifying", About: "mail sent to no group yet, waiting for Claude to say whom it was written to", Query: `(from DOCUMENT @d (where (= kind "mail") (blank parent) (not (exists DOCUMENT_GROUP (= document @d) (= relation "sent_to")))))`}
	links := m.Table("DOCUMENT_GROUP")
	for _, d := range m.Table("DOCUMENT").All() {
		if c, ok := contents.Get(d["content"]); ok && extractable(baseType(c["mime"])) {
			extraction.Total++
			if d["extracted"] == "" {
				extraction.Waiting++
			}
		}
		if d["relation"] == "image" || d["relation"] == "linked" {
			fetching.Total++
			if d["content"] == "" && d["fetch"] == "" {
				fetching.Waiting++
			}
		}
		if d["kind"] == "mail" && d["parent"] == "" && !slices.ContainsFunc(links.Referencing("document", d["id"]), func(l store.Row) bool { return l["relation"] == "sent_to" }) {
			classifying.Waiting++
		}
	}
	extraction.Done, fetching.Done = extraction.Total-extraction.Waiting, fetching.Total-fetching.Waiting
	out = append(out, extraction, fetching, classifying)

	search := queueCount{Name: "search indexing", About: "groups, people and extracts whose search entry Claude and Vertex have not made yet", Query: `(from SEARCH (where (not made)))`}
	searchTable, _ := Lookup("SEARCH")
	for _, row := range m.generated(searchTable).rows {
		search.Total++
		if !strings.EqualFold(row["made"], "yes") {
			search.Waiting++
		}
	}
	search.Done = search.Total - search.Waiting
	out = append(out, search)

	photos := queueCount{Name: "photos", About: "photos whose re-encode, crop and thumbnail are not made for the current original and box", Query: `(from PHOTO (where (not ready)))`}
	for _, row := range m.Table("PHOTO").All() {
		photos.Total++
		if !strings.EqualFold(row["ready"], "yes") {
			photos.Waiting++
		}
	}
	photos.Done = photos.Total - photos.Waiting
	out = append(out, photos)

	refs := []string{}
	for _, t := range Tables {
		if t.Generated || t.Name == ChangesTable {
			continue
		}
		for _, c := range t.Columns {
			if c.Kind == Ref && (c.Target == "" || c.Target == "CONTENT") {
				refs = append(refs, `(not (exists `+t.Name+` (= `+c.Name+` @c)))`)
			}
		}
	}
	out = append(out, queueCount{Name: "content sweep", About: "stored content no row refers to, which the sweeper deletes with its object", Waiting: len(m.unreferencedContent()), Query: `(from CONTENT @c (where ` + strings.Join(refs, " ") + `))`})
	return out
}
