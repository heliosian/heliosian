package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

func celebrateWidget(directory *who.Cache, calendarCache *when.Cache, people when.Directory, linked func(email string) []when.Linked) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := directory.Model().Resolve(auth.Email(r))
		parties := calendarCache.Model().PartiesFor(people, email, linked(email), time.Now().In(when.Location))
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Parties []when.Card `json:"parties"`
		}{parties}); err != nil {
			slog.ErrorContext(r.Context(), "encode celebrate widget", "error", err)
		}
	}
}
