package main

import (
	"log/slog"

	"heliosian/internal/capture"
	"heliosian/internal/logging"
)

func main() {
	profile, err := capture.Start()
	if err != nil {
		logging.Fatal("start capture browser", "error", err)
	}
	slog.Info("capture browser running", "devtools", capture.DevTools, "profile", profile)
}
