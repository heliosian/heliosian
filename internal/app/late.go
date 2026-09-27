package app

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"heliosian/internal/auth"
	"heliosian/internal/birthday"
	"heliosian/internal/who"
)

func lateBirthdays(directory *who.Cache, birthdayCache *birthday.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := directory.Model().Resolve(auth.Email(r))
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Late  []birthday.Late `json:"late"`
			Admin bool            `json:"admin"`
		}{birthdayCache.Late(directory.Model, email), birthdayCache.IsAdmin(email)}); err != nil {
			slog.ErrorContext(r.Context(), "encode late birthdays", "error", err)
		}
	}
}
