package app

import (
	"net/http"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/serve"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

func teamWidget(directory *who.Cache, teamCache *team.Cache) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (team.Widget, error) {
		email := directory.Model().Resolve(auth.Email(r))
		return teamCache.Widget(email, time.Now().In(when.Location)), nil
	})
}
