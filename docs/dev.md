# Local development

Toolchain prerequisites: see [setup.md](setup.md).

## Run

    go run .

The server listens on http://localhost:8080 (override with `PORT`). Templates, static assets, and sample data are read from disk on every request — edit a file and refresh the browser; no restart needed.

## Auth

Everything — pages, static assets, and the API — sits behind Google sign-in restricted to the school's Google Workspace domain. Unauthenticated requests get the login page (API paths get a 401). The OAuth 2.0 Web application client (authorized JavaScript origins must include `http://localhost:8080` for local development) is read from `creds/oauth-client.json` — the JSON downloaded from the Cloud console — or from `GOOGLE_CLIENT_ID` when set; the server refuses to start with neither. After Google sign-in the server issues its own HMAC-signed session cookie; set `SESSION_KEY` to keep sessions valid across restarts and instances (without it each start generates a random key).

To capture authenticated pages with the screenshot tooling, launch the capture browser (`go run ./tools/capturebrowser`), sign in to the local server there once, and use `tools/browse` or `tools/screenshot -remote` — the session cookie lives in the capture profile. Plain `tools/screenshot` runs a fresh headless browser with no session and captures the login page.

## Local data

The server reads local data from `sampledata/`, mirroring the production Sheets layout: one directory per app, one CSV file per table, first row is the schema. It goes through the same data-source interface production backends implement, so app code never knows which backend it is talking to.

## Real data

    DIRECTORY_SHEET=<spreadsheet id> go run .

switches the directory app to the Google Sheets source. At startup the directory tables are read from the spreadsheet and normalized into the in-memory data model (see `docs/data.md`); the server refuses to start if that load fails, and the model reloads every five minutes. Requires the service account key at `creds/service-account.json` (the directory is gitignored) with the Sheets API enabled and the spreadsheet shared read-only with the service account. Real data never leaves the process: nothing is written to disk.

## Layout

- `main.go` — server entry point and app routing
- `internal/data` — data source interface and the CSV sample-data implementation
- `internal/directory` — directory app handlers
- `web/directory` — directory app page templates and static assets
- `tools/screenshot` — dev-site page capture, see [screenshots.md](screenshots.md)
