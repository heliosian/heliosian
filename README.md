# Heliosian

Web apps for the Helios school community (K-8), served as one static Go binary on Cloud Run, one app per hostname under heliosian.com; each app is described under Docs below. Built in the open by community volunteers, mostly through coding agents; the repo contains no secrets and no real community data.

## Quick start

    brew install go
    go run ./cmd/startserver

Open https://who.local.heliosian.com:8080 and click past the browser's warning about the self-signed certificate (the server picks the app by hostname; that name resolves to your own machine). That's the whole setup: the dev server loads the fictional community in `sampledata/`, signs every request in as a sample parent, and fakes geocoding — no credentials, no cloud project. Templates, static assets, and sample data are read from disk on every request, so edit a file and refresh; nothing needs restarting.

`brew install --cask google-chrome` additionally enables the screenshot tooling used to verify visual changes ([docs/screenshots.md](docs/screenshots.md)). No Node, no Docker. Go 1.27 or later.

To run against real community data instead, see [docs/dev.md](docs/dev.md).

## Layout

- `main.go` — the production entry point; dev serving lives in `cmd/startserver`
- `internal/app` — server wiring shared by production and the dev server: host routing, file serving, and the production assembly
- `internal/auth` — Google sign-in and session cookies
- `internal/data` — tabular data sources: sample CSVs and Google Sheets
- `internal/config` — the Config sheet: super admins and the platform settings, served at `/api/config`
- `internal/who` — the directory app: model load, handlers, admin tools, self-service edits
- `internal/home` — the link portal: model load, handlers, admin edits
- `internal/team` — HCA-Team, the volunteer portal: the Events sheet's model, handlers, sign-ups, and admin edits
- `internal/birthday` — the birthday team: the Birthdays sheet's model, the derived dates and stages, handlers, and admin edits
- `internal/celebrate` — Helios Celebrate: the Celebrate sheet's model, parties, tickets and the waitlist, handlers, and admin edits
- `internal/calendar` — Helios When: the Calendar sheet's model, audience resolution, the day plan, handlers, and personal feeds
- `internal/calendarimport` — the calendar import: the school's Google Calendar into the Calendar sheet as it changes, from the serving binary, and its year calendar PDF from the periodic sync, classified by Claude
- `internal/loop` — Helios Loop: the Groups sheet's model, the rule evaluator, the mail received and forwarded, handlers, and admin edits
- `internal/ask` — Helios Ask: the chat with Claude over every app's data - the prompt, the conversation, the streaming turn and the read-only tools
- `internal/artifacts` — the community's documents Helios Ask searches: which mail and which website pages belong, messages and pages to markdown, the chunks, their embeddings through Vertex AI, and the model that scans them
- `internal/feedback` — the toolbar's reports, queued and filed as issues in the private triage repository
- `internal/blob` — media from Cloud Storage, held in memory with stored thumbnails
- `internal/sharecard` — the 1200x630 picture a chat app shows for a shared link, drawn for HCA-Team and Helios Celebrate in each one's dress
- `internal/imagesearch` — the editors' picture search (stock libraries and Wikimedia Commons) and import, shared by HCA-Team and Heliosian
- `internal/geocode` — address → coordinates for the map
- `internal/capture` — drive Chrome and screenshot a page, for the dev tools
- `internal/devtls` — the in-memory self-signed certificate local HTTPS runs on
- `web/` — one directory per app, named for its hostname, holding its pages and every file it serves behind sign-in, `web/common/` for what all apps share, and `web/public/<app>/` and `web/public/common/` for the few files served before sign-in (frameworkless JavaScript throughout)
- `sampledata/` — the fictional community served by default
- `cmd/` — dev tooling: screenshots, browser driving, sheet inspection; and `cmd/periodicsync`, the scheduled job that reads the school's year calendar PDF
- `docs/` — everything below

## Docs

Platform:

- [goals.md](docs/goals.md) — what this is and the principles behind it
- [dev.md](docs/dev.md) — local development, including real-data mode
- [screenshots.md](docs/screenshots.md) — page capture for humans and agents
- [deploy.md](docs/deploy.md) — production deployment
- [config.md](docs/config.md) — the Config sheet: super admins and the platform settings
- [toolbar.md](docs/toolbar.md) — the top bar every app shares: search with `/`, the account avatar, the switch between apps
- [feedback.md](docs/feedback.md) — "Report a problem or idea": the form in every app and the private triage repository it files to

Helios Who?, in `docs/who/`:

- [directory.md](docs/who/directory.md) — the directory app spec
- [data.md](docs/who/data.md) — the directory data model and load pipeline
- [design.md](docs/who/design.md) — palette, typography, brand
- [pwa.md](docs/who/pwa.md) — installable-app wiring

Heliosian, in `docs/home/`:

- [home.md](docs/home/home.md) — the link portal spec
- [data.md](docs/home/data.md) — the Apps sheet and its rules

HCA-Team, in `docs/team/`:

- [team.md](docs/team/team.md) — the volunteer portal spec
- [data.md](docs/team/data.md) — the Events sheet and its rules

Helios Staff Birthdays, in `docs/birthday/`:

- [birthday.md](docs/birthday/birthday.md) — the birthday team app spec
- [data.md](docs/birthday/data.md) — the Birthdays sheet and its rules

Helios Celebrate, in `docs/celebrate/`:

- [celebrate.md](docs/celebrate/celebrate.md) — the fun(d)raiser parties site spec
- [data.md](docs/celebrate/data.md) — the Celebrate sheet and its rules

Helios When, in `docs/calendar/`:

- [calendar.md](docs/calendar/calendar.md) — the school calendar app spec: the pages, the filters, and personal feeds
- [data.md](docs/calendar/data.md) — the Calendar sheet, the day plan, and the import

Helios Loop, in `docs/loop/`:

- [loop.md](docs/loop/loop.md) — the email groups app spec: groups, rules, managers, and how mail to a group reaches its members
- [data.md](docs/loop/data.md) — the Groups sheet, the membership evaluator, and the mail pipeline through Mailgun

Helios Ask, in `docs/ask/`:

- [ask.md](docs/ask/ask.md) — the chat app spec: the page, what Claude knows, what it can read
- [data.md](docs/ask/data.md) — the call to Claude, the conversation, the tools and their limits
- [artifacts.md](docs/ask/artifacts.md) — the community's documents, its mail and its website's pages: what is in them and what is withheld, how they become markdown, chunks and embeddings, and how they are searched

Each app keeps its own docs under `docs/<app>/`, named for its hostname; the top level is only what every app shares.
