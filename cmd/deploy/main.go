// Command deploy applies the production cloud run service configuration.
package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"

	"heliosian/internal/app"
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
	service    = "heliosian"
	region     = "us-west1"
	image      = "us-west1-docker.pkg.dev/heliosian/heliosian/heliosian:latest"
	job        = "periodicsync"
	jobImage   = "us-west1-docker.pkg.dev/heliosian/heliosian/periodicsync:latest"
	jobSecrets = "ANTHROPIC_API_KEY=heliosian-anthropic-key:latest"
	identity   = "directory@heliosian.iam.gserviceaccount.com"
	secrets    = "SESSION_KEY=heliosian-session-key:latest," +
		"GOOGLE_MAPS_SERVER_KEY=heliosian-geocoding-key:latest," +
		"GOOGLE_MAPS_BROWSER_KEY=heliosian-maps-browser-key:latest," +
		"UNSPLASH_KEY=heliosian-unsplash-key:latest," +
		"PEXELS_KEY=heliosian-pexels-key:latest," +
		"PIXABAY_KEY=heliosian-pixabay-key:latest," +
		"RESEND_KEY=heliosian-resend-key:latest," +
		"RESEND_WEBHOOK_SECRET=heliosian-resend-webhook-secret:latest," +
		"GITHUB_TOKEN=heliosian-github-token:latest," +
		"ANTHROPIC_API_KEY=heliosian-anthropic-key:latest"
)

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("[ERROR] %s is required", name)
	}
	return value
}

func gcloud(args ...string) {
	cmd := exec.Command("gcloud", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("[ERROR] gcloud %s: %v", args[0], err)
	}
}

// gcloudLines runs a gcloud command for its output, one value per line.
func gcloudLines(args ...string) []string {
	cmd := exec.Command("gcloud", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("[ERROR] gcloud %s: %v", args[0], err)
	}
	return strings.Fields(string(out))
}

// mapDomains gives the service a domain mapping for every hostname the
// router answers (app.Hostnames) that it lacks. Existing mappings are left
// as they are and none is ever removed; the DNS record each new one needs
// is printed, since that is written at the registrar by hand.
func mapDomains() {
	have := gcloudLines("beta", "run", "domain-mappings", "list", "--region", region, "--format", "value(metadata.name)")
	for _, host := range app.Hostnames() {
		if slices.Contains(have, host) {
			continue
		}
		log.Printf("mapping %s to %s", host, service)
		gcloud("beta", "run", "domain-mappings", "create", "--service", service, "--domain", host, "--region", region, "--quiet")
		log.Printf("add at the registrar: %s CNAME ghs.googlehosted.com.", strings.TrimSuffix(host, ".heliosian.com"))
	}
}

func main() {
	jobEnvVars := "DIRECTORY_SHEET=" + requiredEnv("DIRECTORY_SHEET") +
		",PREFERENCES_SHEET=" + requiredEnv("PREFERENCES_SHEET") +
		",CALENDAR_SHEET=" + requiredEnv("CALENDAR_SHEET") +
		",CONFIG_SHEET=" + requiredEnv("CONFIG_SHEET") +
		",EVENTS_SHEET=" + requiredEnv("EVENTS_SHEET") +
		",CELEBRATE_SHEET=" + requiredEnv("CELEBRATE_SHEET") +
		",GROUPS_SHEET=" + requiredEnv("GROUPS_SHEET")
	envVars := jobEnvVars +
		",INVITES_SHEET=" + requiredEnv("INVITES_SHEET") +
		",APPS_SHEET=" + requiredEnv("APPS_SHEET") +
		",BIRTHDAY_SHEET=" + requiredEnv("BIRTHDAY_SHEET") +
		",GOOGLE_CLIENT_ID=" + clientID()
	log.Printf("deploying %s to %s in %s", image, service, region)
	gcloud("run", "deploy", service,
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
	mapDomains()
	log.Printf("deploying %s to job %s in %s", jobImage, job, region)
	gcloud("run", "jobs", "deploy", job,
		"--image", jobImage,
		"--region", region,
		"--service-account", identity,
		"--memory", "1Gi",
		"--task-timeout", "30m",
		"--max-retries", "0",
		"--args=--i-have-user-permission-to-spend-money",
		"--set-env-vars", jobEnvVars,
		"--set-secrets", jobSecrets,
		"--quiet")
}
