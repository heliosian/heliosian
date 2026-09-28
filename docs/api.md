# The resource API

One data-centric API, served identically on every app's host, that pages compose instead of each app's own `/api/<app>/` routes. `internal/api` is the machinery and knows nothing about any app; `internal/app` registers the types and lifts them onto the snapshot. Which types are registered is `resources` in `internal/app/snapshot.go`.

## The snapshot

`app.Snapshot` holds every store's model. `store.Queue.OnSwap` runs its hook under the commit lock after every commit's swap and every refresh's swaps, and the hook publishes a new snapshot to the registry (`Registry.Publish`), which also builds the alias index then. A request takes the published snapshot once and reads nothing else, so a batch answers from one consistent world. Write handlers keep reading `Cache.Model()`.

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

`GET /api/me` answers the viewer's address and every allowance they hold, each with whether it counts right now under Super Admin Mode.

## Writes

`POST /api/{type}` creates and answers the new `id`; `POST /api/{type}/{id}/{action}` acts; `DELETE /api/{type}/{id}` is the `delete` action. The resource must be visible to the viewer. `Do` builds nothing itself: it calls the package's `writes.go` and commits, as any handler does (`docs/dev.md`, The rules live with the thing).

## Client

`web/common/data.js`: `query(path)` and `batch({name: path})` fetch, replace the page's store whole and return `data`; `get(id)`, `all(type)` and `follow(resource, relation)` read it; `now()` is the server's clock from the last read; `act`, `create` and `remove` write, after which the page reads again; `me()` is `/api/me`.
