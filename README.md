# Heliosian

Web apps for the Helios school community (K-8), served as one static Go binary on Cloud Run, one app per hostname. The first app is the school directory, "Helios Who?", at who.heliosian.com. Built in the open by community volunteers, mostly through coding agents; the repo contains no secrets and no real community data.

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
- `internal/who` — the directory app: model load, handlers, admin tools, self-service edits
- `internal/blob` — media from Cloud Storage, held in memory with stored thumbnails
- `internal/geocode` — address → coordinates for the map
- `internal/capture` — drive Chrome and screenshot a page, for the dev tools
- `internal/devtls` — the in-memory self-signed certificate local HTTPS runs on
- `web/` — one directory per app (`web/who/`) holding its page templates and every file it serves, plus `web/common/` for what all apps share (frameworkless JavaScript)
- `sampledata/` — the fictional community served by default
- `cmd/` — dev tooling: screenshots, browser driving, sheet inspection
- `docs/` — everything below

## Docs

Platform:

- [goals.md](docs/goals.md) — what this is and the principles behind it
- [dev.md](docs/dev.md) — local development, including real-data mode
- [screenshots.md](docs/screenshots.md) — page capture for humans and agents
- [deploy.md](docs/deploy.md) — production deployment
- [plan.md](docs/plan.md) — what remains to build

Helios Who?, in `docs/who/`:

- [directory.md](docs/who/directory.md) — the directory app spec
- [data.md](docs/who/data.md) — the directory data model and load pipeline
- [design.md](docs/who/design.md) — palette, typography, brand
- [pwa.md](docs/who/pwa.md) — installable-app wiring

Each app keeps its own docs under `docs/<app>/`; the top level is only what every app shares.
