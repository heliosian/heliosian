# Heliosian

Web apps for the Helios school community (K-8), served as one static Go binary on Cloud Run. The first app is the school directory, "Helios Who?". Built in the open by community volunteers, mostly through coding agents; the repo contains no secrets and no real community data.

## Quick start

    brew install go
    go run .

Open http://localhost:8080. That's the whole setup: with no configuration the server loads the fictional community in `sampledata/`, signs every request in as a sample parent, and fakes geocoding — no credentials, no cloud project. Templates, static assets, and sample data are read from disk on every request, so edit a file and refresh; nothing needs restarting.

`brew install --cask google-chrome` additionally enables the screenshot tooling used to verify visual changes ([docs/screenshots.md](docs/screenshots.md)). No Node, no Docker. Go 1.26 or later.

To run against real community data instead, see [docs/dev.md](docs/dev.md).

## Layout

- `main.go` — entry point and app wiring
- `internal/auth` — Google sign-in and session cookies
- `internal/data` — tabular data sources: sample CSVs and Google Sheets
- `internal/directory` — the directory app: model load, handlers, self-service edits
- `internal/blob` — media from Cloud Storage, held in memory with stored thumbnails
- `internal/geocode` — address → coordinates for the map
- `web/` — page templates and static assets (frameworkless JavaScript)
- `sampledata/` — the fictional community served by default
- `tools/` — dev tooling: screenshots, browser driving, sheet inspection
- `docs/` — everything below

## Docs

- [goals.md](docs/goals.md) — what this is and the principles behind it
- [dev.md](docs/dev.md) — local development, including real-data mode
- [data.md](docs/data.md) — the directory data model and load pipeline
- [directory.md](docs/directory.md) — the directory app spec
- [design.md](docs/design.md) — palette, typography, brand
- [pwa.md](docs/pwa.md) — installable-app wiring
- [screenshots.md](docs/screenshots.md) — page capture for humans and agents
- [deploy.md](docs/deploy.md) — production deployment
- [plan.md](docs/plan.md) — what remains to build
