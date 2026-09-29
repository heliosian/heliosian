package app

import (
	"net/http"

	"heliosian/internal/auth"
	"heliosian/internal/model"
	"heliosian/internal/serve"
)

func staleAlerts(directory *model.DirectoryCache, settings *model.ConfigCache) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (model.Alerts, error) {
		return directory.Alerts(auth.Email(r), settings.Config().StaleYears), nil
	})
}
