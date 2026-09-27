package app

import (
	"net/http"

	"heliosian/internal/auth"
	"heliosian/internal/birthday"
	"heliosian/internal/serve"
	"heliosian/internal/who"
)

type lateView struct {
	Late  []birthday.Late `json:"late"`
	Admin bool            `json:"admin"`
}

func lateBirthdays(directory *who.Cache, birthdayCache *birthday.Cache) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (lateView, error) {
		email := directory.Model().Resolve(auth.Email(r))
		return lateView{birthdayCache.Late(directory.Model, email), birthdayCache.IsAdmin(email)}, nil
	})
}
