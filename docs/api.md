# The resource API

One data-centric API, served identically on every app's host, that pages compose instead of each app's own `/api/<app>/` routes. `internal/api` is the machinery and knows nothing about any app; `internal/app` registers the types and lifts them onto the snapshot. Which types are registered is `resources` in `internal/app/snapshot.go`.

## The snapshot

`app.Snapshot` holds every store's model. `store.Queue.OnSwap` runs its hook under the commit lock after every commit's swap and every refresh's swaps, and the hook publishes a new snapshot to the registry (`Registry.Publish`), which also builds the alias index then. A request takes the published snapshot once and reads nothing else, so a batch answers from one consistent world. A write reads a snapshot built from what its transaction has staged (`Config.Staged`), so each write in a batch sees the ones before it.

Every request then reads the snapshot through its scope (`Config.Scope`): the snapshot as that one request sees it, built once from the request's query (its actor and its time) before anything is read, and dropped with the request. Every `Has`, `Get`, `List`, relation, filter, `Can` and `Do` of the request - a whole batch of reads, or each write of a batch over what the writes before it staged - gets that scope, so whatever a type works out from the query, it works out once per request and at the request's own time. `Registry.Taken` outside a request scopes the snapshot at the current time. The per-app write handlers not yet converted keep reading `Cache.Model()`.

## Types

A package declares its types against its own model in its `resources.go`, as `api.Type[*Model]`, and `internal/app` lifts each onto the snapshot with `api.Lift`. A type has:

- `Shape` - a zero value of the struct `Get` answers, required at registration. A `Get` answering any other type fails the read, so the spec below always matches what is served.
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
- `POST /api/query` with `{"name": {"path": "/api/..."}}` - several reads from one snapshot, one set of `resources`.
- `include=a,b.c` on any read - dotted relation paths. An include only reaches what a direct fetch would.

Every read answers `{"now", "result", "resources"}`: `now` is the server's clock in the school's zone, `result` the ID or IDs the read answers with, in the type's order (by read name in a batch), and `resources` every resource reached, the result's and those its includes brought in, keyed by type then ID, each once.

`GET /api/me` answers the viewer's address, `email`, and every allowance they hold, `allowances`, a plain list of names.

`GET /api/openapi.json` is an OpenAPI 3.1 description of every registered type, built from the registry on each request: each type's fields from its `Shape`, its relations, filters, actions and create. `/api/docs` on any host is Swagger UI over it, open to anyone signed in, and its Try it out runs as them. It is `web/common/swagger/`: the vendored `swagger-ui-dist` bundle and stylesheet, since the security policy allows no scripts from elsewhere. The bundle is patched: where an example draws one key of an object keyed by data, it names the key the value schema's `x-additionalPropertiesName` as it stands (`<id>` in `resources`) rather than with a counter appended, in both sample generators (`ae[_>1?o+s:o]=u`); a new copy of the bundle needs the patch again.

`/api/erd` on any host is an entity relationship diagram of the same spec, drawn in the browser from `/api/openapi.json` on each load (`web/common/erd/`): a box per type with its fields, a nested object's fields indented under it, and an arrow per relation from the type that has it to its target, labelled with the relation's name: a single arrowhead for one, a double for many. The spec marks a relation with `x-relation` naming the target type. It draws with the vendored `mermaid.min.js` bundle, unpatched: a to-many relation is drawn as Mermaid's composition and the page redraws that marker (`-compositionEnd`) as the double arrowhead after rendering, and the indent is the Braille blank, U+2800, since Mermaid trims any whitespace from a field's start. A new copy of the bundle needs both checked again.

## Writes

`POST /api/{type}` creates and answers the new `id`; `POST /api/{type}/{id}/{action}` acts; `DELETE /api/{type}/{id}` is the `delete` action. The resource must be visible to the viewer. `POST /api/act` takes a list of writes, each `{"method", "path", "body"}` in the same forms, and runs them in order as one change: all go in or none do, a refusal naming the write that failed, and the answer lists each write's result (`id` for a create). A single write is a batch of one.

`Do` and `Create` get an `api.Write`: the request, the transaction, the snapshot as the batch has left it, the query, the resolved ID, the raw body (`Decode` reads it) and `Taken`, the check a minted ID must pass. They call the package's `writes.go` and stage its operations into the transaction (`Store.Stage`, `docs/storage.md`), never commit; mail and logging go in `Tx.After`, which runs only once the change is in (`docs/dev.md`, The rules live with the thing).

## Client

`web/common/data.js`: `query(path)` and `batch({name: path})` each answer a result of their own, holding that read's `result` and `now` and `get(id)`, `all(type)` and `follow(resource, relation)` over that read's `resources` only. There is no page-wide store: a page keeps its read's result and replaces it when it reads again, and a picker keeps its own, so no read ever disturbs another. `act`, `create` and `remove` write, and `actAll` sends a batch to `/api/act`, after which the page reads again; `me()` is `/api/me`.

`web/common/directory.js` is the directory every picker and Spoof Mode read: `directory()` reads `/api/people?listed` with each person's `partners`, `children`, `parents` and `siblings` once per page, `listed()` is those people, and `contactLine` is the line under a name (a student's grade and classroom, a parent's children, anyone else's `words`).

## Staff Birthdays

`internal/birthday/resources.go` registers `birthdays`, `donations`, `birthday-notes`, `charities`, `newsletter-dates`, `birthday-team`, `birthday-settings` and `birthday-invites`, declared against `birthday.World` - the Birthdays model with the directory - which the snapshot builds once. Everything but the settings is seen by the birthday team only (`Model.Sees`); anyone may read the settings resource, which answers how the viewer stands with the team (`me`) and the viewer's own person (`viewer`), and carries the settings themselves for the team alone. A birthday is one per directory-listed staff member, whether or not the sheet has a row for them yet (`missing`), and resolves by the person's addresses and slug; its dates, `stage` and `urgency` are the server's. A donation, a note and a team row have IDs derived from their keys; charities, newsletter dates and invites are minted, and charities and newsletter dates resolve by name and by date. Invites are created, never edited or deleted: the server fills in each one's sending.

## The directory

`internal/who/resources.go` registers `people`, `families`, `classrooms`, `grades`, `crews`, `departments` (in the directory's order) and `tags`. Every member sees all but the tags, as every member sees Who?: consent masking and opt-outs are applied when the directory loads, so no rule depends on the viewer. A person with no real address has no `email` and is left out of `people?listed`. A person resolves by their address, any Email Aliases address of theirs, and their `slug`, the part of their address before the @, unless someone else's address has the same part, when only the full address resolves. Classrooms and grades resolve by the slug in their Who? page's path; `grades?enrolled` keeps the grades some student is in.

A tag is seen by its owner and the people they share it with, and only while it holds someone: `tags` lists the viewer's own, then those shared with them, with `me.mine` saying which, and relations `owner`, `people` and `managers`. A rule names a tag as `tag:<id>` (`docs/loop/loop.md`, A rule discloses the tags it names).

## Helios Loop

`internal/loop/resources.go` registers `email-lists`, `email-list-members`, `email-list-messages`, `email-list-copies`, `email-list-suggestions` and `loop-settings`, declared against `loop.World` - the Loop model, the directory, the grade colours and the Magic Tags of the snapshot's Celebrate and HCA-Team models - which the snapshot builds once.

- **Membership** is worked out in the request's scope (`World.At`, the snapshot's `at`), at the request's time, for every email list together the first time anything in the request asks (`World.placement`), so a page listing every email list with its members evaluates the rules once. Magic Tags of parties and activities depend on the time: a past party has none.
- **`email-lists`** is every email list the viewer sees (`docs/loop/loop.md`, Visible). An email list resolves by its name and each of its aliases; `slug` is the name. `rules` (each with its `tagLabels`), `additions`, `excluded` and `sent` are there for its managers and the app's admins alone (`Group.Sees`). `memberCount` is the members as the rules, additions and excluded list pick them out now; `me` says whether the viewer manages it, whether the rules or additions place them on it (`member`, whatever the excluded list says), whether they are on its excluded list (`unsubscribed`) and whether they have archived it. A manager the directory does not hold is in `managersOutside` rather than the `managers` relation. Relations `managers`, `members` and `messages`.
- **Writes** on an email list: `POST /api/email-lists` makes one with every field; `edit` lays the fields it is sent over the email list as it stands and saves the whole, writing only what changed (`groupOps`), so the Managers box sends `managers` alone; `delete` removes its posts from Helios Ask once the change is in; `unsubscribe` and `resubscribe` add and remove the viewer's own excluded row, for someone the email list places on it; `archive` and `unarchive` are the viewer's own. Each refuses what would change nothing, so its `can` is exact.
- **`email-list-members`**: one per email list and member, ID derived from both. A directory member is its `person` relation; an addition carries `email`, `name` and `outside`. `reasons` - the rules or the addition that put them on - go to the email list's managers and the admins alone.
- **`email-list-messages`** and **`email-list-copies`** are History: each `sent` message with its delivered, failed and pending counts, its `sender` (or `fromEmail` and `fromName` when the directory does not hold them) and its `copies`, failed first, then pending, then delivered; a copy has its `state`, `when`, its `attempts` and its `person` (or `email`). Managers and admins alone see them. IDs are derived from the email list, the message's row and the address.
- **`email-list-suggestions`**: the viewer's own suggestions (`docs/loop/loop.md`, Suggestion), each an ID derived from the tag's or Magic Tag's key, with the `managers` the email list would start with.
- **`loop-settings`**: one resource for everyone, answering the domain, the grade colours, the Magic Tags the viewer may name in a rule (`magicTags`), the roles and relations a rule offers, and the `viewer`.

Preview and Generate with AI read an unsaved draft and stay Loop's own routes (`POST /api/loop/preview`, `POST /api/loop/describe`); the preview answers each member's address and reasons with a directory member's person ID, or an addition's name.
