# Plan

## Architecture

- Single Go server binary hosting multiple apps under one process, with per-app routing. The directory app is the first; the structure anticipates more.
- Client is frameworkless JavaScript served by the binary. No build step unless/until TypeScript is adopted, and then a minimal one.
- Structured data is read from Google Sheets through a data-source abstraction, so a Veracross-backed implementation can replace the Sheets one without touching app code.
- Photos and other blobs are stored in Google Drive and served through the binary (with caching), never linked directly.

## Hosting and deployment

- Runs on Google Cloud Run.
- Sign-in is Google authentication restricted to the school's Google Workspace domain. Community-only data is never served to unauthenticated requests.
- GitHub is the source of truth. Pushes to the main branch trigger an automatic build and deploy to Cloud Run.
- The Docker build produces a static binary in a minimal base image containing only tzinfo and CA certificates.

## Local development

- One command starts the local server.
- Templates, static assets, and content reload on change without restarting the server.
- A local data mode serves generated, non-production sample data so contributors never need real community data to develop or test.
- A documented screenshot mechanism captures pages from the local dev server, so agents (and humans) can verify visual changes. See `docs/screenshots.md` once it exists.

## Directory app

- Clone the existing app's functionality and layout, working from screenshots of the current app as the reference.
- Views: browsable/searchable directory of families and individuals; detail pages with contact info, photos, and name pronunciation.
- Photo handling: individual and family photos uploaded to Drive, resized/cached for serving.
- Pronunciation: stored per person; representation (text respelling vs. audio) decided during the clone.

## Data

- Sheets layout: one spreadsheet per app, one tab per entity type, first row is the schema. The server reads via the Sheets API with a service account.
- Import tooling brings existing data from the current platforms into Sheets and Drive.
- When a Veracross API is available, a second data-source implementation replaces Sheets as the backend for directory data.

## Milestones

1. Repo scaffold: server skeleton, app routing, local dev loop with reload, sample data mode, screenshot tooling, contributor/agent docs.
2. Directory app read-only clone against sample data.
3. Google Sheets and Drive integration; real data imported.
4. Cloud Run service, GitHub auto-deploy pipeline, minimal image build.
5. Google domain sign-in gating community-only data.
6. Photo/pronunciation upload flows.
7. Subsequent apps.

## Open questions

- TypeScript adoption: start with plain JavaScript; revisit if client code grows.
- Serving domain and Cloud project layout.
