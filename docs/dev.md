# Local development

## Run

    go run ./cmd/startserver

Every app answers at `https://<app>.local.heliosian.com:8080`, under the same name as its production hostname (override the port with `PORT`), clicking past the browser's warning about the self-signed certificate the first time. The dev server serves the fictional community in `sampledata/`, signs every request in as a sample parent, and geocodes with a deterministic fake — no credentials or configuration. All sample-mode composition lives in `cmd/startserver`; the production binary and the shared wiring in `internal/app` carry none of it. Templates and served files are read from disk on every request; edit a file and refresh. The sample CSVs back a fake spreadsheet that loads each tab on first read and keeps every write in memory, so `sampledata/` never changes on disk — restart to get the fixtures back.

## Hosts and files

One binary serves every app, and the hostname picks the app by convention: `<app>.heliosian.com` is production, `<app>.lab.heliosian.com` is the same service under a second name, and `<app>.local.heliosian.com` is a developer's machine, so `who.heliosian.com`, `who.lab.heliosian.com`, and `who.local.heliosian.com` all mean the directory. Home additionally answers as `heliosian.com` and `www.heliosian.com`, the volunteer portal (`team`) as `hca` on every tier, and any other host gets a 404 (`internal/app`). Public DNS points `*.local.heliosian.com` at loopback, so local development uses real hostnames under it with no hosts-file entry, and the OAuth client and Maps key can name them (Google refuses made-up domains such as `.localhost`).

## Local HTTPS

The dev server speaks HTTPS in every mode. Google sign-in only accepts a plain-HTTP login URI on `localhost` itself, and the dev hostname is a real name, so it has to be HTTPS. `internal/devtls` mints a self-signed certificate for `*.local.heliosian.com` in memory every time the dev server starts; nothing is written to disk. Browsers warn about it; click through. Headless captures pass `--ignore-certificate-errors`; the capture browser needs the warning clicked once in its own profile. The production binary itself stays plain HTTP, because Cloud Run terminates TLS in front of it.

Within an app, a request resolves in this order: `web/public/<app>/<path>`, then `web/public/common/<path>`, served to anyone with no sign-in; then sign-in; then `web/<app>/<path>`, then `web/common/<path>`; then the app's routes. Public URLs are the file's own path, with no prefix: `/style.css`, `/brand/icon-192.png`, `/fonts/fonts.css`. The public tree holds exactly what a browser fetches without credentials and what the sign-in and no-access pages need: the manifest, icons, splash art, login art, fonts, the login page itself, and the no-access page the member gate serves to anyone the directory does not list. Everything else, every page, script, style, image, and API, requires a session; there are no other exceptions.

Pages are plain files, not templates. The login page learns the OAuth client id from `/auth/client`, one of the two endpoints (with the login POST) reachable before sign-in, so the id lives only in the server's configuration, and builds its login URI from its own origin; the directory shell learns who is signed in from the model it fetches. Shared browser code goes in `web/common/` (or `web/public/common/` when it must load before sign-in) and is reached by every app at the same path.

Sample-mode limits: the map section needs a real Maps JavaScript key (`GOOGLE_MAPS_BROWSER_KEY`) to render tiles, and self-service edits and media uploads need real-data mode — those handlers register alongside the blob store, which the sample CSVs have no counterpart for. Tags do work, since they need only the writable data source.

`sampledata/` mirrors the production Sheets layout: one directory per spreadsheet, named as the server's data source names it, one CSV per tab, first row is the schema, served through the same data-source interface the Sheets backend implements. It stays fictional — real community data never goes here. `sampledata/directory/Website Staff Import.csv` covers what the staff page layer has to resolve: a bio an override outranks, a title Veracross outranks, an entry with no address matched by name through `Name to Email`, another matched by name against the staff import, an entry under a second address resolved through `Email Aliases`, and entries matching nobody — a vendor the directory drops, and a staff member added by hand, whom the layer runs too early to see. `sampledata/preferences/` is the consent form's response sheet, exercising every combination the loader has to resolve: both permissions, each alone, neither, a family that never answered, a staff member's own opt-out, a superseded submission, and a two-household student whose parents answered differently.

In sample mode `go run ./cmd/startserver --capture <url> --out <png>` serves and screenshots one page of either app in a single command, and `cmd/screenshot` captures against an already-running server (see `docs/screenshots.md`).

## Real data

    eval "$(go run ./cmd/findsheet)"
    SESSION_KEY=<secret> go run .

runs the production binary: the production spreadsheets, the media bucket (see `docs/who/data.md`), real geocoding, and Google sign-in, with no sample fallback — every input is required and the server refuses to start without one. `cmd/findsheet` prints every spreadsheet id variable the server needs, found among the sheets the service account can see. The model loads at startup — the server refuses to start if the load fails — and reloads every five minutes. Real data never leaves the process: nothing is written to disk. Requirements:

- **Sign-in** — everything sits behind Google sign-in restricted to the school's Workspace domain (API paths get a 401 instead of the login page). The OAuth web client id is read from `creds/oauth-client.json` or `GOOGLE_CLIENT_ID` and handed to the login page by `/auth/client`; the client's authorized JavaScript origins must include `https://who.local.heliosian.com:8080`. The server issues its own HMAC-signed session cookie, keyed by the required `SESSION_KEY`.
- **Data access** — Google credentials come from application-default credentials impersonating the data service account, set up once per machine:

      gcloud auth application-default login --impersonate-service-account=directory@heliosian.iam.gserviceaccount.com

  This requires `roles/iam.serviceAccountTokenCreator` on that account. `directory@` reaches the `Directory` sheet as a content manager of the community shared drive, and the media bucket through project IAM; local processes then act as exactly the identity production runs as, with no key file anywhere (the org forbids creating one).
- **Mail** — in sample mode the volunteer portal writes each email it would send as an `.html` file under `MAIL_DIR` (default `$TMPDIR/hca-mail`); open one in a browser to see it. Real-data mode sends through Resend when `creds/resend.key` (or `RESEND_KEY`) is present, else over SMTP when `SMTP_HOST` is set (with `SMTP_USER` and `creds/smtp.pass` or `SMTP_PASS`), else drops and logs.
- **Image search** — the volunteer portal's image picker ("Find an image") searches Unsplash when `creds/unsplash.key` (or `UNSPLASH_KEY`, an Unsplash API access key - a demo app's 50 requests an hour is plenty) is present, with the photographer credited on each tile and the photo's download endpoint pinged on import as Unsplash's terms ask; likewise Pexels (`creds/pexels.key` / `PEXELS_KEY`) and Pixabay (`creds/pixabay.key` / `PIXABAY_KEY`, photos plus illustrations and vectors); and always Wikimedia Commons through the server: no key, free-to-use pictures with their licence on each tile, and a picked image is fetched and stored like an upload. Set `creds/search.key` + `creds/search.cx` (or `GOOGLE_SEARCH_KEY` + `GOOGLE_SEARCH_CX`) and it searches a Google Programmable Search Engine through the Custom Search JSON API instead - but note Google has stopped granting new projects access to that API (`This project does not have the access to Custom Search JSON API`) and retired whole-web engines, so Commons is the expected path.
- **Maps** — a server key for the Geocoding API (`creds/geocoding.key` or `GOOGLE_MAPS_SERVER_KEY`; never rendered into pages, restrict by server IP or leave unrestricted for dev) and a browser key for the Maps JavaScript API (`creds/maps.key` or `GOOGLE_MAPS_BROWSER_KEY`; rendered into pages, restrict by HTTP referer). Geocoding results are cached in memory per address.

To capture authenticated real-data pages, launch the capture browser (`go run ./cmd/capturebrowser`), sign in to the local server there once, and use `cmd/browse` or `cmd/screenshot --remote` — the session cookie lives in the capture profile.

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
- `startserver` — the dev server: sample data in the foreground by default, `--capture` for a one-command page screenshot, `--real` for the production assembly in the foreground, and `--detach` to launch that in the background with its output in a log file plus a minted session cookie (needs `SESSION_KEY` and every spreadsheet id)
- `cookie` — print a signed session cookie for local API testing
- `loadcheck` — run the full load pipeline against the directory and preferences sheets and print a model summary
- `findsheet` — print the spreadsheet ids the server needs as shell exports, found by the sheets' titles, with every other visible spreadsheet as a comment
- `sheets` — dump a sheet's tabs, sizes, and header rows
- `dumptab` / `writetab` — copy one tab to a local CSV / write a local CSV into a tab, header-checked
- `createtabs` — create the tabs of every spreadsheet that has a layout with their header rows, adding missing columns to tabs that exist (needs each of those spreadsheets' id variables, and refuses a spreadsheet whose title isn't the layout the variable promised)
- `setcell` — set one cell in a tab by key column, appending the row if missing
- `fixdates` — rewrite Glide-style `…T17:30:00.000Z` timestamps in the Events sheet into the loader's `2025-03-01 17:30` / `2025-03-01` forms (needs `EVENTS_SHEET`; dry run unless `-write`)
- `import` — run a fresh Veracross export and a fresh export of the school website's staff page, upload the portraits from both, sync the import tabs, and clear the overrides those imports have caught up with, or report what that would change with `--dry-run` (needs `DIRECTORY_SHEET`, `PREFERENCES_SHEET`, and `CONFIG_SHEET`, and a `vcexport` and a `webexport` checkout, found at `../vcexport` and `../webexport` or wherever `VCEXPORT` and `WEBEXPORT` point)
- `birthdayimport` — load the Glide birthday app's table exports in `imports/` into the empty `Birthdays` spreadsheet (needs `BIRTHDAY_SHEET`); see `docs/birthday/data.md`
- `splash` — download an app's iOS splash battery from its captured Glide manifest into a brand directory; see `docs/who/pwa.md`
