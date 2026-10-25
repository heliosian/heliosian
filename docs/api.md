# The HTTP API

Every app's pages read and write the data model, and nothing else, through the same few routes on their own host: `QUERY /api/q` for reads, `POST /api/q` for batches of writes, the imperative calls under `/api/do/`, and `GET /api/blob/{id}/{column}` for a blob cell's bytes. What they take and answer, who they run as, and how a write is checked and committed are in `docs/datamodel.md` (The query API); `GET /api/openapi.json` describes them, table by table (`docs/admin/admin.md`, The spec). This file maps which host serves what.

## /api: one router for every host

`internal/app.NewCore` (`internal/app/wiring.go`) builds one router for `/api/`, and `Core.Handlers` sends every request whose path starts `/api/` to it, whichever host it came to, after the sign-in gate and request logging (`withAPI` in `internal/app/server.go`). An app's own mux never sees an `/api/` path, so a route there would never be reached: every API route is on every host, and a page calls its own origin whatever app it is. Each route checks for itself who may use it; none relies on the host it was reached through. The router holds:

- the data model's API (`db.Register` in `internal/db/http.go`): `QUERY` and `POST /api/q`, the `/api/do/` calls that act on any data (`photo`, `pronunciation`, `file`, `fetched`, `person-at`, `subscription`, `wiki`), `GET /api/blob/{id}/{column}`, `GET /api/openapi.json`, `GET /api/policies` and `GET /api/explain/{id}`;
- search, `POST /api/do/search` (`db.RegisterSearch`);
- the drafting calls (`db.RegisterDrafts` in `internal/db/draft.go`): `POST /api/do/draft-members`, the rule editor's preview of whom a draft holds, and `describe-group`, `describe-activity` and `describe-charity`, Claude's descriptions;
- the guest-list calls, `POST /api/do/invite`, `/api/do/add-guests` and `/api/do/guest-mail` (`RegisterInvitations`, `docs/datamodel.md`, Guest lists);
- Report a problem or idea, `POST /api/feedback`, and its triage, `POST /api/do/report-draft` and `POST /api/do/file-report` (`docs/feedback.md`);
- the address suggestions, `GET /api/address/suggest` (`internal/geocode/suggest.go`);
- image search, one prefix per app's image folder: `/api/home`, `/api/when`, `/api/team`, `/api/celebrate`, `/api/wiki` (`internal/imagesearch`, behind the shared sign-in gate);
- Ask's chat, `/api/ask/` (`docs/ask/ask.md`), and the MCP sign-in's approval calls, `/api/mcp/` (`docs/mcp/mcp.md`);
- Admin's calls: `POST /api/do/compose`, `GET /api/queues`, and the dashboard's `GET /api/system` and `GET /api/dashboard` (`docs/admin/admin.md`).

A registration that also draws pages (`RegisterInvitations`, `RegisterDashboard`, `ask.Register`, `mcp.Register`) takes the app's mux for its pages and the API router for its `/api/` routes.

## On every host, outside /api

Every app's mux also carries the opt-in form at `/optin`; stored bytes are served by `/api/blob/` alone (`docs/datamodel.md`, The query API). Sign-in's own routes (`/auth/…`, Spoof Mode's `/auth/spoof`) are `internal/auth`'s, registered on each mux with the gate (`docs/toolbar.md`, Spoof Mode).

## On one host

What an app adds outside `/api/` is only what it alone needs, registered in `internal/app/wiring.go`:

- **Heliosian** — its page and `/admin` (`HomePages` in `internal/db/homepages.go`) and its share card.
- **Helios Who?** — its pages, `GET /maps.js` and its about card (`registerDirectoryPages` in `internal/app/directory.go`).
- **Helios When** — its page shell for each address, the feeds (`/open/feed/`), share cards and previews (`WhenPages`), the outside guest's routes (`RegisterInvitations`), the delivery webhook `POST /hooks/events`, and the calendar watch's hook (Cloud Run only).
- **HCA-Team** — its page shell, share cards and redirects (`ActivityPages`).
- **Helios Celebrate** — share cards, previews and redirects (`PartyPages`).
- **Helios Staff Birthdays** — its page shell and about card.
- **Helios Loop** — its page shell and about card, the list mail's hook `POST /hooks/mail/mime` and unsubscribe page (`RegisterListMail`), and the delivery webhook.
- **Helios Ask** — its page and the school mail's hook `POST /hooks/mail/mime` (`RegisterMailHook` in `internal/db/mailhook.go`, `docs/datamodel.md`, Documents).
- **Helios Wiki** — its pages and share cards.
- **Helios Admin** — its pages, the dashboard's page and the build hook (`docs/admin/admin.md`).
- **Helios MCP** — the MCP endpoint and its OAuth server (`docs/mcp/mcp.md`).

Which hostnames reach which app is `hosts.Apps` in `internal/hosts/hosts.go`, the first of an app's `Hosts` its own name and the rest aliases; `hosts.Qualify` puts a label on a domain and `hosts.Origin` answers an app's origin on the production domain.

## Client

`web/common/api.js`'s `api(method, url, body)` is every page's one fetch, so a signed-out page reloads to sign-in as any other request would. `queries({name: query})` in `web/common/rail.js` sends a named batch to `QUERY /api/q` and answers each name's `result` beside the batch's shared `resources`; a page loads what it needs as one such batch, reads an answer's rows with `rowsOf(answer, table)` (in the answer's order) or `all(answer, table)` (every row reached, by ID), and writes with `write(batch)` in the same file, a post to `/api/q`; `q(text)` runs one query written in the language. `me()` in `web/common/data.js` answers the viewer's primary address, `email`, and `allowances`, `<app>.admins` for each of Who?, HCA-Team, Helios When and Heliosian whose `APP.admins` group holds the viewer, or every one for a super admin, which the shell reads for Admin Tools (`docs/toolbar.md`).

`web/common/directory.js` is the directory every picker, the person window and Spoof Mode read, in every app: `directory()` reads, once per page, through `/api/q`, everyone in the `everyone` group who is not hidden, their primary addresses, their ready photos, family memberships, the students, parents and staff role groups' effective members, and the grade, classroom and department groups. `listed()` is those `PERSON` rows as they are; `known(email)` and `personByEmail` find one by address, `emailOf`, `photoOf` (their own photo, else their family's), `gradeOf`, `placeOf` and `wordsOf` read one, `isStudent`, `isParent` and `isStaff` place them, `relativesOf(p, parents|children|partners|siblings)` reads their families, a row's `who_href` is their Who? page, and `contactLine` is the line under a name (a student's grade and classroom, a parent's children, anyone else's words).
