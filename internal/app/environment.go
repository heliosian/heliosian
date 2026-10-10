package app

import (
	"context"
	"os"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
	run "google.golang.org/api/run/v2"

	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/db"
	"heliosian/internal/env"
	"heliosian/internal/feedback"
	"heliosian/internal/imagesearch"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/model"
	"heliosian/internal/ops"
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

func githubApp() *feedback.GitHubApp {
	app, err := feedback.NewGitHubApp(env.Required("GITHUB_APP_ID"), env.Required("GITHUB_APP_KEY"))
	if err != nil {
		logging.Fatal("read the github app key", "error", err)
	}
	return app
}

const runtimeAccount = "directory@heliosian.iam.gserviceaccount.com"

func OpsDeps(github *feedback.GitHubApp, adminKey string) ops.Deps {
	builds, err := cloudbuild.NewService(context.Background())
	if err != nil {
		logging.Fatal("cloud build client", "error", err)
	}
	runs, err := run.NewService(context.Background())
	if err != nil {
		logging.Fatal("cloud run client", "error", err)
	}
	return ops.Deps{GitHub: github, Builds: builds, Run: runs, AdminKey: adminKey}
}

func calendarMail(sessionKey string) model.CalendarMail {
	return model.CalendarMail{Sender: newMailer(calendarMailFrom), ReplyTo: calendarReplyTo, Base: calendarBase, SigningKey: mailgunSigningKey(), Key: []byte(sessionKey)}
}

func mailgunKey() string {
	return env.Required("MAILGUN_KEY")
}

func mailgunSigningKey() string {
	return os.Getenv("MAILGUN_WEBHOOK_KEY")
}

func newMailer(from string) *mail.Mailgun {
	return mail.NewMailgun(mailgunKey(), from)
}

func listMail(sessionKey string) db.ListMailConfig {
	return db.ListMailConfig{Sender: mail.NewMailgun(mailgunKey(), ""), SigningKey: mailgunSigningKey(), Key: []byte(sessionKey), Base: "https://loop.heliosian.com"}
}

func artifactsMail(bucket *blob.Bucket) artifacts.Inbox {
	return artifacts.Inbox{SigningKey: mailgunSigningKey(), Bucket: bucket}
}

func ImageSearchKeys() imagesearch.Search {
	return imagesearch.Search{
		Unsplash: os.Getenv("UNSPLASH_KEY"),
		Pexels:   os.Getenv("PEXELS_KEY"),
		Pixabay:  os.Getenv("PIXABAY_KEY"),
	}
}
