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
	raw, err := os.ReadFile("local/creds/oauth-client.json")
	if err != nil {
		log.Fatalf("read local/creds/oauth-client.json (or set GOOGLE_CLIENT_ID): %v", err)
	}
	var parsed struct {
		Web struct {
			ClientID string `json:"client_id"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Web.ClientID == "" {
		log.Fatal("local/creds/oauth-client.json is not an oauth web client file")
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
		"MAILGUN_KEY=heliosian-mailgun-key:latest," +
		"MAILGUN_WEBHOOK_KEY=heliosian-mailgun-webhook-key:latest," +
		"GITHUB_APP_ID=heliosian-github-app-id:latest," +
		"GITHUB_APP_KEY=heliosian-github-app-key:latest," +
		"ANTHROPIC_API_KEY=heliosian-anthropic-key:latest"
)

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func sheetEnvVars(sheets []app.Spreadsheet) string {
	pairs := []string{}
	for _, s := range sheets {
		pairs = append(pairs, s.Env+"="+requiredEnv(s.Env))
	}
	return strings.Join(pairs, ",")
}

func gcloud(args ...string) {
	cmd := exec.Command("gcloud", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("gcloud %s: %v", args[0], err)
	}
}

func gcloudLines(args ...string) []string {
	cmd := exec.Command("gcloud", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("gcloud %s: %v", args[0], err)
	}
	return strings.Fields(string(out))
}

func mapDomains() {
	have := gcloudLines("beta", "run", "domain-mappings", "list", "--region", region, "--format", "value(metadata.name)")
	for _, host := range app.Hostnames() {
		if slices.Contains(have, host) {
			continue
		}
		log.Printf("mapping %s to %s", host, service)
		gcloud("beta", "run", "domain-mappings", "create", "--service", service, "--domain", host, "--region", region, "--quiet")
	}
}

func main() {
	jobEnvVars := sheetEnvVars(app.SpreadsheetsOf(app.SyncSources))
	envVars := sheetEnvVars(app.Spreadsheets) + ",GOOGLE_CLIENT_ID=" + clientID()
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
