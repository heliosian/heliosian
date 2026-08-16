# Production deployment

The server runs as Cloud Run service `heliosian` in project `gen-lang-client-0758114984`, region `us-west1`, at https://heliosian-326077318680.us-west1.run.app. Everything below is driven with the gcloud CLI (`brew install --cask gcloud-cli`) authenticated as the deploy identity:

    gcloud auth activate-service-account --key-file=creds/service-account.json
    gcloud config set project gen-lang-client-0758114984

## Image

The Dockerfile builds in two stages: a `golang` stage compiles the static binary (`CGO_ENABLED=0`), and `gcr.io/distroless/static-debian12` — CA certificates and tzdata, nothing else — carries the binary plus `web/`, whose templates and static assets are read from disk at runtime. `sampledata/` is deliberately excluded: production always sets `DIRECTORY_SHEET`, and a misconfigured server fails at startup rather than silently serving sample data. `creds/` never enters the image or the source upload (`.dockerignore`, `.gcloudignore`).

Cloud Build produces the image into Artifact Registry. Builds ship git HEAD, never the working tree — building the live tree can capture files mid-edit:

    BUILDDIR=$(mktemp -d)
    git archive HEAD | tar -x -C "$BUILDDIR"
    gcloud builds submit --tag us-west1-docker.pkg.dev/gen-lang-client-0758114984/heliosian/heliosian "$BUILDDIR"

## Service

    gcloud run deploy heliosian \
      --image us-west1-docker.pkg.dev/gen-lang-client-0758114984/heliosian/heliosian:latest \
      --region us-west1 --allow-unauthenticated \
      --min-instances 1 --memory 2Gi --no-cpu-throttling \
      --set-env-vars DIRECTORY_SHEET=<spreadsheet id>,GOOGLE_CLIENT_ID=<oauth client id> \
      --set-secrets "/app/creds/service-account.json=heliosian-sa-key:latest,SESSION_KEY=heliosian-session-key:latest,GOOGLE_MAPS_SERVER_KEY=heliosian-geocoding-key:latest,GOOGLE_MAPS_BROWSER_KEY=heliosian-maps-browser-key:latest" \
      --quiet

Each flag is load-bearing:

- `--min-instances 1` — startup preloads every media file from Drive before listening (about 90 seconds); far too slow for scale-to-zero.
- `--memory 2Gi` — the blob store holds all media and thumbnails in RAM. The startup log line `blob store: … MB in memory` reports the footprint; resize when it approaches the limit.
- `--no-cpu-throttling` — the directory model and blob store refresh on five-minute tickers between requests; default throttling would starve them.
- `--allow-unauthenticated` — the app enforces its own Google sign-in; Cloud Run must let everyone reach the login page.

Cloud Run injects `PORT`; the server honors it.

## Configuration

Plain environment variables:

- `DIRECTORY_SHEET` — the production spreadsheet id. The sheet is the single spreadsheet shared with the service account.
- `GOOGLE_CLIENT_ID` — the OAuth web client id; not a secret (it is embedded in the login page). The client secret from `creds/oauth-client.json` is never used by the server and lives nowhere in production.

Secret Manager secrets, delivered per `--set-secrets` above:

- `heliosian-sa-key` — `creds/service-account.json`, mounted as a file at `/app/creds/service-account.json` (the exact path the server opens, relative to `/app`)
- `heliosian-session-key` — session-cookie HMAC key (any long random string); losing or rotating it signs everyone out
- `heliosian-geocoding-key` — `creds/geocoding.key`
- `heliosian-maps-browser-key` — `creds/maps.key`

## IAM

`heliosian-test@gen-lang-client-0758114984.iam.gserviceaccount.com` serves two unrelated purposes:

- Data access: the spreadsheet and the media shared drive are shared with it in Drive/Sheets directly — never through project IAM.
- Deploy identity: project roles Editor, Service Account User, Cloud Run Admin, and Secret Manager Admin. The extra roles exist because Editor cannot set IAM policy on services or secrets.

The runtime identity is the default compute service account (`326077318680-compute@developer.gserviceaccount.com`) holding Secret Manager Secret Accessor on each secret individually. The basic Editor role deliberately cannot read secret payloads, so these explicit grants are the only thing standing between the service and a startup failure — the console's inherited-role rows on a secret's Permissions tab do not imply payload access.

## OAuth

The service URL belongs in the OAuth client's authorized JavaScript origins alongside `http://localhost:8080`; sign-in fails on any origin not listed, and edits take a few minutes to propagate. The browser maps key is rendered into every page, so it carries an HTTP-referer restriction for the service URL and localhost (see `docs/dev.md`).

## Verifying a deploy

The startup log (Cloud Run → Logs, or `gcloud logging read`) shows the full boot sequence: geocoding count, directory model load, the blob store footprint line, then `listening`. After any deploy, an existing session should still work — if everyone got signed out, `SESSION_KEY` stopped reaching the server.
