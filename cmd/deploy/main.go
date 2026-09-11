// Command deploy applies the production cloud run service configuration.
package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
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

const (
	service  = "heliosian"
	region   = "us-west1"
	image    = "us-west1-docker.pkg.dev/heliosian/heliosian/heliosian:latest"
	identity = "directory@heliosian.iam.gserviceaccount.com"
	secrets  = "SESSION_KEY=heliosian-session-key:latest," +
		"GOOGLE_MAPS_SERVER_KEY=heliosian-geocoding-key:latest," +
		"GOOGLE_MAPS_BROWSER_KEY=heliosian-maps-browser-key:latest," +
		"UNSPLASH_KEY=heliosian-unsplash-key:latest," +
		"PEXELS_KEY=heliosian-pexels-key:latest," +
		"PIXABAY_KEY=heliosian-pixabay-key:latest," +
		"RESEND_KEY=heliosian-resend-key:latest"
)

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("[ERROR] %s is required", name)
	}
	return value
}

func main() {
	envVars := "DIRECTORY_SHEET=" + requiredEnv("DIRECTORY_SHEET") +
		",PREFERENCES_SHEET=" + requiredEnv("PREFERENCES_SHEET") +
		",INVITES_SHEET=" + requiredEnv("INVITES_SHEET") +
		",APPS_SHEET=" + requiredEnv("APPS_SHEET") +
		",EVENTS_SHEET=" + requiredEnv("EVENTS_SHEET") +
		",BIRTHDAY_SHEET=" + requiredEnv("BIRTHDAY_SHEET") +
		",CONFIG_SHEET=" + requiredEnv("CONFIG_SHEET") +
		",GOOGLE_CLIENT_ID=" + clientID()
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
		"--set-env-vars", envVars,
		"--set-secrets", secrets,
		"--quiet")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf("deploying %s to %s in %s", image, service, region)
	if err := cmd.Run(); err != nil {
		log.Fatalf("[ERROR] deploy: %v", err)
	}
}
