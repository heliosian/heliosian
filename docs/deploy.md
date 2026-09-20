# Production deployment

The server runs as Cloud Run service `heliosian` in project `heliosian`, region `us-west1`. The one service hosts every app, choosing by hostname: `<app>.heliosian.com` names the app, the apex and `www` name home, and any other hostname gets a 404 even if Cloud Run routes it here (`internal/app`). Administration is driven with the gcloud CLI (`brew install --cask gcloud-cli`) authenticated as a project owner (`gcloud auth login`).

## Build and deploy pipeline

A push to `main` on `github.com/heliosian/heliosian` is meant to deploy production, through one Cloud Build trigger in `us-west1`, `build-main`, attached to the repository through a GitHub connection and carrying its build config inline — nothing in the repo describes a build. What it needs to do, in order:

- `docker build --target serve` of the repo's Dockerfile, tagged with the commit SHA and `latest`, then `docker build --target periodicsync` of the same Dockerfile, tagged the same way under the job's image name. The two builds share the worker's Docker daemon, so the second reuses the first's compile stage and the module compiles once per push.
- Push each SHA tag to Artifact Registry: `us-west1-docker.pkg.dev/heliosian/heliosian/heliosian` and `us-west1-docker.pkg.dev/heliosian/heliosian/periodicsync`.
- `gcloud run deploy heliosian --image …:<sha>` — image only, so every other service setting persists from the last full deploy — and `gcloud run jobs deploy periodicsync --image …:<sha>`, which changes only the job's image. The job has to exist first, from a `cmd/deploy` run: a job created with nothing but an image would run as the default compute account, which the build account may not act as, so the step fails until the job is there with its identity.

The trigger has to name its build service account (the project's default compute service account) explicitly; trigger creation in this project refuses to infer one. The trigger itself is the only record of what a build does — this page says what it needs to do, never what it does — so read it, and the recent builds with each one's outcome beside it, rather than this page:

    gcloud builds triggers describe build-main --region us-west1 --project heliosian
    gcloud builds list --region us-west1 --project heliosian --limit 10

Builds ship the pushed commit, never a working tree. To rebuild and redeploy current `main` without a push:

    gcloud builds triggers run build-main --region=us-west1 --branch=main

The Dockerfile has one compile stage and two final targets. The `golang` stage compiles both static binaries (`CGO_ENABLED=0`) in one `go build`. The `serve` target, `gcr.io/distroless/static-debian12` — CA certificates and tzdata, nothing else — carries the server binary plus `web/`, whose templates and static assets are read from disk at runtime; it is the last stage, so a `docker build` naming no target produces the serving image. The `periodicsync` target is a Debian base carrying poppler for the job (below). The repository keeps the ten most recent versions of each image and deletes the rest under a cleanup policy; what it holds is `gcloud artifacts repositories describe heliosian --location us-west1 --project heliosian`. `sampledata/` is deliberately excluded: production always sets `DIRECTORY_SHEET`, and a misconfigured server fails at startup rather than silently serving sample data. `creds/` never enters the image (`.dockerignore`, `.gcloudignore`), and the image holds no credentials of any kind — the runtime identity below is the only Google identity in play.

## Service configuration

`cmd/deploy` holds the full service configuration and is the only place it is written down:

    eval "$(go run ./cmd/findsheet)"
    go run ./cmd/deploy

It deploys the `latest` image with every setting the pipeline does not touch, so it both creates the service from nothing and repairs drift on an existing one, then adds any domain mapping the service lacks (Domain, below), and then does the same for the `periodicsync` job (below). The spreadsheet ids come from the environment (`cmd/findsheet` prints them as shell exports) and the OAuth client id from `creds/oauth-client.json` — the same resolution the server itself uses — so none of them is written into the repository. Every spreadsheet id is required, here and in `Production()`. Because a per-push deploy only ever changes the image (see above), changing a spreadsheet id only ever takes effect through a `cmd/deploy` run, never a plain push.

Why each setting is what it is:

- **service account** — the runtime identity, `directory@`. Application-default credentials inside the container resolve to it through the metadata server; there is no key file anywhere in the system.
- **min instances 1** — startup fetches every media object the sheets name before listening (how long that takes is in the startup log; see Verifying a deploy); still worth avoiding on a cold request.
- **Sheets quota** — the API allows 60 read requests per minute per user. Startup reads each spreadsheet in one batch request (`Tabs` in `internal/data`), so a start costs about one read per sheet rather than one per tab, which with every app's tabs would exceed the minute's quota on its own; the invite templates are the exception, read one tab at a time. A start beside the previous revision's refresh, two deploys inside a minute, or a deploy on top of a burst of local `loadcheck`/`dumptab` runs can still hit it, and a refused read at startup used to fail the revision (`load config: Error 429`). `internal/data/sheet.go` waits out a 429 and retries (5, 10, 20, 40 seconds), within the 240-second startup window; a refused write is retried the same way rather than dropped.
- **max instances 1** — the directory model and blob store live in per-instance memory with no cross-instance coherency; a self-service edit refreshes only the instance that handled it, so a second instance would serve stale data.
- **memory 2Gi** — the blob store holds every named object and its thumbnail in RAM. Because thumbnails are stored rather than generated, startup decodes no images and the peak sits close to the steady state. The startup log's `blob store: prefetched` lines report how many objects are held (`held`), and the service's Metrics tab in the Cloud Run console (container memory utilization) shows what share of this limit the container takes; resize when the footprint approaches half the limit.
- **concurrency 250** — with a single instance, every simultaneous request shares it, and a photo-heavy page fans out many image requests at once. The work is served from memory, so the ceiling exists to bound queueing, not CPU.
- **no CPU throttling** — the directory model and blob store refresh on five-minute tickers between requests; default throttling would starve them.
- **HTTP/2** — Cloud Run speaks cleartext HTTP/2 to the container, which the server accepts. Browsers already get HTTP/2 from the frontend either way.
- **allow unauthenticated** — the app enforces its own Google sign-in; Cloud Run must let everyone reach the login page.

Cloud Run injects `PORT`; the server honors it.

## The periodic sync job

The Cloud Run Job `periodicsync` in the same region runs `cmd/periodicsync` from its own image: one binary running every scheduled stage in turn, the way the service runs every app - the calendar import's PDF stage, the school's published year calendar, is the one stage; the import's Google stage runs in the service as the school's calendar changes (`docs/calendar/data.md`, Keeping up). A stage that fails is logged and the next runs; the run exits non-zero at the end naming every stage that failed. `cmd/deploy` writes the job's configuration beside the service's: the same runtime identity, the spreadsheet ids the stages read (`DIRECTORY_SHEET`, `PREFERENCES_SHEET`, `CALENDAR_SHEET`, `CONFIG_SHEET`), the Anthropic key as `ANTHROPIC_API_KEY` from the `heliosian-anthropic-key` secret, and the `--i-have-user-permission-to-spend-money` argument, since scheduling the job is that permission. A task gets 1 GiB, since the import rasterizes the year calendar at 300 dpi and works on it pixel by pixel, and 30 minutes, since reading a new PDF is dozens of Claude calls at maximum effort; a task that fails is not retried, because the import already carries what it completed to the sheet and asks about only what failed on the next run, so a retry would spend the same money on the same failure. What the job carries right now - image, identity, resources, timeout, retries, environment and secrets - is `gcloud run jobs describe periodicsync --region us-west1 --project heliosian`, and the service's counterpart is `gcloud run services describe heliosian --region us-west1 --project heliosian`.

Cloud Scheduler job `periodicsync` (location `us-west1`) starts an execution on a cron schedule, by posting to the Cloud Run Admin API's run method as `directory@`, which needs `run.invoker` on the job for that (IAM, below, says how to check). The schedule lives only in that Scheduler job - nothing in the repository records or reconciles it - so read it there rather than assuming a cadence, along with when it last fired and when it fires next:

    gcloud scheduler jobs describe periodicsync --location us-west1 --project heliosian

`schedule` is the cron expression, read in `timeZone` (school time), `lastAttemptTime` the last firing, `scheduleTime` the next. A different cadence is an update to the same resource:

    gcloud scheduler jobs update http periodicsync --location us-west1 --project heliosian --schedule "<cron expression>"

The Scheduler job is a one-time resource created by hand rather than in `cmd/deploy`:

    gcloud run jobs add-iam-policy-binding periodicsync --region us-west1 --project heliosian --member serviceAccount:directory@heliosian.iam.gserviceaccount.com --role roles/run.invoker
    gcloud scheduler jobs create http periodicsync --location us-west1 --project heliosian --schedule "<cron expression>" --time-zone America/Los_Angeles --http-method POST --uri https://run.googleapis.com/v2/projects/heliosian/locations/us-west1/jobs/periodicsync:run --oauth-service-account-email directory@heliosian.iam.gserviceaccount.com

A project still carrying the earlier `calendarimport` trigger, Cloud Run job and Scheduler job, from when the calendar import was the only stage, moves to these names by hand: the trigger is renamed in the Cloud Build console (its build config is inline there, so the Dockerfile, the image name and the job name change with it), the Cloud Run job is made afresh by `cmd/deploy` under the new name, and the Scheduler job is created again with the commands above, the old three deleted once the new ones have run.

To run the sync now rather than wait for the next firing:

    gcloud run jobs execute periodicsync --region us-west1 --project heliosian --wait

The recent executions, each with when it ran and whether it succeeded:

    gcloud run jobs executions list --job periodicsync --region us-west1 --project heliosian --limit 24 --format "table(metadata.name,status.startTime,status.completionTime,status.succeededCount,status.failedCount)"

One execution's log, by the name that list gives:

    gcloud logging read 'resource.type="cloud_run_job" AND labels."run.googleapis.com/execution_name"="<execution name>"' --project heliosian --order asc --format "value(timestamp,severity,textPayload)"

The same logs are under the job in the console, or in Logs Explorer with `resource.type="cloud_run_job"`. Each stage announces itself with a `stage:` line. A run that changed nothing is two fetches and no Claude calls; one that read a new PDF says so. A run whose stage failed exits non-zero naming it and shows as a failed execution.

## The school calendar watch

The service keeps the calendar's Google Import in step with the school's calendar on its own, through a Calendar API watch channel posting to `https://when.heliosian.com/hooks/calendar` and an hourly sweep (`docs/calendar/data.md`, Keeping up). It starts only on Cloud Run, which it knows by Cloud Run's own `K_SERVICE` variable, and needs nothing beyond what the service already carries: `directory@`, the Calendar API's read-only scope, the Anthropic key. Google offers no way to list channels, so the service log is the record - the channel each revision opened and when it expires, every post from Google, and every run with how long it took:

    jsonPayload.message:"calendar watch"
    jsonPayload.message="calendar changed"
    jsonPayload.message="calendar import"

The run's own lines - the feed count, the cells updated, the change log rows - are plain text beside them, as the job writes them.

## Configuration values

Plain environment variables:

- `DIRECTORY_SHEET` — the production spreadsheet id: the `Directory` sheet living in the community shared drive.
- `PREFERENCES_SHEET` — the `Preferences` sheet in the same shared drive: the sharing-consent form's response spreadsheet.
- `INVITES_SHEET` — the `Invite List Builder` spreadsheet id. Powers the Invites page (`/greenvelope`).
- `APPS_SHEET` — the `Apps` spreadsheet id: Heliosian's categories, links, and admins (`docs/home/data.md`).
- `EVENTS_SHEET` — the `Events` spreadsheet id: the volunteer portal's activities, roles, sign-ups, and admins (`docs/team/data.md`).
- `BIRTHDAY_SHEET` — the `Birthdays` spreadsheet id: the birthday team's staff birthdays, pipeline progress, charities, and admins (`docs/birthday/data.md`).
- `CELEBRATE_SHEET` — the `Celebrate` spreadsheet id: Helios Celebrate's parties, hosts, tickets, and admins (`docs/celebrate/data.md`).
- `CALENDAR_SHEET` — the `Calendar` spreadsheet id: Helios When's imported events, enrichment, overrides, day types, feeds, and admins (`docs/calendar/data.md`).
- `CONFIG_SHEET` — the `Config` spreadsheet id: the platform super admins and settings (`docs/config.md`).
- `GROUPS_SHEET` — the `Groups` spreadsheet id: Helios Loop's groups, managers, rules, and admins (`docs/loop/data.md`).
- `ARTIFACTS_SHEET` — the `Artifacts` spreadsheet id: the index of the community's documents Helios Ask searches (`docs/ask/artifacts.md`).
- `GOOGLE_CLIENT_ID` — the OAuth web client id; not a secret (every login page fetches it from `/auth/client`), but kept out of the repository.

Secret Manager secrets, delivered as environment variables. Values are used raw, so payloads must not carry trailing newlines. Which exist is `gcloud secrets list --project heliosian`, and which the service and the job are actually given is in their `describe` output (The periodic sync job, above); each is what the service needs, not a claim that it is there:

- `heliosian-session-key` — session-cookie HMAC key (any long random string); losing or rotating it signs everyone out
- `heliosian-geocoding-key` — the Geocoding API server key, mirrored locally as `creds/geocoding.key`
- `heliosian-maps-browser-key` — the Maps JavaScript browser key, mirrored locally as `creds/maps.key`
- `heliosian-pexels-key` / `heliosian-pixabay-key` (optional, as `PEXELS_KEY` / `PIXABAY_KEY`) — the Pexels and Pixabay API keys, two more sources for the image picker
- `heliosian-unsplash-key` — the Unsplash API access key (as `UNSPLASH_KEY`) behind the portal's image picker's Unsplash source, mirrored locally as `creds/unsplash.key`; without it the picker offers Wikimedia Commons alone
- `heliosian-mailgun-key` — the Mailgun API key (as `MAILGUN_KEY`) every app's mail goes out through, mirrored locally as `creds/mailgun.key`. What the apps need of the Mailgun account (US region) is written here as needs, never as what the account holds - the dashboard and the API, read below, are the only record of that. `reply.heliosian.com` is the domain every other app sends from; the volunteer portal sends as `HCA-Team <team@loop.heliosian.com>` (override with `MAIL_FROM`), the `team` group in Helios Loop, so a reply to it reaches whoever that group holds, as does Loop's notice to someone a group does not let post. Helios Celebrate sends as `Helios Celebrate <celebrate@reply.heliosian.com>` (`CELEBRATE_MAIL_FROM`), Staff Birthdays as `Helios Staff Birthdays <birthday@reply.heliosian.com>` (`BIRTHDAY_MAIL_FROM`; `BIRTHDAY_BASE_URL`, default `https://birthday.heliosian.com`, is the address its reminders link to), Helios Who? as `Helios Who? <who@reply.heliosian.com>` (`WHO_MAIL_FROM`) and Helios When as `Helios When <when@reply.heliosian.com>` (`CALENDAR_MAIL_FROM`), which its invites also name as their organizer and reply address (`CALENDAR_REPLY_TO`, the same address, since a calendar app may answer the sender rather than the organizer). So it needs to be verified for sending - its DKIM record and its SPF `include` at the registrar, where the wildcard CNAME otherwise answers for the name - and set up to receive, its MX records pointing at Mailgun, so that the Accept or Decline a person taps in their calendar app lands on Mailgun whichever address it answers; and it needs one route matching every recipient under the domain that stores the message with a notification to `https://when.heliosian.com/hooks/replies` (`docs/calendar/calendar.md`, Answering), which reads the calendar's replies and drops everything else. Beside it, a route matching the one secret address a parent's school mailbox forwards to stores the message with a notification to `https://ask.heliosian.com/hooks/mail`, so Helios Ask takes that mail into its documents (`docs/ask/artifacts.md`, From a message to a document); the address is kept nowhere but that route. Both routes fire for that address, and the calendar's hook drops the copy, since it reads only mail to its own reply address. `heliosian.com` is a domain no app may send from: its MX belongs to Google Workspace, where the school reads mail to its addresses, so an address there on an app's mail invites replies into an inbox nobody watches. `loop.heliosian.com` is Helios Loop's, and needs the same: sending verified, MX records at Mailgun (which is what takes the domain's mail away from Google Workspace, where it is otherwise a secondary domain), a route matching every recipient under it that stores the message with a notification to `https://loop.heliosian.com/hooks/mail`, and the domain's webhooks for delivery, permanent failure, temporary failure and complaint pointing at `https://loop.heliosian.com/hooks/events` (`docs/loop/data.md`, Mail). What the account holds, read with the local key:

      curl --user "api:$(cat creds/mailgun.key)" https://api.mailgun.net/v4/domains
      curl --user "api:$(cat creds/mailgun.key)" https://api.mailgun.net/v4/domains/reply.heliosian.com
      curl --user "api:$(cat creds/mailgun.key)" https://api.mailgun.net/v3/routes
      curl --user "api:$(cat creds/mailgun.key)" https://api.mailgun.net/v3/domains/loop.heliosian.com/webhooks

  The first lists the domains and each one's state; the second is one domain's sending and receiving DNS records, each with whether Mailgun last found it valid (a `PUT` to the same address with `/verify` on the end makes it look again); the third is every route with its filter and its actions; the fourth is a domain's webhooks and where they point. What the registrar answers right now, whatever Mailgun last saw:

      dig +short MX reply.heliosian.com
      dig +short TXT reply.heliosian.com

  Mail that has gone out, with each bounce and complaint, is under the domain's events - `https://api.mailgun.net/v3/reply.heliosian.com/events` with the same key, or Sending › Logs in the dashboard. Mailgun is the only way mail leaves: without the key the apps send nothing, and the portal says so in Admin Tools.
- `heliosian-mailgun-webhook-key` — Mailgun's HTTP webhook signing key (as `MAILGUN_WEBHOOK_KEY`), from the account's Webhooks page, which signs every route's notifications and every domain's event webhooks. Mirrored locally as `creds/mailgun-webhook.key`. Without it, or without the API key, the calendar's reply route, Loop's two routes and Helios Ask's mail route answer 404: replies go unread, mail to the groups goes nowhere and forwarded school mail stays out of Ask's documents.
- `heliosian-github-token` — the fine-grained GitHub personal access token (as `GITHUB_TOKEN`) that files the toolbar's "Report a problem or idea" reports as issues in the private `heliosian/triage` repository (`docs/feedback.md`), mirrored locally as `creds/github.token`; required, so the secret must be wired through a `cmd/deploy` run before a build carrying the feedback endpoint is deployed
- `heliosian-anthropic-key` — the Anthropic API key (as `ANTHROPIC_API_KEY`) the calendar import job classifies and reads with, and the service uses for Staff Birthdays' one-sentence charity descriptions (`docs/birthday/birthday.md`) and for every answer in Helios Ask (`docs/ask/data.md`); required by the service. Mirrored locally as `creds/anthropic.key`. The key never reaches a browser: the server asks on behalf of someone signed in
- `heliosian-search-key` / `heliosian-search-cx` (optional, as `GOOGLE_SEARCH_KEY` / `GOOGLE_SEARCH_CX`) — a Custom Search API key and Programmable Search Engine id, should Google ever grant this project the JSON API; without them the portal's image picker searches Wikimedia Commons, which needs nothing. Wire them with `gcloud run services update heliosian --region us-west1 --update-secrets ...` and add them to `secrets` in `cmd/deploy` so a full deploy keeps them.

## Media storage

Photos, pronunciation recordings, Heliosian's link images, the volunteer portal's activity images, and the community's documents Helios Ask searches, under `artifacts/`, which no route serves (`docs/ask/artifacts.md`) - live in `gs://heliosian-media`, the single source of truth for media — see `docs/who/data.md` for the naming convention and stored thumbnails. The bucket needs to be in `us-west1` with uniform access and public access prevented; what it is actually set to is `gcloud storage buckets describe gs://heliosian-media --project heliosian`. Sitting in the same region as the service, everything the sheets name fetches into memory in a few seconds - each `blob store: prefetched` line in the startup log says how many names and how long (`took`) - with no per-request throttle to work around. Nothing ever lists the bucket: the sheets are the index, each loader asks for its objects by name, and an object no sheet has named for a while is dropped from memory. Object versioning carries no upload history: an object is named for its own bytes and is never rewritten, so each upload is a separate object that a sheet either names or does not.

Every read goes through the app's own sign-in gate, and no object may ever be publicly readable, which rests on the bucket's public access prevention (the `describe` above) and on its policy granting nothing to `allUsers` or `allAuthenticatedUsers` (`gcloud storage buckets get-iam-policy gs://heliosian-media --project heliosian`); the service reads and writes it as `directory@`.

Helios Loop keeps every message its groups receive in a second bucket, `gs://heliosian-mail`, which needs the same settings as the media bucket (the same `describe` and `get-iam-policy`, with its name, read them), whole as it came, under `loop/<group>/` (`docs/loop/data.md`, Mail). It is its own bucket because nothing in the media bucket's serving routes may ever reach a message: the mail bucket has no route at all, and the `Messages` tab of the Groups sheet is its index. Make it as the media bucket was made, with `roles/storage.objectAdmin` on it to `directory@`.

## IAM

`directory@heliosian.iam.gserviceaccount.com` is both the data identity and the runtime identity. Each line here is what it needs and where to read whether it has it; none is a claim that it does:

- Data access: content manager on the community shared drive, which covers editing the `Directory` sheet inside it (self-service edits write cells and append to the Change Log tab). Shared in Drive directly, never through project IAM, so the shared drive's members list in Drive is the record, and `go run ./cmd/findsheet` is the working check: it lists every spreadsheet the account can see.
- Media: `roles/storage.objectAdmin` on `gs://heliosian-media` and on `gs://heliosian-mail`, granted on the bucket: `gcloud storage buckets get-iam-policy gs://heliosian-media --project heliosian`, and the same with the mail bucket's name.
- Runtime: the Cloud Run service and the `periodicsync` job run as it (`cmd/deploy` sets both, and their `describe` output, above, says what they run as), and it needs Secret Manager Secret Accessor on each secret individually: `gcloud secrets get-iam-policy <secret name> --project heliosian`, one secret at a time from `gcloud secrets list --project heliosian`.
- Vertex AI: Helios Ask embeds every question, and the importer every document, through Gemini's embedding model on Vertex AI (`docs/ask/artifacts.md`), which needs the Vertex AI API (`aiplatform.googleapis.com`) on in the project and `roles/aiplatform.user` on the project for `directory@`: `gcloud projects get-iam-policy heliosian --flatten "bindings[].members" --filter "bindings.members:directory@" --format "value(bindings.role)"`.
- School calendar: the import reads the school's public Google Calendar through the Google Calendar API (`calendar-json.googleapis.com`, which has to be on in the project) with the read-only calendar scope, which the client asks for by name since a token without it is refused; the calendar being public, no sharing to `directory@` is needed. Which APIs the project has on is `gcloud services list --enabled --project heliosian`.
- Scheduling: `roles/run.invoker` on the `periodicsync` job, which is how the Cloud Scheduler job starts an execution as it: `gcloud run jobs get-iam-policy periodicsync --region us-west1 --project heliosian`.
- Humans: `roles/iam.serviceAccountTokenCreator` on this account for whoever runs real-data development locally (`docs/dev.md`): `gcloud iam service-accounts get-iam-policy directory@heliosian.iam.gserviceaccount.com --project heliosian`.

The build service account (the default compute service account) needs `cloudbuild.builds.builder` and `run.developer` on the project (`gcloud projects get-iam-policy heliosian`) and Service Account User on `directory@` (the account's own policy, above) — the last because deploying a service that runs as an account requires permission to act as it.

The `heliosian.com` organization ships Google's secure-by-default org policies, two of which the design rests on. What each is set to, as it lands on the project:

    gcloud org-policies describe iam.managed.disableServiceAccountKeyCreation --project heliosian --effective
    gcloud org-policies describe iam.allowedPolicyMemberDomains --project heliosian --effective

- `iam.managed.disableServiceAccountKeyCreation` has to stay enforced. The keyless design depends on it: no key for `directory@` may exist.
- `iam.allowedPolicyMemberDomains` has to be allow-all at the project scope, and nowhere wider, because unauthenticated access requires granting `run.invoker` to `allUsers`.

## OAuth

The consent screen has to live in this project, audience External, published to production; APIs & Services › OAuth consent screen in the console is where that is set and the only place to read it, gcloud having no view of it. Internal is not an option: the app restricts sign-in to `heliosschool.org` accounts (`internal/auth`), and those live outside the `heliosian.com` org. Published-External keeps the basic sign-in scopes free of verification friction.

The web client's authorized JavaScript origins have to hold the `*.local.heliosian.com` development hosts and each app's production hostnames; sign-in fails on any origin not listed, and edits take a few minutes to propagate. The client is under APIs & Services › Credentials, the one whose id `creds/oauth-client.json` carries, and its origins list there is the only record of what is allowed. A new app's hostnames go here as well as into the domain mappings below. Google rejects made-up domains such as `.localhost` here, which is why local development runs under a real subdomain that public DNS points at loopback, and it only accepts a plain-HTTP login URI on `localhost` itself, which is why the dev server serves HTTPS with a self-signed certificate (`docs/dev.md`). The browser maps key is rendered into every page, so it has to carry an HTTP-referer restriction of its own, `https://*.local.heliosian.com:8080/*` and `https://*.heliosian.com/*`, which covers every host the service answers to; `gcloud services api-keys list --project heliosian` names the keys, and `gcloud services api-keys describe <key id> --project heliosian` shows what one is restricted to.

## Domain

Every hostname is a Cloud Run domain mapping on the one service, one per app plus `www` and the apex. `cmd/deploy` keeps them: the list of hostnames is the router's own (`app.Hostnames` in `internal/app` - every app in the registry, every alias, the apex and `www`), and after the service deploy it creates whatever mapping the service lacks. A mapping already there is left as it is and none is ever removed, so a hostname that leaves the router stays mapped until someone deletes it by hand. A new app therefore takes nothing here beyond its registry entry, a `cmd/deploy` run, and its origins in the OAuth client. The current set, with each certificate's status:

    gcloud beta run domain-mappings list --region us-west1 --project heliosian --format "table(metadata.name,status.conditions[0].status)"

DNS at Namecheap has to carry a wildcard `CNAME ghs.googlehosted.com.` record, `*`, so that every subdomain resolves without a record of its own and a new mapping needs nothing at the registrar; on the apex, which cannot be a CNAME, a Namecheap `ALIAS` record to the same name, which Namecheap flattens into that host's current A and AAAA records on a fixed five-minute TTL; and a `*.local` record answering loopback for development (`docs/dev.md`). The registrar's Advanced DNS page is the record of what is set; what the world resolves right now is

    dig +short CNAME who.heliosian.com
    dig +short heliosian.com
    dig +short who.local.heliosian.com

the first answering `ghs.googlehosted.com.`, the second Google addresses, the last `127.0.0.1`. Cloud Run routes by hostname at Google's front end, so any of Google's addresses works, and the Cloud Run console's list of eight static apex addresses is a suggestion, not a check: certificate issuance only needs the challenge to be reachable. Google provisions and renews each certificate once its name resolves, retrying on an hourly poll, so a fresh mapping can take up to an hour to show as provisioned. Helios When answers under three labels, `calendar`, `cal` and `when`, so it takes three mappings, and its personal feed addresses (`/open/feed/…`) are fetched by calendar apps with no session, so nothing in front of the service may demand one.

## Logs

The server writes one JSON object per line to stdout (`internal/logging`), and Cloud Run's agent turns each into a structured entry: `severity` and `message` are promoted, `httpRequest` is rendered like the request log's own summary, and every other field lands in `jsonPayload`, where it can be filtered on. Every record written while handling a request carries `app` (the app's hostname label), `user` (the signed-in address), and Cloud Run's trace, so the Logs Explorer nests it under the Cloud Run request entry with the same trace. Each request past sign-in also gets one record of its own, `message="request"`, holding method, URL, status, latency, and user agent; media reads (`/photos/…` and the other bucket routes) skip that record, since a photo-heavy page fans out hundreds of them and Cloud Run's request log already lists each one. Writes log what changed as fields, and carry `actor` where the acting identity can differ from `user`: the directory and the volunteer portal resolve aliases. While a super admin is in Spoof Mode (`docs/toolbar.md`) every record of theirs carries `as`, the person they are viewing as, beside their own `user`.

Queries that answer the usual questions, in Logs Explorer with the service selected or with `gcloud logging read`:

    jsonPayload.user="someone@heliosschool.org"
    severity>=ERROR
    jsonPayload.app="team" AND jsonPayload.message:"events:"
    jsonPayload.httpRequest.status>=500
    trace="projects/heliosian/traces/<trace id>"

Local development installs the same records over a text handler on stderr, so the dev server's output shows the same fields in `key=value` form.

## Verifying a deploy

The startup log (Cloud Run → Logs, or `gcloud logging read`) shows the full boot sequence: `loaded config`, each app's `loaded … model`, the blob store's `prefetching` and `prefetched` lines around each, the geocoding line (how many addresses came from the `Geocode` tab and how many went to the API), then `listening`; the boot time is the span from the instance's first line to that one. The five-minute refreshes log the same `loaded` and `blob store` lines without a `listening`, so a boot is the run that ends in one. When the most recent boot happened:

    gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="heliosian" AND jsonPayload.message="listening"' --project heliosian --freshness 7d --limit 1 --format "value(timestamp)"

and its whole sequence, over the minute leading up to that time:

    gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="heliosian" AND timestamp>="<a minute before>" AND timestamp<="<that time>" AND (jsonPayload.message:"loaded" OR jsonPayload.message:"blob store: prefetch" OR jsonPayload.message:"geocoded" OR jsonPayload.message="listening")' --project heliosian --order asc --format "value(timestamp,jsonPayload)"

After any deploy, an existing session should still work — if everyone got signed out, `SESSION_KEY` stopped reaching the server.
