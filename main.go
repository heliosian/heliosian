// Heliosian serves the Helios school community apps in production. Local
// development against sample data runs through cmd/startserver instead.
package main

import (
	"log/slog"

	"heliosian/internal/app"
	"heliosian/internal/logging"
)

func main() {
	slog.SetDefault(logging.Cloud())
	app.Serve(app.Production())
}
