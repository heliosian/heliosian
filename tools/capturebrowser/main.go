package main

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"heliosian/internal/capture"
	"heliosian/internal/logging"
)

func main() {
	profile, err := filepath.Abs("local/capture-profile")
	if err != nil {
		logging.Fatal("resolve profile dir", "error", err)
	}
	if err := os.MkdirAll(profile, 0o700); err != nil {
		logging.Fatal("create profile dir", "error", err)
	}
	cmd := exec.Command("open", "-na", "Google Chrome", "--args",
		"--user-data-dir="+profile,
		"--remote-debugging-port="+capture.Port,
		"--no-first-run",
		"--no-default-browser-check")
	if err := cmd.Run(); err != nil {
		logging.Fatal("launch chrome", "error", err)
	}
	slog.Info("capture browser running", "devtools", capture.DevTools, "profile", profile)
}
