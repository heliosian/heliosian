package env

import (
	"os"

	"heliosian/internal/logging"
)

type Secret struct {
	Env  string
	Name string
}

var Secrets = []Secret{
	{"SESSION_KEY", "heliosian-session-key"},
	{"ID_KEY", "heliosian-id-key"},
	{"GOOGLE_CLIENT_ID", "heliosian-oauth-client-id"},
	{"GOOGLE_MAPS_SERVER_KEY", "heliosian-geocoding-key"},
	{"GOOGLE_MAPS_BROWSER_KEY", "heliosian-maps-browser-key"},
	{"UNSPLASH_KEY", "heliosian-unsplash-key"},
	{"PEXELS_KEY", "heliosian-pexels-key"},
	{"PIXABAY_KEY", "heliosian-pixabay-key"},
	{"MAILGUN_KEY", "heliosian-mailgun-key"},
	{"MAILGUN_WEBHOOK_KEY", "heliosian-mailgun-webhook-key"},
	{"GITHUB_APP_ID", "heliosian-github-app-id"},
	{"GITHUB_APP_KEY", "heliosian-github-app-key"},
	{"ANTHROPIC_API_KEY", "heliosian-anthropic-key"},
	{"ANTHROPIC_ADMIN_KEY", "heliosian-anthropic-admin-key"},
	{"GITHUB_WEBHOOK_SECRET", "heliosian-github-webhook-secret"},
	{"IMPORT_KEY", "heliosian-import-key"},
}

func Required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		logging.Fatal("environment variable is required", "name", name)
	}
	return value
}
