package who

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"
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

func formEmail(r *http.Request, name string) string {
	return strings.ToLower(strings.TrimSpace(r.FormValue(name)))
}

func (t tagger) rename(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	from := strings.TrimSpace(r.FormValue("tag"))
	to := strings.TrimSpace(r.FormValue("name"))
	actor := requestActor(t.cache, r)
	ops, people, err := t.cache.Model().renameTag(actor, from, to)
	if err != nil {
		refuse(w, err)
		return
	}
	if len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !t.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "tag: renamed", "owner", actor.Email, "from", from, "to", to, "people", people)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) copy(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	from := strings.TrimSpace(r.FormValue("tag"))
	to := strings.TrimSpace(r.FormValue("name"))
	actor := requestActor(t.cache, r)
	ops, fromOwner, people, err := t.cache.Model().copyTag(actor, formEmail(r, "owner"), from, to)
	if err != nil {
		refuse(w, err)
		return
	}
	if !t.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "tag: copied", "owner", actor.Email, "fromOwner", fromOwner, "from", from, "to", to, "people", people)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) share(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	tag := strings.TrimSpace(r.FormValue("tag"))
	manager := formEmail(r, "manager")
	on := r.FormValue("on") == "1"
	actor := requestActor(t.cache, r)
	ops, err := t.cache.Model().shareTag(actor, tag, manager, on)
	if err != nil {
		refuse(w, err)
		return
	}
	if !t.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "tag: shared", "owner", actor.Email, "on", on, "tag", tag, "manager", manager)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) leave(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	owner := formEmail(r, "owner")
	tag := strings.TrimSpace(r.FormValue("tag"))
	actor := requestActor(t.cache, r)
	ops, err := t.cache.Model().leaveTag(actor, owner, tag)
	if err != nil {
		refuse(w, err)
		return
	}
	if !t.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "tag: left", "owner", owner, "tag", tag, "manager", actor.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) drop(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	tag := strings.TrimSpace(r.FormValue("tag"))
	actor := requestActor(t.cache, r)
	ops, people, err := t.cache.Model().dropTag(actor, tag)
	if err != nil {
		refuse(w, err)
		return
	}
	if !t.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "tag: deleted", "owner", actor.Email, "tag", tag, "people", people)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) set(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	person := formEmail(r, "person")
	tag := strings.TrimSpace(r.FormValue("tag"))
	on := r.FormValue("on") == "1"
	actor := requestActor(t.cache, r)
	ops, owner, err := t.cache.Model().setTag(actor, formEmail(r, "owner"), tag, person, on)
	if err != nil {
		refuse(w, err)
		return
	}
	if len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !t.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "tag: changed", "owner", owner, "on", on, "tag", tag, "person", person)
	w.WriteHeader(http.StatusNoContent)
}
