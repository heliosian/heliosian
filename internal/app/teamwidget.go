package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

func teamWidget(directory *who.Cache, teamCache *team.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := directory.Model().Resolve(auth.Email(r))
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(teamCache.Widget(email, time.Now().In(when.Location))); err != nil {
			slog.ErrorContext(r.Context(), "encode team widget", "error", err)
		}
	}
}
