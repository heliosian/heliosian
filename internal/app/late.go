package app

import (
	"net/http"

	"heliosian/internal/auth"
	"heliosian/internal/model"
	"heliosian/internal/serve"
)

type lateView struct {
	Late  []model.Late `json:"late"`
	Admin bool         `json:"admin"`
}

func lateBirthdays(directory *model.DirectoryCache, birthdayCache *model.BirthdaysCache) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (lateView, error) {
		email := directory.Model().Resolve(auth.Email(r))
		return lateView{birthdayCache.Late(directory.Model, email), birthdayCache.IsAdmin(email)}, nil
	})
}
