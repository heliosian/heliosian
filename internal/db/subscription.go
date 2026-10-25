package db

import (
	"log/slog"
	"net/http"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/serve"
)

func registerSubscription(mux *http.ServeMux, s *Store, tokens auth.Tokens, now func() time.Time) {
	mux.HandleFunc("POST /api/do/subscription", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, _, ok := caller(w, r, m, tokens, now())
		if !ok {
			return
		}
		if env.Viewer == "" {
			serve.Error(w, r, access.Forbidden("only a person subscribes or unsubscribes"))
			return
		}
		var asked struct {
			Group      string `json:"group"`
			Subscribed bool   `json:"subscribed"`
		}
		if !serve.Decode(w, r, &asked) {
			return
		}
		g, found := m.Table("GROUP").Get(asked.Group)
		if !found || !groupOpen(g) || !(m.effectivelyIn(g["id"], env.Viewer) || m.unsubscribed(g, env.Viewer)) {
			serve.Error(w, r, access.Missing("you are not in a group %s", asked.Group))
			return
		}
		edits := m.unsubscribeEdits(g, env.Viewer, "Unsubscribed in Loop")
		if asked.Subscribed {
			edits = m.resubscribeEdits(g, env.Viewer)
		}
		if len(edits) > 0 {
			if _, err := Write(r.Context(), s, access.System(mailerSystem), Env{System: mailerSystem, Now: now()}, Batch{Batch: edits}); err != nil {
				serve.Error(w, r, err)
				return
			}
		}
		slog.InfoContext(r.Context(), "list mail: subscription set", "group", g["id"], "person", env.Viewer, "subscribed", asked.Subscribed)
		serve.Write(w, r, http.StatusOK, written{Result: []string{g["id"]}})
	})
}
