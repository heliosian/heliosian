package app

import (
	"net/http"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/model"
	"heliosian/internal/serve"
)

func teamWidget(directory *model.DirectoryCache, teamCache *model.ActivitiesCache) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (model.ActivityWidget, error) {
		email := directory.Model().Resolve(auth.Email(r))
		return teamCache.Widget(email, time.Now().In(model.Location)), nil
	})
}
