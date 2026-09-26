package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/calendar"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

// teamWidget answers GET /api/apps/team on Heliosian's host: the home
// page's HCA-Team widget for the viewer - what they are signed up for that
// is still ahead, and what needs people (team.Cache.Widget).
func teamWidget(directory *who.Cache, teamCache *team.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := directory.Model().Resolve(auth.Email(r))
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(teamCache.Widget(email, time.Now().In(calendar.Location))); err != nil {
			slog.ErrorContext(r.Context(), "encode team widget", "error", err)
		}
	}
}
