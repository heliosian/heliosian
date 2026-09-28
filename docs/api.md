# The resource API

One data-centric API, served identically on every app's host, that pages compose instead of each app's own `/api/<app>/` routes. `internal/api` is the machinery and knows nothing about any app; `internal/app` registers the types and lifts them onto the snapshot. Which types are registered is `resources` in `internal/app/snapshot.go`.

## The snapshot

`app.Snapshot` holds every store's model. `store.Queue.OnSwap` runs its hook under the commit lock after every commit's swap and every refresh's swaps, and the hook publishes a new snapshot to the registry (`Registry.Publish`), which also builds the alias index then. A request takes the published snapshot once and reads nothing else, so a batch answers from one consistent world. A write reads a snapshot built from what its transaction has staged (`Config.Staged`), so each write in a batch sees the ones before it. The per-app write handlers not yet converted keep reading `Cache.Model()`.

## Types

A package declares its types against its own model in its `resources.go`, as `api.Type[*Model]`, and `internal/app` lifts each onto the snapshot with `api.Lift`. A type has:

- `Has` - whether an ID is one of its rows, whoever is asking.
- `Get` - the row as the viewer may see it, after the owner's redaction rule, or false when they may not see it at all. Its JSON is the resource's fields; the envelope adds `id`, `can` and any included relations.
- `List` - the IDs of the collection the viewer may see.
- `Aliases` - alias to ID: slugs for friendly URLs and IDs the row used to have. Aliases only name public addresses; references between rows are always IDs. An alias is unique within its type; the owner's load refuses duplicates.
- `Relations` - name, target type, one or many, and the function listing related IDs for the viewer. A relation crossing packages the owner can't import is added in `internal/app` with `Registry.Relate`.
- `Filters` - named functions narrowing a collection. A filter the owner didn't declare is a 400.
- `Actions` - each a `Can` rule and a `Do` handler, both required at registration, so a button can't exist without its route or a route without its flag. `can` on a resource is every action's `Can`.
- `Create` - optional.

## Reads

- `GET /api/{type}/{id-or-alias}` - a path segment resolves as an ID of that type, then as an alias of it. IDs parse case-insensitively.
- `GET /api/{type}?{filters}` - a collection. Besides the type's own filters, `can=<action>` keeps what the viewer may do that to, and `mine` keeps what the resource's `me.mine` says is theirs.
- `GET /api/r/{id}` - any resource by ID, whatever its type; IDs are unique across every type. Aliases don't resolve here.
- `POST /api/query` with `{"name": {"path": "/api/..."}}` - several reads from one snapshot, one `included`.
- `include=a,b.c` on any read - dotted relation paths. An include only reaches what a direct fetch would.

Every read answers `{"now", "data", "included"}`: `now` is the server's clock in the school's zone, `data` the ID or IDs asked for (by entry name in a batch), `included` every resource reached, keyed by type then ID, each once.

`GET /api/me` answers the viewer's address, `email`, and every allowance they hold, `allowances`, a plain list of names.

## Writes

`POST /api/{type}` creates and answers the new `id`; `POST /api/{type}/{id}/{action}` acts; `DELETE /api/{type}/{id}` is the `delete` action. The resource must be visible to the viewer. `POST /api/act` takes a list of writes, each `{"method", "path", "body"}` in the same forms, and runs them in order as one change: all go in or none do, a refusal naming the write that failed, and the answer lists each write's result (`id` for a create). A single write is a batch of one.

`Do` and `Create` get an `api.Write`: the request, the transaction, the snapshot as the batch has left it, the query, the resolved ID, the raw body (`Decode` reads it) and `Taken`, the check a minted ID must pass. They call the package's `writes.go` and stage its operations into the transaction (`Store.Stage`, `docs/storage.md`), never commit; mail and logging go in `Tx.After`, which runs only once the change is in (`docs/dev.md`, The rules live with the thing).

## Client

`web/common/data.js`: `query(path)` and `batch({name: path})` each answer a result of their own, holding that read's `data` and `now` and `get(id)`, `all(type)` and `follow(resource, relation)` over that read's `included` only. There is no page-wide store: a page keeps its read's result and replaces it when it reads again, and a picker keeps its own, so no read ever disturbs another. `act`, `create` and `remove` write, and `actAll` sends a batch to `/api/act`, after which the page reads again; `me()` is `/api/me`.

`web/common/directory.js` is the directory every picker and Spoof Mode read: `directory()` reads `/api/people?listed` with each person's `partners`, `children`, `parents` and `siblings` once per page, `listed()` is those people, and `contactLine` is the line under a name (a student's grade and classroom, a parent's children, anyone else's `words`).

## Staff Birthdays

`internal/birthday/resources.go` registers `birthdays`, `donations`, `birthday-notes`, `charities`, `newsletter-dates`, `birthday-team`, `birthday-settings` and `birthday-invites`, declared against `birthday.World` - the Birthdays model with the directory - which the snapshot builds once. Everything but the settings is seen by the birthday team only (`Model.Sees`); anyone may read the settings resource, which answers how the viewer stands with the team (`me`) and the viewer's own person (`viewer`), and carries the settings themselves for the team alone. A birthday is one per directory-listed staff member, whether or not the sheet has a row for them yet (`missing`), and resolves by the person's addresses and slug; its dates, `stage` and `urgency` are the server's. A donation, a note and a team row have IDs derived from their keys; charities, newsletter dates and invites are minted, and charities and newsletter dates resolve by name and by date. Invites are created, never edited or deleted: the server fills in each one's sending.

## The directory

`internal/who/resources.go` registers `people`, `families`, `classrooms`, `grades`, `crews` and `departments` (in the directory's order). Every member sees all of them, as every member sees Who?: consent masking and opt-outs are applied when the directory loads, so no rule depends on the viewer. A person with no real address has no `email` and is left out of `people?listed`. A person resolves by their address, any Email Aliases address of theirs, and their `slug`, the part of their address before the @, unless someone else's address has the same part, when only the full address resolves. Classrooms and grades resolve by the slug in their Who? page's path.
