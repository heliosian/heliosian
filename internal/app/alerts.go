package app

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"heliosian/internal/auth"
	"heliosian/internal/config"
	"heliosian/internal/who"
)

func staleAlerts(directory *who.Cache, settings *config.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(directory.Alerts(auth.Email(r), settings.Settings().StaleYears)); err != nil {
			slog.ErrorContext(r.Context(), "encode alerts", "error", err)
		}
	}
}
