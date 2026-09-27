package app

import (
	"net/http"

	"heliosian/internal/auth"
	"heliosian/internal/config"
	"heliosian/internal/serve"
	"heliosian/internal/who"
)

func staleAlerts(directory *who.Cache, settings *config.Cache) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (who.Alerts, error) {
		return directory.Alerts(auth.Email(r), settings.Settings().StaleYears), nil
	})
}
