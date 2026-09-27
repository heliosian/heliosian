package app

import (
	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/birthday"
	"heliosian/internal/blob"
	"heliosian/internal/describe"
	"heliosian/internal/env"
	"heliosian/internal/feedback"
	"heliosian/internal/imagesearch"
	"heliosian/internal/keypoints"
	"heliosian/internal/logging"
	"heliosian/internal/loop"
	"heliosian/internal/mail"
	"heliosian/internal/when"
)

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
	app, err := feedback.NewGitHubApp(env.Key("GITHUB_APP_ID", "local/creds/github-app.id"), env.Key("GITHUB_APP_KEY", "local/creds/github-app.pem"))
	if err != nil {
		logging.Fatal("read the github app key", "error", err)
	}
	return app
}

func calendarMail(sessionKey string) when.Mail {
	return when.Mail{Sender: newMailer(calendarMailFrom), From: calendarMailFrom, ReplyTo: calendarReplyTo, Base: calendarBase, SigningKey: mailgunSigningKey(), Key: []byte(sessionKey)}
}

func mailgunKey() string {
	return env.OptionalKey("MAILGUN_KEY", "local/creds/mailgun.key")
}

func mailgunSigningKey() string {
	return env.OptionalKey("MAILGUN_WEBHOOK_KEY", "local/creds/mailgun-webhook.key")
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
	if d := describe.New(env.OptionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"), spend); d != nil {
		return d
	}
	return nil
}

func ClaudeGroupDescriber() loop.Describer {
	if d := describe.New(env.OptionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"), spend); d != nil {
		return d
	}
	return nil
}

func ClaudeKeyPoints() keypoints.Summarizer {
	if c := keypoints.New(env.OptionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key")); c != nil {
		return c
	}
	return nil
}

func ClaudeAsker() ask.Responder {
	if key := env.OptionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"); key != "" {
		return ask.NewClaude(key)
	}
	return nil
}

func ImageSearchKeys() imagesearch.Search {
	return imagesearch.Search{
		Unsplash: env.OptionalKey("UNSPLASH_KEY", "local/creds/unsplash.key"),
		Pexels:   env.OptionalKey("PEXELS_KEY", "local/creds/pexels.key"),
		Pixabay:  env.OptionalKey("PIXABAY_KEY", "local/creds/pixabay.key"),
	}
}
