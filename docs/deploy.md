# Production deployment

The server runs as Cloud Run service `heliosian` in project `heliosian`, region `us-west1`, at https://heliosian-489539474126.us-west1.run.app, with https://who2.heliosian.com mapped on top. Administration is driven with the gcloud CLI (`brew install --cask gcloud-cli`) authenticated as a project owner (`gcloud auth login`).

## Build and deploy pipeline

Every push to `main` on `github.com/heliosian/heliosian` deploys production. The Cloud Build trigger `build-main` (region `us-west1`, attached through the `github` connection) carries its build config inline — nothing in the repo — and runs three steps:

1. `docker build` of the repo's Dockerfile, tagged with the commit SHA and `latest`
2. push the SHA tag to Artifact Registry (`us-west1-docker.pkg.dev/heliosian/heliosian/heliosian`)
3. `gcloud run deploy heliosian --image …:<sha>` — image only; every other service setting persists from the configuration below

The trigger names its build service account (the project's default compute service account) explicitly; trigger creation in this project refuses to infer one. Builds ship the pushed commit, never a working tree. To rebuild and redeploy current `main` without a push:

    gcloud builds triggers run build-main --region=us-west1 --branch=main

The Dockerfile builds in two stages: a `golang` stage compiles the static binary (`CGO_ENABLED=0`), and `gcr.io/distroless/static-debian12` — CA certificates and tzdata, nothing else — carries the binary plus `web/`, whose templates and static assets are read from disk at runtime. `sampledata/` is deliberately excluded: production always sets `DIRECTORY_SHEET`, and a misconfigured server fails at startup rather than silently serving sample data. `creds/` never enters the image (`.dockerignore`, `.gcloudignore`), and the image holds no credentials of any kind — the runtime identity below is the only Google identity in play.

## Service

The service was created once with:

    gcloud run deploy heliosian \
      --image us-west1-docker.pkg.dev/heliosian/heliosian/heliosian:latest \
      --region us-west1 --allow-unauthenticated \
      --service-account directory@heliosian.iam.gserviceaccount.com \
      --min-instances 1 --max-instances 1 --memory 4Gi --no-cpu-throttling \
      --set-env-vars DIRECTORY_SHEET=<spreadsheet id>,GOOGLE_CLIENT_ID=<oauth client id> \
      --set-secrets "SESSION_KEY=heliosian-session-key:latest,GOOGLE_MAPS_SERVER_KEY=heliosian-geocoding-key:latest,GOOGLE_MAPS_BROWSER_KEY=heliosian-maps-browser-key:latest" \
      --quiet

Each flag is load-bearing:

- `--service-account` — the runtime identity. Application-default credentials inside the container resolve to `directory@` through the metadata server; there is no key file anywhere in the system.
- `--min-instances 1` — startup preloads every media file from Drive before listening (about two and a half minutes all told); far too slow for scale-to-zero, and close enough to Cloud Run's four-minute startup probe window to watch as the media set grows.
- `--max-instances 1` — the directory model and blob store live in per-instance memory with no cross-instance coherency; a self-service edit refreshes only the instance that handled it, so a second instance would serve stale data.
- `--memory 4Gi` — the blob store holds all media and thumbnails in RAM, and the preload peaks well above the steady-state footprint (a roughly 600 MB blob store OOMed a 2 GiB instance during preload). The startup log line `blob store: … MB in memory` reports the steady state; resize when boots start failing or the footprint approaches half the limit.
- `--no-cpu-throttling` — the directory model and blob store refresh on five-minute tickers between requests; default throttling would starve them.
- `--allow-unauthenticated` — the app enforces its own Google sign-in; Cloud Run must let everyone reach the login page.

Cloud Run injects `PORT`; the server honors it.

## Configuration

Plain environment variables:

- `DIRECTORY_SHEET` — the production spreadsheet id: the `Directory` sheet living in the community shared drive.
- `GOOGLE_CLIENT_ID` — the OAuth web client id; not a secret (it is embedded in the login page).

Secret Manager secrets, delivered as environment variables per `--set-secrets` above. Values are used raw, so payloads must not carry trailing newlines:

- `heliosian-session-key` — session-cookie HMAC key (any long random string); losing or rotating it signs everyone out
- `heliosian-geocoding-key` — the Geocoding API server key, mirrored locally as `creds/geocoding.key`
- `heliosian-maps-browser-key` — the Maps JavaScript browser key, mirrored locally as `creds/maps.key`

## IAM

`directory@heliosian.iam.gserviceaccount.com` is both the data identity and the runtime identity:

- Data access: content manager on the community shared drive — which covers editing the `Directory` sheet inside it (self-service edits write cells and append to the Change Log tab) and managing media (uploads create files and archive old versions). Shared in Drive directly, never through project IAM.
- Runtime: the Cloud Run service runs as it, and it holds Secret Manager Secret Accessor on each secret individually.
- Humans: `roles/iam.serviceAccountTokenCreator` on this account enables the local impersonation that real-data development uses (`docs/dev.md`).

The build service account (the default compute service account) holds `cloudbuild.builds.builder`, `run.developer`, and Service Account User on `directory@` — the last because deploying a service that runs as an account requires permission to act as it.

The `heliosian.com` organization ships Google's secure-by-default org policies, two of which matter here:

- `iam.managed.disableServiceAccountKeyCreation` stays enforced. The keyless design depends on it staying enforced: no key for `directory@` can exist.
- `iam.allowedPolicyMemberDomains` is overridden to allow-all at the project scope only, because `--allow-unauthenticated` requires granting `run.invoker` to `allUsers`.

## OAuth

The consent screen lives in this project, audience External and published to production. Internal is not an option: the app restricts sign-in to `heliosschool.org` accounts (`internal/auth`), and those live outside the `heliosian.com` org. Published-External keeps the basic sign-in scopes free of verification friction.

The web client's authorized JavaScript origins are `http://localhost:8080`, the run.app URL, and `https://who2.heliosian.com`; sign-in fails on any origin not listed, and edits take a few minutes to propagate. The browser maps key is rendered into every page, so it carries an HTTP-referer restriction for the same three origins.

## Domain

`who2.heliosian.com` is a Cloud Run domain mapping on the service. DNS carries `who2 CNAME ghs.googlehosted.com.`; Google provisions and renews the certificate once the record resolves. The run.app URL stays live alongside it.

## Verifying a deploy

The startup log (Cloud Run → Logs, or `gcloud logging read`) shows the full boot sequence: the blob store footprint line, geocoding count, directory model load, then `listening` — about two and a half minutes after the instance starts. After any deploy, an existing session should still work — if everyone got signed out, `SESSION_KEY` stopped reaching the server.
