package who

import (
	"log/slog"
	"net/http"
	"strings"

	"heliosian/internal/data"
)

const maxTagLength = 40

type tagger struct {
	cache  *Cache
	writer data.Writer
	queue  *Queue
}

func RegisterTags(mux *http.ServeMux, cache *Cache, writer data.Writer, queue *Queue) {
	t := tagger{cache: cache, writer: writer, queue: queue}
	mux.HandleFunc("POST /api/directory/tag", t.set)
	mux.HandleFunc("POST /api/directory/tag-all", t.setAll)
}

func (t tagger) setAll(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	owner := effectiveEmail(t.cache, r)
	tag := strings.TrimSpace(r.FormValue("tag"))
	if tag == "" || len(tag) > maxTagLength {
		http.Error(w, "bad tag name", http.StatusBadRequest)
		return
	}
	people := []string{}
	for _, raw := range r.Form["person"] {
		person := strings.ToLower(strings.TrimSpace(raw))
		if t.cache.Model().Person(person) == nil {
			http.Error(w, "no such person", http.StatusBadRequest)
			return
		}
		if !t.cache.tagged(owner, tag, person) {
			people = append(people, person)
		}
	}
	if len(people) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	applied := make(chan struct{})
	t.queue.Add(func() {
		rows := [][]string{}
		for _, person := range people {
			t.cache.applyTag(owner, tag, person, true)
			rows = append(rows, []string{owner, tag, person})
		}
		close(applied)
		if err := t.writer.AppendAll(appName, tagsTable, rows); err != nil {
			slog.ErrorContext(r.Context(), "tag write", "owner", owner, "tag", tag, "people", len(rows), "error", err)
		}
	})
	<-applied
	slog.InfoContext(r.Context(), "tag: saved list", "owner", owner, "tag", tag, "people", len(people))
	w.WriteHeader(http.StatusNoContent)
}

func (t tagger) set(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	owner := effectiveEmail(t.cache, r)
	person := strings.ToLower(strings.TrimSpace(r.FormValue("person")))
	tag := strings.TrimSpace(r.FormValue("tag"))
	on := r.FormValue("on") == "1"
	if tag == "" || len(tag) > maxTagLength {
		http.Error(w, "bad tag name", http.StatusBadRequest)
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
