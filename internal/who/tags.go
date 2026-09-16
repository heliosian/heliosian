package who

import (
	"log/slog"
	"net/http"
	"strings"

	"heliosian/internal/data"
	"heliosian/internal/mail"
)

const maxTagLength = 40

type tagger struct {
	cache  *Cache
	writer data.Writer
	queue  *Queue
	// mailer sends word of a tag shared; nil sends nothing.
	mailer mail.Sender
}

func RegisterTags(mux *http.ServeMux, cache *Cache, writer data.Writer, queue *Queue, mailer mail.Sender) {
	t := tagger{cache: cache, writer: writer, queue: queue, mailer: mailer}
	mux.HandleFunc("POST /api/directory/tag", t.set)
	mux.HandleFunc("POST /api/directory/tag-delete", t.drop)
	mux.HandleFunc("POST /api/directory/tag-rename", t.rename)
	mux.HandleFunc("POST /api/directory/tag-copy", t.copy)
	mux.HandleFunc("POST /api/directory/tag-share", t.share)
	mux.HandleFunc("POST /api/directory/tag-leave", t.leave)
}

// tagOwnerOf is whose tag a request means: the caller's own unless it names
// an owner, which is a manager working on a tag shared with them - allowed
// only while the owner still lists them. The empty string refuses.
func (t tagger) tagOwnerOf(r *http.Request, tag string) string {
	caller := effectiveEmail(t.cache, r)
	owner := strings.ToLower(strings.TrimSpace(r.FormValue("owner")))
	if owner == "" || owner == caller {
		return caller
	}
	if !t.cache.canManage(caller, owner, tag) {
		return ""
	}
	return owner
}

// rename gives one of the caller's tags a new name, its managers following
// - the owner's alone, as delete is. A name the caller already uses for
// another tag is refused rather than the two silently merged.
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
	hadManagers := len(t.cache.TagManagers(owner)[from]) > 0
	applied := make(chan struct{})
	var people int
	t.queue.Add(func() {
		people = t.cache.renameTag(owner, from, to)
		close(applied)
		if people == 0 {
			return
		}
		t.cache.changed()
		if err := t.writer.Set(appName, tagsTable, map[string]string{tagOwner: owner, tagName: from}, map[string]string{tagName: to}); err != nil {
			slog.ErrorContext(r.Context(), "tag rename", "owner", owner, "from", from, "to", to, "error", err)
		}
		// Set appends a row when nothing matches, so the managers' tab is
		// only touched when there are managers to follow.
		if !hadManagers {
			return
		}
		if err := t.writer.Set(appName, managersTable, map[string]string{tagOwner: owner, tagName: from}, map[string]string{tagName: to}); err != nil {
			slog.ErrorContext(r.Context(), "tag rename managers", "owner", owner, "from", from, "to", to, "error", err)
		}
	})
	<-applied
	slog.InfoContext(r.Context(), "tag: renamed", "owner", owner, "from", from, "to", to, "people", people)
	w.WriteHeader(http.StatusNoContent)
}

// copy makes the caller a new tag of their own with the same people as one
// of theirs or - with an owner named - one shared with them, a starting
// point to change as they like; the copy theirs alone, without the
// original's managers. A name already in use is refused, as rename refuses
// it.
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
	if len(t.cache.Tags(fromOwner)[from]) == 0 {
		http.Error(w, "no such tag", http.StatusBadRequest)
		return
	}
	if len(t.cache.Tags(owner)[to]) > 0 {
		http.Error(w, "you already have a tag called "+to, http.StatusConflict)
		return
	}
	applied := make(chan struct{})
	var rows [][]string
	t.queue.Add(func() {
		rows = t.cache.copyTag(fromOwner, from, owner, to)
		close(applied)
		if len(rows) == 0 {
			return
		}
		t.cache.changed()
		if err := t.writer.AppendAll(appName, tagsTable, rows); err != nil {
			slog.ErrorContext(r.Context(), "tag copy", "owner", owner, "from", from, "to", to, "error", err)
		}
	})
	<-applied
	slog.InfoContext(r.Context(), "tag: copied", "owner", owner, "fromOwner", fromOwner, "from", from, "to", to, "people", len(rows))
	w.WriteHeader(http.StatusNoContent)
}

// share lets the owner of a tag add or remove a manager of it - anyone in
// the directory, a student included, other than themselves, who then sees
// the tag under "Shared Tags" and can tag and untag through it as the owner
// can. Only a tag
// with people in it can be shared: a tag is nothing but its rows, so an
// empty one isn't there to share.
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
	applied := make(chan struct{})
	t.queue.Add(func() {
		t.cache.applyManager(owner, tag, manager, on)
		close(applied)
		if err := t.flushManager(owner, tag, manager, on); err != nil {
			slog.ErrorContext(r.Context(), "tag share write", "owner", owner, "tag", tag, "manager", manager, "error", err)
		}
	})
	<-applied
	slog.InfoContext(r.Context(), "tag: shared", "owner", owner, "on", on, "tag", tag, "manager", manager)
	if on {
		t.notifyShared(r, owner, tag, manager)
	}
	w.WriteHeader(http.StatusNoContent)
}

// leave takes the caller off a tag shared with them - their own choice, as
// against the owner's through share - the tag itself untouched.
func (t tagger) leave(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	manager := effectiveEmail(t.cache, r)
	owner := strings.ToLower(strings.TrimSpace(r.FormValue("owner")))
	tag := strings.TrimSpace(r.FormValue("tag"))
	if tag == "" || len(tag) > maxTagLength || owner == "" || owner == manager {
		http.Error(w, "bad tag", http.StatusBadRequest)
		return
	}
	applied := make(chan struct{})
	t.queue.Add(func() {
		t.cache.applyManager(owner, tag, manager, false)
		close(applied)
		if err := t.flushManager(owner, tag, manager, false); err != nil {
			slog.ErrorContext(r.Context(), "tag leave write", "owner", owner, "tag", tag, "manager", manager, "error", err)
		}
	})
	<-applied
	slog.InfoContext(r.Context(), "tag: left", "owner", owner, "tag", tag, "manager", manager)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) flushManager(owner, tag, manager string, on bool) error {
	if on {
		return t.writer.Append(appName, managersTable, []string{owner, tag, manager})
	}
	return t.writer.Delete(appName, managersTable, map[string]string{
		tagOwner:     owner,
		tagName:      tag,
		managerEmail: manager,
	})
}

// drop removes one of the caller's tags outright - every person in it at
// once, and whoever managed it - the way the list page's Delete tag button
// asks, rather than untagging them one by one through set. The owner's
// alone: a manager leaves instead.
func (t tagger) drop(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	owner := effectiveEmail(t.cache, r)
	tag := strings.TrimSpace(r.FormValue("tag"))
	if tag == "" || len(tag) > maxTagLength {
		http.Error(w, "bad tag name", http.StatusBadRequest)
		return
	}
	applied := make(chan struct{})
	var people int
	t.queue.Add(func() {
		people = t.cache.dropTag(owner, tag)
		close(applied)
		if people == 0 {
			return
		}
		t.cache.changed()
		if err := t.writer.Delete(appName, tagsTable, map[string]string{tagOwner: owner, tagName: tag}); err != nil {
			slog.ErrorContext(r.Context(), "tag delete", "owner", owner, "tag", tag, "error", err)
		}
		if err := t.writer.Delete(appName, managersTable, map[string]string{tagOwner: owner, tagName: tag}); err != nil {
			slog.ErrorContext(r.Context(), "tag delete managers", "owner", owner, "tag", tag, "error", err)
		}
	})
	<-applied
	slog.InfoContext(r.Context(), "tag: deleted", "owner", owner, "tag", tag, "people", people)
	w.WriteHeader(http.StatusNoContent)
}

// set tags or untags one person, on the caller's own tag or - with an owner
// named - one shared with them.
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
	if t.cache.tagged(owner, tag, person) == on {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	applied := make(chan struct{})
	t.queue.Add(func() {
		t.cache.applyTag(owner, tag, person, on)
		close(applied)
		t.cache.changed()
		if err := t.flush(owner, tag, person, on); err != nil {
			slog.ErrorContext(r.Context(), "tag write", "owner", owner, "tag", tag, "person", person, "error", err)
		}
	})
	<-applied
	slog.InfoContext(r.Context(), "tag: changed", "owner", owner, "on", on, "tag", tag, "person", person)
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) flush(owner, tag, person string, on bool) error {
	if on {
		return t.writer.Append(appName, tagsTable, []string{owner, tag, person})
	}
	return t.writer.Delete(appName, tagsTable, map[string]string{
		tagOwner:  owner,
		tagName:   tag,
		tagPerson: person,
	})
}
