package db

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

func registerGuest(mux *http.ServeMux, s *Store, queue *store.Queue, pics *Pictures, tokens auth.Tokens, now func() time.Time) {
	mux.HandleFunc("POST /api/do/guest", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, actor, ok := caller(w, r, m, tokens, now())
		if !ok {
			return
		}
		var asked struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&asked); err != nil {
			serve.Error(w, r, access.Invalid("send {\"email\": \"…\", \"name\": \"…\"}: %v", err))
			return
		}
		address := strings.ToLower(strings.TrimSpace(asked.Email))
		if address == "" || mail.AddressOf(address) != address || !strings.Contains(address, "@") {
			serve.Error(w, r, access.Invalid("%q is not an address", asked.Email))
			return
		}
		name := strings.Join(strings.Fields(asked.Name), " ")
		guest := store.Row{"source": "guest"}
		if err := m.Authorize(env, Change{Table: "PERSON", New: guest}); err != nil {
			serve.Error(w, r, err)
			return
		}
		if person := m.personAt(address); person != "" {
			serve.Write(w, r, http.StatusOK, written{Result: []string{person}})
			return
		}
		row := map[string]any{"source": "guest"}
		if name != "" {
			row["name_long_override"] = name
		}
		ids, err := Write(r.Context(), s, queue, pics, actor, env, Batch{Batch: []Edit{
			{Insert: "PERSON", As: "guest", Row: row},
			{Insert: "PERSON_EMAIL", Row: map[string]any{"person": "@guest", "address": address, "primary": true, "source": "guest"}},
		}})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "added a guest", "viewer", env.Viewer, "person", ids[0], "address", address)
		serve.Write(w, r, http.StatusOK, written{Result: ids[:1]})
	})
}
