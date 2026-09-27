package app

import (
	"encoding/json"
	"os"
	"strings"

	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/birthday"
	"heliosian/internal/blob"
	"heliosian/internal/calendar"
	"heliosian/internal/describe"
	"heliosian/internal/feedback"
	"heliosian/internal/imagesearch"
	"heliosian/internal/keypoints"
	"heliosian/internal/logging"
	"heliosian/internal/loop"
	"heliosian/internal/mail"
)

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		logging.Fatal("environment variable is required", "name", name)
	}
	return value
}

func clientID() string {
	if id := os.Getenv("GOOGLE_CLIENT_ID"); id != "" {
		return id
	}
	raw, err := os.ReadFile("local/creds/oauth-client.json")
	if err != nil {
		logging.Fatal("read local/creds/oauth-client.json (or set GOOGLE_CLIENT_ID)", "error", err)
	}
	var parsed struct {
		Web struct {
			ClientID string `json:"client_id"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Web.ClientID == "" {
		logging.Fatal("local/creds/oauth-client.json is not an oauth web client file")
	}
	return parsed.Web.ClientID
}

const (
	mailFrom          = "HCA-Team <team@loop.heliosian.com>"
	birthdayMailFrom  = "Helios Staff Birthdays <birthday@reply.heliosian.com>"
	birthdayBase      = "https://birthday.heliosian.com"
	feedbackBase      = "https://heliosian.com"
	calendarMailFrom  = "Helios When <when@reply.heliosian.com>"
	calendarReplyTo   = "Helios When <when@reply.heliosian.com>"
	calendarBase      = "https://when.heliosian.com"
	celebrateMailFrom = "Helios Celebrate <celebrate@reply.heliosian.com>"
)

func githubApp() feedback.IssueFiler {
	app, err := feedback.NewGitHubApp(mapsKey("GITHUB_APP_ID", "local/creds/github-app.id"), mapsKey("GITHUB_APP_KEY", "local/creds/github-app.pem"))
	if err != nil {
		logging.Fatal("read the github app key", "error", err)
	}
	return app
}

func calendarMail(sessionKey string) calendar.Mail {
	return calendar.Mail{Sender: newMailer(calendarMailFrom), From: calendarMailFrom, ReplyTo: calendarReplyTo, Base: calendarBase, SigningKey: mailgunSigningKey(), Key: []byte(sessionKey)}
}

func mailgunKey() string {
	return optionalKey("MAILGUN_KEY", "local/creds/mailgun.key")
}

func mailgunSigningKey() string {
	return optionalKey("MAILGUN_WEBHOOK_KEY", "local/creds/mailgun-webhook.key")
}

func newMailer(from string) mail.Sender {
	return mail.New(mailgunKey(), from, "")
}

func loopMail(sessionKey string) loop.Mail {
	archive, err := blob.Open(blob.MailBucket)
	if err != nil {
		logging.Fatal("mail archive", "error", err)
	}
	m := loop.Mail{SigningKey: mailgunSigningKey(), Key: []byte(sessionKey), Base: "https://loop.heliosian.com", Archive: archive}
	if key := mailgunKey(); key != "" {
		m.Sender = mail.NewMailgun(key, "")
	}
	return m
}

func artifactsMail(bucket *blob.Bucket) artifacts.Inbox {
	return artifacts.Inbox{SigningKey: mailgunSigningKey(), Bucket: bucket}
}

// A nil *describe.Describer must stay a nil interface, or the app would call it.
func ClaudeDescriber() birthday.Describer {
	if d := describe.New(optionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"), spend); d != nil {
		return d
	}
	return nil
}

func ClaudeGroupDescriber() loop.Describer {
	if d := describe.New(optionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"), spend); d != nil {
		return d
	}
	return nil
}

func ClaudeKeyPoints() keypoints.Summarizer {
	if c := keypoints.New(optionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key")); c != nil {
		return c
	}
	return nil
}

func ClaudeAsker() ask.Responder {
	if key := optionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"); key != "" {
		return ask.NewClaude(key)
	}
	return nil
}

func ImageSearchKeys() imagesearch.Search {
	return imagesearch.Search{
		Unsplash: optionalKey("UNSPLASH_KEY", "local/creds/unsplash.key"),
		Pexels:   optionalKey("PEXELS_KEY", "local/creds/pexels.key"),
		Pixabay:  optionalKey("PIXABAY_KEY", "local/creds/pixabay.key"),
	}
}

func optionalKey(envName, file string) string {
	if key := os.Getenv(envName); key != "" {
		return key
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func mapsKey(envName, file string) string {
	if key := os.Getenv(envName); key != "" {
		return key
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		logging.Fatal("read key file", "file", file, "or set", envName, "error", err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		logging.Fatal("key file is empty", "file", file)
	}
	return key
}
