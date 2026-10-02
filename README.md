# Heliosian

Web apps for the Helios school community (K-8), served as one static Go binary on Cloud Run, one app per hostname under heliosian.com; each app is described under Docs below. Built in the open by community volunteers, mostly through coding agents; the repo contains no secrets and no real community data.

## Quick start

    brew install go
    go run ./tools/startserver

Open https://who.heliosiandev.com:8080 and click past the browser's warning about the self-signed certificate (the server picks the app by hostname; that name resolves to your own machine). That's the whole setup: the dev server loads the fictional community in `sampledata/`, signs every request in as a sample parent, and answers the outside services it calls - Maps, Claude, Vertex AI, Mailgun - in-process — no credentials, no cloud project. Templates, static assets, and sample data are read from disk on every request, so edit a file and refresh; nothing needs restarting.

`brew install --cask google-chrome` additionally enables the screenshot tooling used to verify visual changes ([docs/screenshots.md](docs/screenshots.md)). No Node, no Docker. Go 1.27 or later.

To run against real community data instead, see [docs/dev.md](docs/dev.md).

## Layout

- `main.go` — the production entry point; dev serving lives in `tools/startserver`
- `internal/app` — server wiring shared by production and the dev server: host routing, file serving, and the production assembly
- `internal/auth` — Google sign-in and session cookies
- `internal/data` — tabular data sources: sample CSVs and Google Sheets
- `internal/spreadsheets` — which Google Sheets the server and the tools read, the environment variable holding each one's ID, and its title in Drive
- `internal/static` — reads the bundled files under `web/` that the models check image names against
- `internal/store` — the one store over every spreadsheet: the parts that build its model, commits and transactions, the Change Log and the write queue ([docs/storage.md](docs/storage.md))
- `internal/db` — the new data model: its schema, sheets and store, the query language and permissions, `/api/q` and `/api/do/`, the consent step and the opt-in sync, and the calendar import's Google stage and the planning both stages share ([docs/datamodel.md](docs/datamodel.md))
- `internal/model` — the shared data layer: the one `model.Model` every app reads, a field per sheet, with the parts that fill it and the store the server runs (`model.go`), the per-request scope the resource API reads it through (`scope.go`) and the registry of every resource type (`resources.go`); the Config sheet (super admins and the platform settings), every app's admin list, the audience rules every filter reads, the directory's model load, handlers, admin tools and self-service edits; Helios When: the Calendar sheet's model, audience resolution, the day plan, handlers, guest lists and invitations, and personal feeds; HCA-Team, the volunteer portal: the Events sheet's activities, handlers, sign-ups, admin edits, pages and share cards; Helios Celebrate: the Celebrate sheet's parties, tickets and the waitlist, handlers, admin edits, pages and share cards; Staff Birthdays: the Birthdays sheet's model, the derived dates and stages, handlers, and admin edits; Helios Loop: the Groups sheet's email lists, the rule evaluator, the mail received and forwarded, handlers, and admin edits; Heliosian, the link portal: the Apps sheet's model load, handlers, widgets and admin edits; Helios Ask's documents: which mail, website pages and parent portal resources belong, each to markdown, the chunks, and the model that scans them; the toolbar's reports: the Reports sheet they land in, the word of them to the super admins, and the triage queue that edits one into a GitHub issue; and the Magic Tags each app reads from the others
- `internal/who` — the directory app's page host: its page routes, the opt-in form and its share card
- `internal/ask` — Helios Ask: the chat with Claude over every app's data - the prompt, the conversation, the streaming turn and the read-only tools
- `internal/artifacts` — the embeddings of Helios Ask's documents through Vertex AI, and the mail hook's settings
- `internal/digest` — what Claude reads out of each school email and Loop email list post in one call: the key points and audience for Heliosian's Inbox widget and the to-dos for its Reminders widget, kept in the artifacts sheet's Reads and To Dos tabs
- `internal/mail` — outgoing email: Mailgun in production, and in development each message written to disk to open in a browser
- `internal/logging` — structured records Cloud Logging indexes: severity, the signed-in user, the app, and the request's trace
- `internal/feedback` — the GitHub App the triage queue files a kept report as
- `internal/blob` — media from Cloud Storage, held in memory with stored thumbnails
- `internal/sharecard` — the 1200x630 picture a chat app shows for a shared link, and the preview tags that name it, for every app in its own dress over a standard palette, under the app's name and tagline as the registry has them
- `internal/imagesearch` — every app's image routes: uploads, the picture search (Unsplash, Pexels and Pixabay) and import, behind each app's gate and a per-person hourly limit, for Heliosian, HCA-Team, Celebrate and Helios When
- `internal/geocode` — address → coordinates for the map
- `internal/capture` — drive Chrome and screenshot a page, for the dev tools
- `internal/devtls` — the in-memory self-signed certificate local HTTPS runs on
- `internal/intercept` — a loopback proxy, trusted through a certificate authority minted in memory, that answers one host's HTTPS in-process for the dev tools and tests
- `internal/devcache` — the dev tools' intercept of the storage API that caches bucket reads under `local/cache/blobs/`
- `web/` — one directory per app, named for its hostname, holding its pages and every file it serves behind sign-in, `web/common/` for what all apps share, and `web/public/<app>/` and `web/public/common/` for the few files served before sign-in (frameworkless JavaScript throughout)
- `sampledata/` — the fictional community served by default
- `tools/` — dev tooling, never deployed: the dev server, screenshots, browser driving, sheet inspection, imports, deploy
- `asset-sources/` — the designer's Photoshop files and exports every app's brand art is cut from, never deployed (`docs/brand.md`)
- `docs/` — everything below

## Docs

Platform:

- [dev.md](docs/dev.md) — the principles behind it, and local development, including real-data mode
- [api.md](docs/api.md) — the resource API every host serves alike: the model it reads, types, reads, writes and `data.js`
- [screenshots.md](docs/screenshots.md) — page capture for humans and agents
- [deploy.md](docs/deploy.md) — production deployment
- [config.md](docs/config.md) — the Config sheet: super admins and the platform settings
- [toolbar.md](docs/toolbar.md) — the top bar every app shares: search with `/`, the account avatar, the switch between apps
- [brand.md](docs/brand.md) — where every app's logo art lives, and how its icons and lockups are cut from it
- [feedback.md](docs/feedback.md) — "Report a problem or idea": the form in every app, the sheet it lands in, and the queue an admin files from

Helios Who?, in `docs/who/`:

- [directory.md](docs/who/directory.md) — the directory app spec
- [data.md](docs/who/data.md) — the directory data model and load pipeline
- [design.md](docs/who/design.md) — palette, typography, brand
- [pwa.md](docs/who/pwa.md) — installable-app wiring

Heliosian, in `docs/home/`:

- [home.md](docs/home/home.md) — the link portal spec
- [data.md](docs/home/data.md) — the Apps sheet and its rules

HCA-Team, in `docs/team/`:

- [team.md](docs/team/team.md) — the volunteer portal spec
- [data.md](docs/team/data.md) — the Events sheet and its rules

Helios Staff Birthdays, in `docs/birthday/`:

- [birthday.md](docs/birthday/birthday.md) — the birthday team app spec
- [data.md](docs/birthday/data.md) — the Birthdays sheet and its rules

Helios Celebrate, in `docs/celebrate/`:

- [celebrate.md](docs/celebrate/celebrate.md) — the fun(d)raiser parties site spec
- [data.md](docs/celebrate/data.md) — the Celebrate sheet and its rules

Helios When, in `docs/when/`:

- [calendar.md](docs/when/calendar.md) — the school calendar app spec: the pages, the filters, sharing and RSVP invitations, and personal feeds
- [data.md](docs/when/data.md) — the Calendar sheet, the day plan, and the import

Helios Loop, in `docs/loop/`:

- [loop.md](docs/loop/loop.md) — the email lists app spec: email lists, rules, managers, and how mail to an email list reaches its members
- [data.md](docs/loop/data.md) — the Groups sheet, the membership evaluator, and the mail pipeline through Mailgun

Helios Ask, in `docs/ask/`:

- [ask.md](docs/ask/ask.md) — the chat app spec: the page, what Claude knows, what it can read
- [data.md](docs/ask/data.md) — the call to Claude, the conversation, the tools and their limits
- [artifacts.md](docs/ask/artifacts.md) — the community's documents, its mail, its website's pages and its parent portal: what is in them and what is withheld, how they become markdown, chunks and embeddings, and how they are searched

Audits, in `docs/audits/`:

- [README.md](docs/audits/README.md) — how every audit is run: reading the existing issues first, and how findings are filed and closed
Each app keeps its own docs under `docs/<app>/`, named for its hostname; the top level is only what every app shares.
