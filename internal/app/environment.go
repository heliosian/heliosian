package app

import (
	"os"

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
	app, err := feedback.NewGitHubApp(env.Required("GITHUB_APP_ID"), env.Required("GITHUB_APP_KEY"))
	if err != nil {
		logging.Fatal("read the github app key", "error", err)
	}
	return app
}

func calendarMail(sessionKey string) when.Mail {
	return when.Mail{Sender: newMailer(calendarMailFrom), ReplyTo: calendarReplyTo, Base: calendarBase, SigningKey: mailgunSigningKey(), Key: []byte(sessionKey)}
}

func mailgunKey() string {
	return env.Required("MAILGUN_KEY")
}

func mailgunSigningKey() string {
	return os.Getenv("MAILGUN_WEBHOOK_KEY")
}

func newMailer(from string) mail.Sender {
	return mail.NewMailgun(mailgunKey(), from)
}

func loopMail(sessionKey string) loop.Mail {
	archive, err := blob.Open(blob.MailBucket)
	if err != nil {
		logging.Fatal("mail archive", "error", err)
	}
	return loop.Mail{Sender: mail.NewMailgun(mailgunKey(), ""), SigningKey: mailgunSigningKey(), Key: []byte(sessionKey), Base: "https://loop.heliosian.com", Archive: archive}
}

func artifactsMail(bucket *blob.Bucket) artifacts.Inbox {
	return artifacts.Inbox{SigningKey: mailgunSigningKey(), Bucket: bucket}
}

// A nil *describe.Describer must stay a nil interface, or the app would call it.
func ClaudeDescriber() birthday.Describer {
	if d := describe.New(os.Getenv("ANTHROPIC_API_KEY"), spend); d != nil {
		return d
	}
	return nil
}

func ClaudeGroupDescriber() loop.Describer {
	if d := describe.New(os.Getenv("ANTHROPIC_API_KEY"), spend); d != nil {
		return d
	}
	return nil
}

func ClaudeKeyPoints() keypoints.Summarizer {
	if c := keypoints.New(os.Getenv("ANTHROPIC_API_KEY")); c != nil {
		return c
	}
	return nil
}

func ClaudeAsker() ask.Responder {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return ask.NewClaude(key)
	}
	return nil
}

func ImageSearchKeys() imagesearch.Search {
	return imagesearch.Search{
		Unsplash: os.Getenv("UNSPLASH_KEY"),
		Pexels:   os.Getenv("PEXELS_KEY"),
		Pixabay:  os.Getenv("PIXABAY_KEY"),
	}
}
