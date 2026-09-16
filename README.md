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
- `internal/events` — the volunteer portal: the Events sheet's model, handlers, sign-ups, and admin edits
- `internal/birthday` — the birthday team: the Birthdays sheet's model, the derived dates and stages, handlers, and admin edits
- `internal/celebrate` — Helios Celebrate: the Celebrate sheet's model, parties, tickets and the waitlist, handlers, and admin edits
- `internal/calendar` — Helios Calendar: the Calendar sheet's model, audience resolution, the day plan, handlers, and personal feeds
- `internal/calendarimport` — the periodic sync's calendar stage: the school's feed and year calendar into the Calendar sheet, classified by Claude
- `internal/groups` — Helios Loop: the Groups sheet's model, the rule evaluator, the Google Groups sync, handlers, and admin edits
- `internal/ics` — iCalendar feed parsing and recurrence expansion
- `internal/feedback` — the toolbar's reports, queued and filed as issues in the private triage repository
- `internal/blob` — media from Cloud Storage, held in memory with stored thumbnails
- `internal/sharecard` — the 1200x630 picture a chat app shows for a shared link, drawn for HCA-Team and Helios Celebrate in each one's dress
- `internal/imagesearch` — the editors' picture search (stock libraries and Wikimedia Commons) and import, shared by HCA-Team and Heliosian
- `internal/geocode` — address → coordinates for the map
- `internal/capture` — drive Chrome and screenshot a page, for the dev tools
- `internal/devtls` — the in-memory self-signed certificate local HTTPS runs on
- `web/` — one directory per app, named for its hostname, holding its pages and every file it serves behind sign-in, `web/common/` for what all apps share, and `web/public/<app>/` and `web/public/common/` for the few files served before sign-in (frameworkless JavaScript throughout)
- `sampledata/` — the fictional community served by default
- `cmd/` — dev tooling: screenshots, browser driving, sheet inspection; and `cmd/periodicsync`, the scheduled job that runs the calendar import and the groups reconciliation in turn
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
- [plan.md](docs/plan.md) — what remains to build

Helios Who?, in `docs/who/`:

- [directory.md](docs/who/directory.md) — the directory app spec
- [data.md](docs/who/data.md) — the directory data model and load pipeline
- [design.md](docs/who/design.md) — palette, typography, brand
- [pwa.md](docs/who/pwa.md) — installable-app wiring

Heliosian, in `docs/home/`:

- [home.md](docs/home/home.md) — the link portal spec
- [data.md](docs/home/data.md) — the Apps sheet and its rules

HCA-Team, in `docs/events/`:

- [events.md](docs/events/events.md) — the volunteer portal spec
- [data.md](docs/events/data.md) — the Events sheet and its rules

Helios Staff Birthdays, in `docs/birthday/`:

- [birthday.md](docs/birthday/birthday.md) — the birthday team app spec
- [data.md](docs/birthday/data.md) — the Birthdays sheet and its rules

Helios Celebrate, in `docs/celebrate/`:

- [celebrate.md](docs/celebrate/celebrate.md) — the fun(d)raiser parties site spec
- [data.md](docs/celebrate/data.md) — the Celebrate sheet and its rules

Helios Calendar, in `docs/calendar/`:

- [calendar.md](docs/calendar/calendar.md) — the school calendar app spec: the pages, the filters, and personal feeds
- [data.md](docs/calendar/data.md) — the Calendar sheet, the day plan, and the import

Helios Loop, in `docs/groups/`:

- [groups.md](docs/groups/groups.md) — the email groups app spec: groups, rules, managers, and how Google is kept in step
- [data.md](docs/groups/data.md) — the Groups sheet, the membership evaluator, the Google side, and the periodic job

Each app keeps its own docs under `docs/<app>/`, named for the sheet it serves; the top level is only what every app shares.
