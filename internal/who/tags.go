package who

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"heliosian/internal/store"
)

const maxTagLength = 40

type SharedTag struct {
	Owner     string   `json:"owner"`
	OwnerName string   `json:"ownerName"`
	Name      string   `json:"name"`
	People    []string `json:"people"`
	Managers  []string `json:"managers"`
}

func (m *Model) Tags(owner string) map[string][]string {
	tags := map[string][]string{}
	for _, row := range m.tags {
		if !strings.EqualFold(row[tagOwner], owner) {
			continue
		}
		person := strings.ToLower(row[tagPerson])
		if m.Person(person) == nil {
			continue
		}
		tags[row[tagName]] = append(tags[row[tagName]], person)
	}
	for _, people := range tags {
		sort.Strings(people)
	}
	return tags
}

func (m *Model) TagManagers(owner string) map[string][]string {
	out := map[string][]string{}
	for _, row := range m.managers {
		if !strings.EqualFold(row[tagOwner], owner) {
			continue
		}
		manager := strings.ToLower(row[managerEmail])
		if m.Person(manager) == nil {
			continue
		}
		out[row[tagName]] = append(out[row[tagName]], manager)
	}
	for _, managers := range out {
		sort.Strings(managers)
	}
	return out
}

func (m *Model) SharedTags(email string) []SharedTag {
	out := []SharedTag{}
	for _, row := range m.managers {
		if !strings.EqualFold(row[managerEmail], email) {
			continue
		}
		owner := strings.ToLower(row[tagOwner])
		if m.Person(owner) == nil {
			continue
		}
		tag := row[tagName]
		shared := SharedTag{Owner: owner, OwnerName: m.DisplayName(owner), Name: tag, People: []string{}, Managers: []string{}}
		for _, t := range m.tags {
			if strings.EqualFold(t[tagOwner], owner) && t[tagName] == tag {
				person := strings.ToLower(t[tagPerson])
				if m.Person(person) != nil {
					shared.People = append(shared.People, person)
				}
			}
		}
		if len(shared.People) == 0 {
			continue
		}
		for _, other := range m.managers {
			if strings.EqualFold(other[tagOwner], owner) && other[tagName] == tag {
				manager := strings.ToLower(other[managerEmail])
				if m.Person(manager) != nil {
					shared.Managers = append(shared.Managers, manager)
				}
			}
		}
		sort.Strings(shared.People)
		sort.Strings(shared.Managers)
		out = append(out, shared)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Owner < out[j].Owner
	})
	return out
}

func (m *Model) canManage(email, owner, tag string) bool {
	if strings.EqualFold(email, owner) {
		return true
	}
	for _, row := range m.managers {
		if strings.EqualFold(row[tagOwner], owner) && row[tagName] == tag && strings.EqualFold(row[managerEmail], email) {
			return true
		}
	}
	return false
}

func (m *Model) tagged(owner, tag, person string) bool {
	for _, row := range m.tags {
		if strings.EqualFold(row[tagOwner], owner) && row[tagName] == tag && strings.EqualFold(row[tagPerson], person) {
			return true
		}
	}
	return false
}

type tagger struct {
	cache *Cache
}

func RegisterTags(mux *http.ServeMux, cache *Cache) {
	t := tagger{cache: cache}
	mux.HandleFunc("POST /api/directory/tag", t.set)
	mux.HandleFunc("POST /api/directory/tag-delete", t.drop)
	mux.HandleFunc("POST /api/directory/tag-rename", t.rename)
	mux.HandleFunc("POST /api/directory/tag-copy", t.copy)
	mux.HandleFunc("POST /api/directory/tag-share", t.share)
	mux.HandleFunc("POST /api/directory/tag-leave", t.leave)
}

func (t tagger) tagOwnerOf(r *http.Request, tag string) string {
	caller := effectiveEmail(t.cache, r)
	owner := strings.ToLower(strings.TrimSpace(r.FormValue("owner")))
	if owner == "" || owner == caller {
		return caller
	}
	if !t.cache.Model().canManage(caller, owner, tag) {
		return ""
	}
	return owner
}

func (t tagger) rename(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	owner := effectiveEmail(t.cache, r)
	from := strings.TrimSpace(r.FormValue("tag"))
	to := strings.TrimSpace(r.FormValue("name"))
	if from == "" || len(from) > maxTagLength || to == "" || len(to) > maxTagLength {
		http.Error(w, "bad tag name", http.StatusBadRequest)
		return
	}
	if to == from {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	own := t.cache.Tags(owner)
	if len(own[from]) == 0 {
		http.Error(w, "no such tag", http.StatusBadRequest)
		return
	}
	if len(own[to]) > 0 {
		http.Error(w, "you already have a tag called "+to, http.StatusConflict)
		return
	}
	named := store.Row{tagOwner: owner, tagName: from}
	if !t.cache.commit(w, r, owner,
		store.Update(tagsTable, named, store.Row{tagName: to}),
		store.Update(managersTable, named, store.Row{tagName: to}),
	) {
		return
	}
	slog.InfoContext(r.Context(), "tag: renamed", "owner", owner, "from", from, "to", to, "people", len(own[from]))
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) copy(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	owner := effectiveEmail(t.cache, r)
	from := strings.TrimSpace(r.FormValue("tag"))
	to := strings.TrimSpace(r.FormValue("name"))
	if from == "" || len(from) > maxTagLength || to == "" || len(to) > maxTagLength {
		http.Error(w, "bad tag name", http.StatusBadRequest)
		return
	}
	fromOwner := t.tagOwnerOf(r, from)
	if fromOwner == "" {
		http.Error(w, "not your tag to copy", http.StatusForbidden)
		return
	}
	if fromOwner == owner && to == from {
		http.Error(w, "bad tag name", http.StatusBadRequest)
		return
	}
	people := t.cache.Tags(fromOwner)[from]
	if len(people) == 0 {
		http.Error(w, "no such tag", http.StatusBadRequest)
		return
	}
	if len(t.cache.Tags(owner)[to]) > 0 {
		http.Error(w, "you already have a tag called "+to, http.StatusConflict)
		return
	}
	ops := []store.Op{}
	for _, person := range people {
		ops = append(ops, store.Insert(tagsTable, store.Row{tagOwner: owner, tagName: to, tagPerson: person}))
	}
	if !t.cache.commit(w, r, owner, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "tag: copied", "owner", owner, "fromOwner", fromOwner, "from", from, "to", to, "people", len(people))
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) share(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	owner := effectiveEmail(t.cache, r)
	tag := strings.TrimSpace(r.FormValue("tag"))
	manager := strings.ToLower(strings.TrimSpace(r.FormValue("manager")))
	on := r.FormValue("on") == "1"
	if tag == "" || len(tag) > maxTagLength {
		http.Error(w, "bad tag name", http.StatusBadRequest)
		return
	}
	if t.cache.Model().Person(manager) == nil || manager == owner {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	if on && len(t.cache.Tags(owner)[tag]) == 0 {
		http.Error(w, "no such tag", http.StatusBadRequest)
		return
	}
	if !t.cache.commit(w, r, owner, managerOp(owner, tag, manager, on)) {
		return
	}
	slog.InfoContext(r.Context(), "tag: shared", "owner", owner, "on", on, "tag", tag, "manager", manager)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) leave(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	manager := effectiveEmail(t.cache, r)
	owner := strings.ToLower(strings.TrimSpace(r.FormValue("owner")))
	tag := strings.TrimSpace(r.FormValue("tag"))
	if tag == "" || len(tag) > maxTagLength || owner == "" || owner == manager {
		http.Error(w, "bad tag", http.StatusBadRequest)
		return
	}
	if !t.cache.commit(w, r, manager, managerOp(owner, tag, manager, false)) {
		return
	}
	slog.InfoContext(r.Context(), "tag: left", "owner", owner, "tag", tag, "manager", manager)
	w.WriteHeader(http.StatusNoContent)
}

func managerOp(owner, tag, manager string, on bool) store.Op {
	row := store.Row{tagOwner: owner, tagName: tag, managerEmail: manager}
	if on {
		return store.Set(managersTable, row, store.Row{})
	}
	return store.Delete(managersTable, row)
}

func (t tagger) drop(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	owner := effectiveEmail(t.cache, r)
	tag := strings.TrimSpace(r.FormValue("tag"))
	if tag == "" || len(tag) > maxTagLength {
		http.Error(w, "bad tag name", http.StatusBadRequest)
		return
	}
	people := len(t.cache.Tags(owner)[tag])
	named := store.Row{tagOwner: owner, tagName: tag}
	if !t.cache.commit(w, r, owner, store.Delete(tagsTable, named), store.Delete(managersTable, named)) {
		return
	}
	slog.InfoContext(r.Context(), "tag: deleted", "owner", owner, "tag", tag, "people", people)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) set(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	person := strings.ToLower(strings.TrimSpace(r.FormValue("person")))
	tag := strings.TrimSpace(r.FormValue("tag"))
	on := r.FormValue("on") == "1"
	if tag == "" || len(tag) > maxTagLength {
		http.Error(w, "bad tag name", http.StatusBadRequest)
		return
	}
	owner := t.tagOwnerOf(r, tag)
	if owner == "" {
		http.Error(w, "not your tag to manage", http.StatusForbidden)
		return
	}
	if t.cache.Model().Person(person) == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	if t.cache.Model().tagged(owner, tag, person) == on {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	row := store.Row{tagOwner: owner, tagName: tag, tagPerson: person}
	op := store.Delete(tagsTable, row)
	if on {
		op = store.Insert(tagsTable, row)
	}
	if !t.cache.commit(w, r, effectiveEmail(t.cache, r), op) {
		return
	}
	slog.InfoContext(r.Context(), "tag: changed", "owner", owner, "on", on, "tag", tag, "person", person)
	w.WriteHeader(http.StatusNoContent)
}
