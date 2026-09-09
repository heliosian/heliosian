# Local development

## Run

    go run ./cmd/startserver

https://who.local.heliosian.com:8080 (override the port with `PORT`), clicking past the browser's warning about the self-signed certificate the first time. The dev server serves the fictional community in `sampledata/`, signs every request in as a sample parent, and geocodes with a deterministic fake — no credentials or configuration. All sample-mode composition lives in `cmd/startserver`; the production binary and the shared wiring in `internal/app` carry none of it. Templates and served files are read from disk on every request; edit a file and refresh. The sample CSVs back a fake spreadsheet that loads each tab on first read and keeps every write in memory, so `sampledata/` never changes on disk — restart to get the fixtures back.

## Hosts and files

One binary serves every app, and the hostname picks the app: `internal/app` holds the table of hostnames the service answers to (`who.heliosian.com`, the run.app URL, and `who.local.heliosian.com` all mean the directory), and any other host gets a 404. Public DNS points `*.local.heliosian.com` at loopback, so local development uses real hostnames under it with no hosts-file entry, and the OAuth client and Maps key can name them (Google refuses made-up domains such as `.localhost`).

## Local HTTPS

The dev server speaks HTTPS in every mode. Google sign-in only accepts a plain-HTTP login URI on `localhost` itself, and the dev hostname is a real name, so it has to be HTTPS. `internal/devtls` mints a self-signed certificate for `*.local.heliosian.com` in memory every time the dev server starts; nothing is written to disk. Browsers warn about it; click through. Headless captures pass `--ignore-certificate-errors`; the capture browser needs the warning clicked once in its own profile. The production binary itself stays plain HTTP, because Cloud Run terminates TLS in front of it.

Within an app, a request first looks for a file: `web/<app>/<path>`, then `web/common/<path>`. A regular file there is served as-is, public, with no sign-in — which is what the manifest, icons, fonts, scripts, and styles need, since browsers fetch some of them without credentials. Anything else falls through to the app's routes behind sign-in. Public URLs are the file's own path, with no prefix: `/style.css`, `/brand/icon-192.png`, `/fonts/fonts.css`. Pages are templates rendered by their handlers, never served raw, so `.html` is the one thing the file lookup skips; the templates sit in `web/<app>/` alongside the files. Shared browser code goes in `web/common/` and is reached by every app at the same path.

Sample-mode limits: the map section needs a real Maps JavaScript key (`GOOGLE_MAPS_BROWSER_KEY`) to render tiles, and self-service edits and media uploads need real-data mode — those handlers register alongside the blob store, which the sample CSVs have no counterpart for. Tags do work, since they need only the writable data source.

`sampledata/` mirrors the production Sheets layout: one directory per app, one CSV per table, first row is the schema, served through the same data-source interface the Sheets backend implements. It stays fictional — real community data never goes here. `sampledata/preferences/` is the consent form's response sheet, exercising every combination the loader has to resolve: both permissions, each alone, an opt-out, a superseded submission, and a two-household student whose parents answered differently.

In sample mode `go run ./cmd/startserver -capture <path> -out <png>` serves and screenshots one page in a single command, and `cmd/screenshot` captures against an already-running server (see `docs/screenshots.md`).

## Real data

    DIRECTORY_SHEET=<spreadsheet id> PREFERENCES_SHEET=<spreadsheet id> SESSION_KEY=<secret> go run .

runs the production binary: the production spreadsheets, the media bucket (see `docs/who/data.md`), real geocoding, and Google sign-in, with no sample fallback — every input is required and the server refuses to start without one. The model loads at startup — the server refuses to start if the load fails — and reloads every five minutes. Real data never leaves the process: nothing is written to disk. Requirements:

- **Sign-in** — everything sits behind Google sign-in restricted to the school's Workspace domain (API paths get a 401 instead of the login page). The OAuth web client is read from `creds/oauth-client.json` or `GOOGLE_CLIENT_ID`; its authorized JavaScript origins must include `https://who.local.heliosian.com:8080`. The server issues its own HMAC-signed session cookie, keyed by the required `SESSION_KEY`.
- **Data access** — Google credentials come from application-default credentials impersonating the data service account, set up once per machine:

      gcloud auth application-default login --impersonate-service-account=directory@heliosian.iam.gserviceaccount.com

  This requires `roles/iam.serviceAccountTokenCreator` on that account. `directory@` reaches the `Directory` sheet as a content manager of the community shared drive, and the media bucket through project IAM; local processes then act as exactly the identity production runs as, with no key file anywhere (the org forbids creating one).
- **Maps** — a server key for the Geocoding API (`creds/geocoding.key` or `GOOGLE_MAPS_SERVER_KEY`; never rendered into pages, restrict by server IP or leave unrestricted for dev) and a browser key for the Maps JavaScript API (`creds/maps.key` or `GOOGLE_MAPS_BROWSER_KEY`; rendered into pages, restrict by HTTP referer). Geocoding results are cached in memory per address.

To capture authenticated real-data pages, launch the capture browser (`go run ./cmd/capturebrowser`), sign in to the local server there once, and use `cmd/browse` or `cmd/screenshot -remote` — the session cookie lives in the capture profile.

## Setup

Development happens on macOS. Two Homebrew installs cover everything here and in `docs/screenshots.md`:

    brew install go
    brew install --cask google-chrome

- **Go** 1.27 or later — builds and runs the server and all tooling (`go run`, `go vet`).
- **Google Chrome** — launched headless by the screenshot tool from its standard install location; never opened by hand.

No Node, no Docker, and no cloud credentials are needed for local development. Repository layout is in the README.

## Tools

Each runs as `go run ./cmd/<name>`. The sheet, drive, and bucket tools authenticate with the same impersonated application-default credentials as the server (see Real data).

- `screenshot`, `capturebrowser`, `browse` — page capture and browser driving; see `docs/screenshots.md`
- `deploy` — apply the full production service configuration (needs `DIRECTORY_SHEET`); see `docs/deploy.md`
- `startserver` — the dev server: sample data in the foreground by default, `-capture` for a one-command page screenshot, `-real` for the production assembly in the foreground, and `-detach` to launch that in the background with its output in a log file plus a minted session cookie (needs `SESSION_KEY` and `DIRECTORY_SHEET`)
- `cookie` — print a signed session cookie for local API testing
- `loadcheck` — run the full load pipeline against the directory and preferences sheets and print a model summary
- `findsheet` — list spreadsheets visible to the service account
- `sheets` — dump a sheet's tabs, sizes, and header rows
- `dumptab` / `writetab` — copy one tab to a local CSV / write a local CSV into a tab, header-checked
- `createtabs` — create the directory sheet's local-layer tabs with their header rows
- `setcell` — set one cell in a tab by key column, appending the row if missing
- `import` — run a fresh Veracross export, upload its portraits, and sync the import tabs, or report what that would change with `-dry-run` (needs `DIRECTORY_SHEET` and `PREFERENCES_SHEET`, and a `vcexport` checkout)
- `splash` — regenerate the iOS splash battery from the captured original page; see `docs/who/pwa.md`
