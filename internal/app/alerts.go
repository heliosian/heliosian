package app

import (
	"net/http"

	"heliosian/internal/auth"
	"heliosian/internal/model"
	"heliosian/internal/serve"
)

func staleAlerts(s *model.Store) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (model.Alerts, error) {
		return s.Model().Alerts(auth.Email(r)), nil
	})
}
