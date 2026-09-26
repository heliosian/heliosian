package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/calendar"
	"heliosian/internal/who"
)

// celebrateWidget answers GET /api/apps/celebrate on Heliosian's host: the
// home page's Helios Celebrate widget for the viewer - every party still
// ahead with their household's standing and the way in, as the calendar
// cards them (calendar.Model.PartiesFor).
func celebrateWidget(directory *who.Cache, calendarCache *calendar.Cache, people calendar.Directory, linked func(email string) []calendar.Linked) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := directory.Model().Resolve(auth.Email(r))
		parties := calendarCache.Model().PartiesFor(people, email, linked(email), time.Now().In(calendar.Location))
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Parties []calendar.Card `json:"parties"`
		}{parties}); err != nil {
			slog.ErrorContext(r.Context(), "encode celebrate widget", "error", err)
		}
	}
}
