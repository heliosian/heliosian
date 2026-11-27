package db

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const maxNoteWords = 2000

type extNote struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Added   string `json:"added"`
	By      string `json:"by"`
	Host    bool   `json:"host,omitempty"`
	ReplyTo string `json:"replyTo,omitempty"`
}

func (m *Model) notesFor(g store.Row, person string) []extNote {
	household := m.householdOf(person)
	all := m.Table("COMMENT").Referencing("group", g["id"])
	asked := map[string]bool{}
	out := []extNote{}
	for _, n := range all {
		if n["reply_to"] == "" && slices.Contains(household, n["person"]) {
			asked[n["id"]] = true
			out = append(out, extNote{ID: n["id"], Text: n["text"], Added: n["added"], By: m.nameOf(n["person"])})
		}
	}
	for _, n := range all {
		if asked[n["reply_to"]] {
			out = append(out, extNote{ID: n["id"], Text: n["text"], Added: n["added"], By: m.nameOf(n["person"]), Host: true, ReplyTo: n["reply_to"]})
		}
	}
	slices.SortFunc(out, func(a, b extNote) int { return strings.Compare(a.Added, b.Added) })
	return out
}

func (iv *Invitations) extComment(w http.ResponseWriter, r *http.Request) {
	c, err := iv.extCopy(r)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&body); err != nil {
		serve.Error(w, r, access.Invalid("send {\"text\": \"...\"}: %v", err))
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" || len(text) > maxNoteWords {
		serve.Error(w, r, access.Invalid("a note is some words, at most %d characters", maxNoteWords))
		return
	}
	if c.group["kind"] != "event" {
		serve.Error(w, r, access.Forbidden("notes to the hosts are for events"))
		return
	}
	m := iv.s.Model()
	edit := Edit{Insert: "COMMENT", Row: map[string]any{"group": c.group["id"], "person": c.person, "text": text, "added": stamp(time.Now())}}
	if _, err := iv.write(r.Context(), access.Actor{Email: m.addressOf(c.person)}, []Edit{edit}); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "invitations: note to the hosts from outside", "person", c.person, "group", c.group["id"])
	w.WriteHeader(http.StatusNoContent)
}
