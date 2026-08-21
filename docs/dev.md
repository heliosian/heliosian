# Local development

## Run

    go run .

http://localhost:8080 (override with `PORT`). With `DIRECTORY_SHEET` unset the server serves the fictional community in `sampledata/`, signs every request in as a sample parent, and geocodes with a deterministic fake — no credentials or configuration. Templates, static assets, and sample data are read from disk on every request; edit a file and refresh.

Sample-mode limits: the map section needs a real Maps JavaScript key (`GOOGLE_MAPS_BROWSER_KEY`) to render tiles, and self-service edits and media uploads need real-data mode — there is no writable backend or blob store behind the sample CSVs.

`sampledata/` mirrors the production Sheets layout: one directory per app, one CSV per table, first row is the schema, served through the same data-source interface the Sheets backend implements. It stays fictional — real community data never goes here.

In sample mode `tools/screenshot` captures pages directly, no session needed (see `docs/screenshots.md`).

## Real data

    DIRECTORY_SHEET=<spreadsheet id> go run .

serves from the production spreadsheet and media bucket (see `docs/data.md`) and turns on the full stack. The model loads at startup — the server refuses to start if the load fails — and reloads every five minutes. Real data never leaves the process: nothing is written to disk. Requirements:

- **Sign-in** — everything sits behind Google sign-in restricted to the school's Workspace domain (API paths get a 401 instead of the login page). The OAuth web client is read from `creds/oauth-client.json` or `GOOGLE_CLIENT_ID`; its authorized JavaScript origins must include `http://localhost:8080`. The server issues its own HMAC-signed session cookie; set `SESSION_KEY` to keep sessions valid across restarts.
- **Data access** — Google credentials come from application-default credentials impersonating the data service account, set up once per machine:

      gcloud auth application-default login --impersonate-service-account=directory@heliosian.iam.gserviceaccount.com

  This requires `roles/iam.serviceAccountTokenCreator` on that account. `directory@` reaches the `Directory` sheet as a content manager of the community shared drive, and the media bucket through project IAM; local processes then act as exactly the identity production runs as, with no key file anywhere (the org forbids creating one).
- **Maps** — a server key for the Geocoding API (`creds/geocoding.key` or `GOOGLE_MAPS_SERVER_KEY`; never rendered into pages, restrict by server IP or leave unrestricted for dev) and a browser key for the Maps JavaScript API (`creds/maps.key` or `GOOGLE_MAPS_BROWSER_KEY`; rendered into pages, restrict by HTTP referer). Geocoding results are cached in memory per address.

To capture authenticated real-data pages, launch the capture browser (`go run ./tools/capturebrowser`), sign in to the local server there once, and use `tools/browse` or `tools/screenshot -remote` — the session cookie lives in the capture profile.

## Setup

Development happens on macOS. Two Homebrew installs cover everything here and in `docs/screenshots.md`:

    brew install go
    brew install --cask google-chrome

- **Go** 1.27 or later — builds and runs the server and all tooling (`go run`, `go vet`).
- **Google Chrome** — launched headless by the screenshot tool from its standard install location; never opened by hand.

No Node, no Docker, and no cloud credentials are needed for local development. Repository layout is in the README.

## Tools

Each runs as `go run ./tools/<name>`. The sheet, drive, and bucket tools authenticate with the same impersonated application-default credentials as the server (see Real data).

- `screenshot`, `capturebrowser`, `browse` — page capture and browser driving; see `docs/screenshots.md`
- `deploy` — apply the full production service configuration (needs `DIRECTORY_SHEET`); see `docs/deploy.md`
- `startserver` — launch the app detached, wait for it to listen, print the pid, log path, and a minted session cookie (needs `SESSION_KEY` and `DIRECTORY_SHEET`)
- `cookie` — print a signed session cookie for local API testing
- `loadcheck` — run the full load pipeline against a sheet and print a model summary
- `columns` — print each directory table's column names from the configured source
- `findsheet` — list spreadsheets visible to the service account
- `sheets` — dump a sheet's tabs, sizes, and header rows
- `dumptab` / `writetab` — copy one tab to a local CSV / write a local CSV into a tab, header-checked
- `createtabs` — create the directory sheet's local-layer tabs with their header rows
- `setcell` — set one cell in a tab by key column, appending the row if missing
- `splash` — regenerate the iOS splash battery from the captured original page; see `docs/pwa.md`
