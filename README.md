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
- `internal/app` — server wiring shared by production and the dev server: host routing, file serving, the production assembly, each host's page routes and about cards, the directory's old addresses and the opt-in form
- `internal/auth` — Google sign-in and session cookies
- `internal/data` — tabular data sources: sample CSVs and Google Sheets
- `internal/spreadsheets` — which Google Sheets the server and the tools read, the environment variable holding each one's ID, and its title in Drive
- `internal/static` — reads the bundled files under `web/` that the models check image names against
- `internal/store` — the one store over every spreadsheet: the parts that build its model, commits and transactions, the Change Log and the write queue ([docs/storage.md](docs/storage.md))
- `internal/db` — the data model every app reads and writes: its schema, sheets and store, the query language and permissions, `/api/q` and `/api/do/`, the consent step and the opt-in sync, the calendar import's Google stage and the planning both stages share, the school mail's hook and the digest that reads each email into key points and to-dos, the toolbar's reports and their triage, and each app's page shells and share cards ([docs/datamodel.md](docs/datamodel.md); what a query costs and how to measure one, [docs/queryspeed.md](docs/queryspeed.md))
- `internal/model` — the old sheets the server still loads: the one `model.Model` with the parts that fill it and the store the server runs (`model.go`); the Config sheet, whose sign-out times every sign-in checks; the old Directory sheet's load; and the old document store Helios Ask's imports write: which mail, website pages and parent portal resources belong, each to markdown, the chunks, and the model that scans them
- `internal/ask` — Helios Ask: the chat with Claude over the data model - the prompt, the conversation, the streaming turn and the chips
- `internal/tools` — the read-only tools over the data model that Helios MCP serves and Helios Ask gives its model
- `internal/mcp` — Helios MCP: the OAuth server MCP clients sign in through, and the read-only tools over the data model
- `internal/artifacts` — the embeddings of Helios Ask's documents through Vertex AI, and the mail hook's settings
- `internal/hosts` — which hostnames reach which app, and an app's address on a tier
- `internal/mail` — outgoing email: Mailgun in production, and in development each message written to disk to open in a browser
- `internal/logging` — structured records Cloud Logging indexes: severity, the signed-in user, the app, and the request's trace
- `internal/feedback` — the GitHub App the triage queue files a kept report as
- `internal/blob` — media from Cloud Storage, held in memory with stored thumbnails
- `internal/sharecard` — the 1200x630 picture a chat app shows for a shared link, and the preview tags that name it, for every app in its own dress over a standard palette, under the app's name and subtitle as its `APP` row has them
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
- [api.md](docs/api.md) — the HTTP routes: what every host serves alike, what each app adds, and the shared client
- [screenshots.md](docs/screenshots.md) — page capture for humans and agents
- [deploy.md](docs/deploy.md) — production deployment
- [config.md](docs/config.md) — the Config sheet: the opt-in link and the sign-out times
- [toolbar.md](docs/toolbar.md) — the top bar every app shares: search with `/`, the account avatar, the switch between apps
- [brand.md](docs/brand.md) — where every app's logo art lives, and how its icons and lockups are cut from it
- [feedback.md](docs/feedback.md) — "Report a problem or idea": the form in every app, the `REPORT` row it becomes, and the queue a super admin files from

Helios Who?, in `docs/who/`:

- [directory.md](docs/who/directory.md) — the directory app spec
- [data.md](docs/who/data.md) — the directory data model and load pipeline
- [design.md](docs/who/design.md) — palette, typography, brand
- [pwa.md](docs/who/pwa.md) — installable-app wiring

Heliosian, in `docs/home/`:

- [home.md](docs/home/home.md) — the link portal spec
- [data.md](docs/home/data.md) — the widget, app, link and layout rows, who sees each, and who may edit them

HCA-Team, in `docs/team/`:

- [team.md](docs/team/team.md) — the volunteer portal spec
- [data.md](docs/team/data.md) — how the portal's data sits in the data model

Helios Staff Birthdays, in `docs/birthday/`:

- [birthday.md](docs/birthday/birthday.md) — the birthday team app spec

Helios Celebrate, in `docs/celebrate/`:

- [celebrate.md](docs/celebrate/celebrate.md) — the fun(d)raiser parties site spec
- [data.md](docs/celebrate/data.md) — where Celebrate's parties, tickets and payments live in the data model

Helios When, in `docs/when/`:

- [calendar.md](docs/when/calendar.md) — the school calendar app spec: the pages, the filters, sharing and RSVP invitations, and personal feeds
Helios Loop, in `docs/loop/`:

- [loop.md](docs/loop/loop.md) — the email lists app spec: email lists, rules, managers, and how mail to an email list reaches its members
- [data.md](docs/loop/data.md) — where an email list sits in the data model, what is keyed by its name, and the mail pipeline through Mailgun

Helios Ask, in `docs/ask/`:

- [ask.md](docs/ask/ask.md) — the chat app spec: the page, what Claude knows, what it can read
- [data.md](docs/ask/data.md) — the call to Claude, the conversation, the tools and their limits
- [artifacts.md](docs/ask/artifacts.md) — the community's documents, its mail, its website's pages and its parent portal: what is in them and what is withheld, how they become markdown, chunks and embeddings, and how they are searched

Helios Admin, in `docs/admin/`:

- [admin.md](docs/admin/admin.md) — the views of the data behind the apps: the table browser, the query runner, the schema diagram, and the OpenAPI spec of the query API

Helios Wiki, in `docs/wiki/`:

- [wiki.md](docs/wiki/wiki.md) — parent-to-parent info: wiki pages as data model documents, who may write them, and the app

Helios MCP, in `docs/mcp/`:

- [mcp.md](docs/mcp/mcp.md) — the data model for Claude and other MCP clients: connecting, signing in through OAuth, and the tools

Audits, in `docs/audits/`:

- [README.md](docs/audits/README.md) — how every audit is run: reading the existing issues first, and how findings are filed and closed
Each app keeps its own docs under `docs/<app>/`, named for its hostname; the top level is only what every app shares.
