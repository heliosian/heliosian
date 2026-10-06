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
	Name    string       `json:"name"`
	About   string       `json:"about,omitempty"`
	Pending int          `json:"pending"`
	Done    int          `json:"done"`
	Total   int          `json:"total"`
	Query   string       `json:"query,omitempty"`
	Parts   []queueCount `json:"parts,omitempty"`
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
		report.Queues = append(report.Queues, queueCount{Name: "pending writes", About: "commits and refreshes waiting on the write queue, plus work held open", Pending: status.Pending + status.Held})
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

type tally struct {
	parts map[string]*queueCount
}

func (t *tally) add(c *queueCount, part string, pending bool) {
	c.Total++
	if pending {
		c.Pending++
	}
	if t.parts == nil {
		t.parts = map[string]*queueCount{}
	}
	p := t.parts[part]
	if p == nil {
		p = &queueCount{Name: part}
		t.parts[part] = p
	}
	p.Total++
	if pending {
		p.Pending++
	}
}

func (t *tally) finish(c *queueCount, query func(part string) string) {
	for _, name := range slices.Sorted(maps.Keys(t.parts)) {
		p := t.parts[name]
		p.Done = p.Total - p.Pending
		p.Query = query(name)
		c.Parts = append(c.Parts, *p)
	}
	c.Done = c.Total - c.Pending
}

func (m *Model) queueCounts() []queueCount {
	contents := m.Table("CONTENT")
	mimesOf := map[string][]string{}
	for kind := range extractors {
		mimesOf[kind] = []string{kind}
	}
	for _, c := range contents.All() {
		kind := baseType(c["mime"])
		if extractable(kind) && !slices.Contains(mimesOf[kind], c["mime"]) {
			mimesOf[kind] = append(mimesOf[kind], c["mime"])
		}
	}
	allMimes := []string{}
	for _, kind := range slices.Sorted(maps.Keys(mimesOf)) {
		allMimes = append(allMimes, mimesOf[kind]...)
	}
	extractionQuery := func(mimes []string) string {
		return `(from DOCUMENT (where (blank extracted) (in content.mime ` + quoted(mimes) + `)))`
	}
	fetchingQuery := func(relations ...string) string {
		return `(from DOCUMENT (where (in relation ` + quoted(relations) + `) (blank content) (blank fetch)))`
	}
	extraction := queueCount{Name: "extraction", About: "documents whose content the extractor reads, not yet read", Query: extractionQuery(allMimes)}
	fetching := queueCount{Name: "fetching", About: "images and links an email shows, not yet fetched", Query: fetchingQuery("image", "linked")}
	signInQuery := func(relations ...string) string {
		return `(from DOCUMENT (where (in relation ` + quoted(relations) + `) (blank content) (= fetch "sign_in")))`
	}
	signIn := queueCount{Name: "fetching signed in", About: "images and links that need someone signed in to Google: go run ./tools/fetchsignin", Query: signInQuery("image", "linked")}
	signInBy := map[string]int{}
	classifying := queueCount{Name: "classifying", About: "mail sent to no group yet, waiting for Claude to say whom it was written to", Query: `(from DOCUMENT @d (where (= kind "mail") (blank parent) (not (exists DOCUMENT_GROUP (= document @d) (= relation "sent_to")))))`}
	byType, byRelation := &tally{}, &tally{}
	links := m.Table("DOCUMENT_GROUP")
	for _, d := range m.Table("DOCUMENT").All() {
		if c, ok := contents.Get(d["content"]); ok && extractable(baseType(c["mime"])) {
			byType.add(&extraction, baseType(c["mime"]), d["extracted"] == "")
		}
		if d["relation"] == "image" || d["relation"] == "linked" {
			byRelation.add(&fetching, d["relation"], d["content"] == "" && d["fetch"] == "")
			if d["content"] == "" && d["fetch"] == "sign_in" {
				signIn.Pending++
				signInBy[d["relation"]]++
			}
		}
		if d["kind"] == "mail" && d["parent"] == "" && !slices.ContainsFunc(links.Referencing("document", d["id"]), func(l store.Row) bool { return l["relation"] == "sent_to" }) {
			classifying.Pending++
		}
	}
	byType.finish(&extraction, func(kind string) string { return extractionQuery(mimesOf[kind]) })
	byRelation.finish(&fetching, func(relation string) string { return fetchingQuery(relation) })
	for _, relation := range []string{"image", "linked"} {
		signIn.Parts = append(signIn.Parts, queueCount{Name: relation, Pending: signInBy[relation], Query: signInQuery(relation)})
	}
	out := []queueCount{extraction, fetching, signIn, classifying}

	search := queueCount{Name: "search indexing", About: "groups, people and extracts whose search entry Claude and Vertex have not made yet", Query: `(from SEARCH (where (not made)))`}
	byTable := &tally{}
	searchTable, _ := Lookup("SEARCH")
	for _, row := range m.generated(searchTable).rows {
		table, _ := TableOf(row["target"])
		byTable.add(&search, table, !strings.EqualFold(row["made"], "yes"))
	}
	byTable.finish(&search, func(table string) string {
		return `(from SEARCH @s (where (not made) (exists ` + table + ` (= id @s.target))))`
	})
	out = append(out, search)

	photos := queueCount{Name: "photos", About: "photos whose re-encode, crop and thumbnail are not made for the current original and box", Query: `(from PHOTO (where (not ready)))`}
	for _, row := range m.Table("PHOTO").All() {
		photos.Total++
		if !strings.EqualFold(row["ready"], "yes") {
			photos.Pending++
		}
	}
	photos.Done = photos.Total - photos.Pending
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
	sweepQuery := func(also string) string {
		return `(from CONTENT @c (where ` + also + strings.Join(refs, " ") + `))`
	}
	sweep := queueCount{Name: "content sweep", About: "stored content no row refers to, which the sweeper deletes with its object", Query: sweepQuery("")}
	byMime := map[string]int{}
	for _, c := range m.unreferencedContent() {
		sweep.Pending++
		byMime[c["mime"]]++
	}
	for _, mimeType := range slices.Sorted(maps.Keys(byMime)) {
		sweep.Parts = append(sweep.Parts, queueCount{Name: baseType(mimeType), Pending: byMime[mimeType], Query: sweepQuery(`(= mime ` + strconv.Quote(mimeType) + `) `)})
	}
	return append(out, sweep)
}
