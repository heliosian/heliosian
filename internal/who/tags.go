package who

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/id"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const maxTagLength = 40

type Tag struct {
	ID        string   `json:"id"`
	Owner     string   `json:"owner"`
	OwnerName string   `json:"ownerName"`
	Name      string   `json:"name"`
	People    []string `json:"people"`
	Managers  []string `json:"managers"`
}

type tagRecord struct {
	id, owner, name  string
	people, managers []string
}

func (l *loader) readTags() error {
	tags := map[string]*tagRecord{}
	named := map[string]bool{}
	for _, row := range l.tagListRows {
		key, ok := id.Parse(row[tagID])
		if !ok {
			return fmt.Errorf("tag list row %v has no valid tag id", row)
		}
		if tags[key] != nil {
			return fmt.Errorf("tag list has two rows for tag %s", key)
		}
		owner, name := strings.ToLower(strings.TrimSpace(row[tagOwner])), row[tagName]
		if owner == "" || !validTagName(name) || name != strings.TrimSpace(name) {
			return fmt.Errorf("tag list row %s needs an owner and a tag name of at most %d characters with no spaces around it", key, maxTagLength)
		}
		if named[owner+"\n"+strings.ToLower(name)] {
			return fmt.Errorf("%s has two tags called %s", owner, name)
		}
		named[owner+"\n"+strings.ToLower(name)] = true
		tags[key] = &tagRecord{id: key, owner: owner, name: name}
	}
	people, err := tagEmails(tags, tagsTable, tagPerson, l.tagRows)
	if err != nil {
		return err
	}
	managers, err := tagEmails(tags, managersTable, managerEmail, l.managerRows)
	if err != nil {
		return err
	}
	for key, t := range tags {
		t.people, t.managers = people[key], managers[key]
		if len(t.people) == 0 {
			return fmt.Errorf("tag %s, %s's %s, has nobody on it: delete its tag list row", key, t.owner, t.name)
		}
	}
	l.model.tags = tags
	return nil
}

func tagEmails(tags map[string]*tagRecord, tab, column string, rows []store.Row) (map[string][]string, error) {
	out := map[string][]string{}
	for _, row := range rows {
		key, _ := id.Parse(row[tagID])
		if tags[key] == nil {
			return nil, fmt.Errorf("%s row %v names no tag in the tag list", tab, row)
		}
		email := strings.ToLower(strings.TrimSpace(row[column]))
		if email == "" {
			return nil, fmt.Errorf("%s row %v has no %s", tab, row, column)
		}
		if slices.Contains(out[key], email) {
			return nil, fmt.Errorf("%s has two rows for %s on tag %s", tab, email, key)
		}
		out[key] = append(out[key], email)
	}
	return out, nil
}

func (m *Model) taken(key string) bool {
	_, ok := m.tags[key]
	return ok
}

func (m *Model) TagIDs() []string {
	out := []string{}
	for key := range m.tags {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func (m *Model) tagByKey(raw string) *tagRecord {
	key, ok := id.Parse(raw)
	if !ok {
		return nil
	}
	return m.tags[key]
}

func (m *Model) listed(emails []string) []string {
	out := []string{}
	for _, email := range emails {
		if m.Person(email) != nil {
			out = append(out, email)
		}
	}
	slices.Sort(out)
	return out
}

func (m *Model) Tag(key string) (Tag, bool) {
	t := m.tagByKey(key)
	if t == nil {
		return Tag{}, false
	}
	people := m.listed(t.people)
	if len(people) == 0 {
		return Tag{}, false
	}
	return Tag{ID: t.id, Owner: t.owner, OwnerName: m.DisplayName(t.owner), Name: t.name, People: people, Managers: m.listed(t.managers)}, true
}

func (m *Model) sortedTags(keep func(t *tagRecord) bool) []Tag {
	out := []Tag{}
	for _, t := range m.tags {
		if !keep(t) {
			continue
		}
		if tag, ok := m.Tag(t.id); ok {
			out = append(out, tag)
		}
	}
	slices.SortFunc(out, func(a, b Tag) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		if c := strings.Compare(a.Owner, b.Owner); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

func (m *Model) Tags(owner string) []Tag {
	owner = strings.ToLower(owner)
	return m.sortedTags(func(t *tagRecord) bool { return t.owner == owner })
}

func (m *Model) SharedTags(email string) []Tag {
	email = strings.ToLower(email)
	return m.sortedTags(func(t *tagRecord) bool { return slices.Contains(t.managers, email) && m.Person(t.owner) != nil })
}

func (m *Model) ownTagNamed(owner, name string) *tagRecord {
	for _, t := range m.tags {
		if t.owner == owner && strings.EqualFold(t.name, name) {
			return t
		}
	}
	return nil
}

type tagger struct {
	cache *Cache
}

type savedTag struct {
	ID string `json:"id"`
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
	key := strings.TrimSpace(r.FormValue("tag"))
	to := strings.TrimSpace(r.FormValue("name"))
	actor := requestActor(t.cache, r)
	ops, from, err := t.cache.Model().renameTag(actor, key, to)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := t.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "tag: renamed", "owner", actor.Email, "tag", key, "from", from, "to", to)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) copy(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	key := strings.TrimSpace(r.FormValue("tag"))
	to := strings.TrimSpace(r.FormValue("name"))
	actor := requestActor(t.cache, r)
	ops, made, people, err := t.cache.Model().copyTag(actor, key, to)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := t.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "tag: copied", "owner", actor.Email, "from", key, "to", made, "name", to, "people", people)
	serve.Write(w, r, http.StatusOK, savedTag{ID: made})
}

func (t tagger) share(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	key := strings.TrimSpace(r.FormValue("tag"))
	manager := formEmail(r, "manager")
	on := r.FormValue("on") == "1"
	actor := requestActor(t.cache, r)
	ops, err := t.cache.Model().shareTag(actor, key, manager, on)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := t.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "tag: shared", "owner", actor.Email, "on", on, "tag", key, "manager", manager)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) leave(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	key := strings.TrimSpace(r.FormValue("tag"))
	actor := requestActor(t.cache, r)
	ops, err := t.cache.Model().leaveTag(actor, key)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := t.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "tag: left", "tag", key, "manager", actor.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) drop(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	key := strings.TrimSpace(r.FormValue("tag"))
	actor := requestActor(t.cache, r)
	ops, people, err := t.cache.Model().dropTag(actor, key)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := t.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "tag: deleted", "owner", actor.Email, "tag", key, "people", people)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) set(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	person := formEmail(r, "person")
	on := r.FormValue("on") == "1"
	actor := requestActor(t.cache, r)
	ops, key, err := t.cache.Model().setTag(actor, strings.TrimSpace(r.FormValue("tag")), strings.TrimSpace(r.FormValue("name")), person, on)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := t.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "tag: changed", "on", on, "tag", key, "person", person, "by", actor.Email)
	serve.Write(w, r, http.StatusOK, savedTag{ID: key})
}
