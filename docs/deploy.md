# Production deployment

The server runs as Cloud Run service `heliosian` in project `heliosian`, region `us-west1`. The one service hosts every app, choosing by hostname: `<app>.heliosian.com` and `<app>.lab.heliosian.com` name the app, the apex and `www` name home, and any other hostname gets a 404 even if Cloud Run routes it here (`internal/app`). Administration is driven with the gcloud CLI (`brew install --cask gcloud-cli`) authenticated as a project owner (`gcloud auth login`).

## Build and deploy pipeline

Every push to `main` on `github.com/heliosian/heliosian` deploys production. The Cloud Build trigger `build-main` (region `us-west1`, attached through the `github` connection) carries its build config inline — nothing in the repo — and runs three steps:

1. `docker build` of the repo's Dockerfile, tagged with the commit SHA and `latest`
2. push the SHA tag to Artifact Registry (`us-west1-docker.pkg.dev/heliosian/heliosian/heliosian`)
3. `gcloud run deploy heliosian --image …:<sha>` — image only; every other service setting persists from the last full deploy

The trigger names its build service account (the project's default compute service account) explicitly; trigger creation in this project refuses to infer one. Builds ship the pushed commit, never a working tree. To rebuild and redeploy current `main` without a push:

    gcloud builds triggers run build-main --region=us-west1 --branch=main

The Dockerfile builds in two stages: a `golang` stage compiles the static binary (`CGO_ENABLED=0`), and `gcr.io/distroless/static-debian12` — CA certificates and tzdata, nothing else — carries the binary plus `web/`, whose templates and static assets are read from disk at runtime. `sampledata/` is deliberately excluded: production always sets `DIRECTORY_SHEET`, and a misconfigured server fails at startup rather than silently serving sample data. `creds/` never enters the image (`.dockerignore`, `.gcloudignore`), and the image holds no credentials of any kind — the runtime identity below is the only Google identity in play.

## Service configuration

`cmd/deploy` holds the full service configuration and is the only place it is written down:

    eval "$(go run ./cmd/findsheet)"
    go run ./cmd/deploy

It deploys the `latest` image with every setting the pipeline does not touch, so it both creates the service from nothing and repairs drift on an existing one. The spreadsheet ids come from the environment (`cmd/findsheet` prints them as shell exports) and the OAuth client id from `creds/oauth-client.json` — the same resolution the server itself uses — so none of them is written into the repository. Every spreadsheet id is required, here and in `Production()`. Because a per-push deploy only ever changes the image (see above), changing a spreadsheet id only ever takes effect through a `cmd/deploy` run, never a plain push.

Why each setting is what it is:

- **service account** — the runtime identity, `directory@`. Application-default credentials inside the container resolve to it through the metadata server; there is no key file anywhere in the system.
- **min instances 1** — startup fetches every media object the sheets name before listening (about ten seconds); still worth avoiding on a cold request.
- **Sheets quota** — the API allows 60 read requests per minute per user, and startup reads every tab of every sheet while the previous revision keeps refreshing; two deploys inside a minute, or a deploy on top of a burst of local `loadcheck`/`dumptab` runs, hit it, and a refused read at startup used to fail the revision (`load config: Error 429`). `internal/data/sheet.go` now waits out a 429 and retries (5, 10, 20, 40 seconds), within the 240-second startup window; a refused write is retried the same way rather than dropped.
- **max instances 1** — the directory model and blob store live in per-instance memory with no cross-instance coherency; a self-service edit refreshes only the instance that handled it, so a second instance would serve stale data.
- **memory 2Gi** — the blob store holds every named object and its thumbnail in RAM. Because thumbnails are stored rather than generated, startup decodes no images and the peak sits close to the steady state: a roughly 360 MB blob store runs the container at about a quarter of this limit. The startup log line `blob store: prefetched` reports how many objects are held; resize when the footprint approaches half the limit.
- **concurrency 250** — with a single instance, every simultaneous request shares it, and a photo-heavy page fans out many image requests at once. The work is served from memory, so the ceiling exists to bound queueing, not CPU.
- **no CPU throttling** — the directory model and blob store refresh on five-minute tickers between requests; default throttling would starve them.
- **HTTP/2** — Cloud Run speaks cleartext HTTP/2 to the container, which the server accepts. Browsers already get HTTP/2 from the frontend either way.
- **allow unauthenticated** — the app enforces its own Google sign-in; Cloud Run must let everyone reach the login page.

Cloud Run injects `PORT`; the server honors it.

## Configuration values

Plain environment variables:

- `DIRECTORY_SHEET` — the production spreadsheet id: the `Directory` sheet living in the community shared drive.
- `PREFERENCES_SHEET` — the `Preferences` sheet in the same shared drive: the sharing-consent form's response spreadsheet.
- `INVITES_SHEET` — the `Invite List Builder` spreadsheet id. Powers the Invites page (`/greenvelope`).
- `APPS_SHEET` — the `Apps` spreadsheet id: Heliosian's categories, links, and admins (`docs/home/data.md`).
- `EVENTS_SHEET` — the `Events` spreadsheet id: the volunteer portal's activities, roles, sign-ups, and admins (`docs/events/data.md`).
- `BIRTHDAY_SHEET` — the `Birthdays` spreadsheet id: the birthday team's staff birthdays, pipeline progress, charities, and admins (`docs/birthday/data.md`).
- `CONFIG_SHEET` — the `Config` spreadsheet id: the platform super admins and settings (`docs/config.md`).
- `GOOGLE_CLIENT_ID` — the OAuth web client id; not a secret (every login page fetches it from `/auth/client`), but kept out of the repository.

Secret Manager secrets, delivered as environment variables. Values are used raw, so payloads must not carry trailing newlines:

- `heliosian-session-key` — session-cookie HMAC key (any long random string); losing or rotating it signs everyone out
- `heliosian-geocoding-key` — the Geocoding API server key, mirrored locally as `creds/geocoding.key`
- `heliosian-maps-browser-key` — the Maps JavaScript browser key, mirrored locally as `creds/maps.key`

## Media storage

Photos, pronunciation recordings, Heliosian's link images, and the volunteer portal's activity images live in `gs://heliosian-media` (us-west1, uniform access, public access prevented, object versioning on), the single source of truth for media — see `docs/who/data.md` for the naming convention and stored thumbnails. It sits in the same region as the service, so everything the sheets name fetches into memory in about eight seconds with no per-request throttle to work around. Nothing ever lists the bucket: the sheets are the index, each loader asks for its objects by name, and an object no sheet has named for a while is dropped from memory. Object versioning no longer carries the upload history: an object is named for its own bytes and is never rewritten, so every version that once accumulated under one name is now a separate object that the sheet either still names or does not.

Because the bucket is private and every read goes through the app's own sign-in gate, no object is ever publicly readable; the service reads and writes it as `directory@`.

## IAM

`directory@heliosian.iam.gserviceaccount.com` is both the data identity and the runtime identity:

- Data access: content manager on the community shared drive, which covers editing the `Directory` sheet inside it (self-service edits write cells and append to the Change Log tab). Shared in Drive directly, never through project IAM.
- Media: `roles/storage.objectAdmin` on `gs://heliosian-media`, granted on the bucket.
- Runtime: the Cloud Run service runs as it, and it holds Secret Manager Secret Accessor on each secret individually.
- Humans: `roles/iam.serviceAccountTokenCreator` on this account enables the local impersonation that real-data development uses (`docs/dev.md`).

The build service account (the default compute service account) holds `cloudbuild.builds.builder`, `run.developer`, and Service Account User on `directory@` — the last because deploying a service that runs as an account requires permission to act as it.

The `heliosian.com` organization ships Google's secure-by-default org policies, two of which matter here:

- `iam.managed.disableServiceAccountKeyCreation` stays enforced. The keyless design depends on it staying enforced: no key for `directory@` can exist.
- `iam.allowedPolicyMemberDomains` is overridden to allow-all at the project scope only, because unauthenticated access requires granting `run.invoker` to `allUsers`.

## OAuth

The consent screen lives in this project, audience External and published to production. Internal is not an option: the app restricts sign-in to `heliosschool.org` accounts (`internal/auth`), and those live outside the `heliosian.com` org. Published-External keeps the basic sign-in scopes free of verification friction.

The web client's authorized JavaScript origins are the `*.local.heliosian.com` development hosts and each app's production and lab hostnames; sign-in fails on any origin not listed, and edits take a few minutes to propagate. A new app's hostnames go here as well as into the domain mappings below. Google rejects made-up domains such as `.localhost` here, which is why local development runs under a real subdomain that public DNS points at loopback, and it only accepts a plain-HTTP login URI on `localhost` itself, which is why the dev server serves HTTPS with a self-signed certificate (`docs/dev.md`). The browser maps key is rendered into every page, so it carries an HTTP-referer restriction of its own: `https://*.local.heliosian.com:8080/*` and `https://*.heliosian.com/*`, which covers every host the service answers to.

## Domain

Every hostname is a Cloud Run domain mapping on the one service, one per app per tier plus `www` and the apex. The current set, with each certificate's status:

    gcloud beta run domain-mappings list --region us-west1 --project heliosian --format "table(metadata.name,status.conditions[0].status)"

A new hostname is one more mapping:

    gcloud beta run domain-mappings create --service heliosian --domain <host>.heliosian.com --region us-west1 --project heliosian

DNS at Namecheap carries `CNAME ghs.googlehosted.com.` for each subdomain; the apex, which cannot be a CNAME, carries a Namecheap `ALIAS` record to the same name, which Namecheap flattens into that host's current A and AAAA records on a fixed five-minute TTL. Cloud Run routes by hostname at Google's front end, so any of Google's addresses works, and the Cloud Run console's list of eight static apex addresses is a suggestion, not a check: certificate issuance only needs the challenge to be reachable. Google provisions and renews each certificate once its record resolves, retrying on an hourly poll, so a freshly changed record can take up to an hour to show as provisioned. A new app's hostnames take only domain mappings here; the server already routes them by the naming convention.

## Logs

The server writes one JSON object per line to stdout (`internal/logging`), and Cloud Run's agent turns each into a structured entry: `severity` and `message` are promoted, `httpRequest` is rendered like the request log's own summary, and every other field lands in `jsonPayload`, where it can be filtered on. Every record written while handling a request carries `app` (the app's hostname label), `user` (the signed-in address), and Cloud Run's trace, so the Logs Explorer nests it under the Cloud Run request entry with the same trace. Each request past sign-in also gets one record of its own, `message="request"`, holding method, URL, status, latency, and user agent; media reads (`/photos/…` and the other bucket routes) skip that record, since a photo-heavy page fans out hundreds of them and Cloud Run's request log already lists each one. Writes log what changed as fields, and carry `actor` where the acting identity can differ from `user`: the directory and the volunteer portal resolve aliases, and a super admin can spoof.

Queries that answer the usual questions, in Logs Explorer with the service selected or with `gcloud logging read`:

    jsonPayload.user="someone@heliosschool.org"
    severity>=ERROR
    jsonPayload.app="hca" AND jsonPayload.message:"events:"
    jsonPayload.httpRequest.status>=500
    trace="projects/heliosian/traces/<trace id>"

Local development installs the same records over a text handler on stderr, so the dev server's output shows the same fields in `key=value` form.

## Verifying a deploy

The startup log (Cloud Run → Logs, or `gcloud logging read`) shows the full boot sequence: the blob store's prefetch line, geocoding count, directory model load, then `listening` — about ten seconds after the instance starts. After any deploy, an existing session should still work — if everyone got signed out, `SESSION_KEY` stopped reaching the server.
