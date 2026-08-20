// Command deploy applies the production cloud run service configuration.
package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
)

const (
	service  = "heliosian"
	region   = "us-west1"
	image    = "us-west1-docker.pkg.dev/heliosian/heliosian/heliosian:latest"
	identity = "directory@heliosian.iam.gserviceaccount.com"
	secrets  = "SESSION_KEY=heliosian-session-key:latest," +
		"GOOGLE_MAPS_SERVER_KEY=heliosian-geocoding-key:latest," +
		"GOOGLE_MAPS_BROWSER_KEY=heliosian-maps-browser-key:latest"
)

func clientID() string {
	if id := os.Getenv("GOOGLE_CLIENT_ID"); id != "" {
		return id
	}
	raw, err := os.ReadFile("creds/oauth-client.json")
	if err != nil {
		log.Fatalf("[ERROR] read creds/oauth-client.json (or set GOOGLE_CLIENT_ID): %v", err)
	}
	var parsed struct {
		Web struct {
			ClientID string `json:"client_id"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Web.ClientID == "" {
		log.Fatal("[ERROR] creds/oauth-client.json is not an oauth web client file")
	}
	return parsed.Web.ClientID
}

func main() {
	sheet := os.Getenv("DIRECTORY_SHEET")
	if sheet == "" {
		log.Fatal("[ERROR] DIRECTORY_SHEET is required")
	}
	cmd := exec.Command("gcloud",
		"run", "deploy", service,
		"--image", image,
		"--region", region,
		"--service-account", identity,
		"--allow-unauthenticated",
		"--min-instances", "1",
		"--max-instances", "1",
		"--memory", "2Gi",
		"--concurrency", "250",
		"--no-cpu-throttling",
		"--use-http2",
		"--set-env-vars", "DIRECTORY_SHEET="+sheet+",GOOGLE_CLIENT_ID="+clientID(),
		"--set-secrets", secrets,
		"--quiet")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf("deploying %s to %s in %s", image, service, region)
	if err := cmd.Run(); err != nil {
		log.Fatalf("[ERROR] deploy: %v", err)
	}
}
