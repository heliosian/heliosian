package app

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"heliosian/internal/auth"
	"heliosian/internal/birthday"
	"heliosian/internal/who"
)

// lateBirthdays answers GET /api/apps/late on every app's host: the
// birthday steps the viewer is behind on - an admin, everyone's - for the
// shared toolbar's alert (birthday.Cache.Late), with or without Super Admin
// Mode, as approvals are - and whether it is an admin's list, everyone's,
// so the badge goes to Process rather than My Jobs.
func lateBirthdays(directory *who.Cache, birthdayCache *birthday.Cache, people birthday.Directory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := directory.Model().Resolve(auth.Email(r))
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Late  []birthday.Late `json:"late"`
			Admin bool            `json:"admin"`
		}{birthdayCache.Late(people, email), birthdayCache.IsAdmin(email)}); err != nil {
			slog.ErrorContext(r.Context(), "encode late birthdays", "error", err)
		}
	}
}
