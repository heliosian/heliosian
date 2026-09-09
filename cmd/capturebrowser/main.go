// Command capturebrowser launches the headed capture browser used to screenshot authenticated sites.
package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	profile := filepath.Join(os.Getenv("HOME"), ".heliosian", "capture-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		log.Fatalf("[ERROR] create profile dir: %v", err)
	}
	cmd := exec.Command("open", "-na", "Google Chrome", "--args",
		"--user-data-dir="+profile,
		"--remote-debugging-port=9222",
		"--no-first-run",
		"--no-default-browser-check")
	if err := cmd.Run(); err != nil {
		log.Fatalf("[ERROR] launch chrome: %v", err)
	}
	log.Printf("capture browser running, devtools on http://localhost:9222, profile in %s", profile)
}
