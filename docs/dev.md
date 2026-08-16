# Local development

## Run

    go run .

http://localhost:8080 (override with `PORT`). With `DIRECTORY_SHEET` unset the server serves the fictional community in `sampledata/`, signs every request in as a sample parent, and geocodes with a deterministic fake — no credentials or configuration. Templates, static assets, and sample data are read from disk on every request; edit a file and refresh.

Sample-mode limits: the map section needs a real Maps JavaScript key (`GOOGLE_MAPS_BROWSER_KEY`) to render tiles, and self-service edits and media uploads need real-data mode — there is no writable backend or blob store behind the sample CSVs.

`sampledata/` mirrors the production Sheets layout: one directory per app, one CSV per table, first row is the schema, served through the same data-source interface the Sheets backend implements. It stays fictional — real community data never goes here.

In sample mode `tools/screenshot` captures pages directly, no session needed (see `docs/screenshots.md`).

## Real data

    DIRECTORY_SHEET=<spreadsheet id> go run .

serves from the production spreadsheet and media drive (see `docs/data.md`) and turns on the full stack. The model loads at startup — the server refuses to start if the load fails — and reloads every five minutes. Real data never leaves the process: nothing is written to disk. Requirements:

- **Sign-in** — everything sits behind Google sign-in restricted to the school's Workspace domain (API paths get a 401 instead of the login page). The OAuth web client is read from `creds/oauth-client.json` or `GOOGLE_CLIENT_ID`; its authorized JavaScript origins must include `http://localhost:8080`. The server issues its own HMAC-signed session cookie; set `SESSION_KEY` to keep sessions valid across restarts.
- **Service account** — key at `creds/service-account.json` (the directory is gitignored), Sheets API enabled, the spreadsheet shared with it as editor (self-service edits write cells and append to the Change Log tab), the media shared drive shared as content manager (uploads create files and archive old versions).
- **Maps** — a server key for the Geocoding API (`creds/geocoding.key` or `GOOGLE_MAPS_SERVER_KEY`; never rendered into pages, restrict by server IP or leave unrestricted for dev) and a browser key for the Maps JavaScript API (`creds/maps.key` or `GOOGLE_MAPS_BROWSER_KEY`; rendered into pages, restrict by HTTP referer). Geocoding results are cached in memory per address.

To capture authenticated real-data pages, launch the capture browser (`go run ./tools/capturebrowser`), sign in to the local server there once, and use `tools/browse` or `tools/screenshot -remote` — the session cookie lives in the capture profile.

## Setup

Development happens on macOS. Two Homebrew installs cover everything here and in `docs/screenshots.md`:

    brew install go
    brew install --cask google-chrome

- **Go** 1.26 or later — builds and runs the server and all tooling (`go run`, `go vet`).
- **Google Chrome** — launched headless by the screenshot tool from its standard install location; never opened by hand.

No Node, no Docker, and no cloud credentials are needed for local development. Repository layout is in the README.
