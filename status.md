# Status

Handoff for the Veracross import and multi-photo work. Read `docs/data.md` for why the
data is shaped as it is; this file is where things stand and what to do next.

## The two repositories

`heliosian` is the directory app. `vcexport` is a sibling checkout (`../vcexport`) that
scrapes the Veracross parent portal and writes CSVs plus photos; `tools/import` runs it
and applies the result to the sheet.

## Committed locally, not pushed

The multi-photo change is `heliosian` `97cde89` and `vcexport` `b3d035b`. Both repositories
are one commit ahead of their remote, so this work exists only on this machine until
somebody pushes. Both vet clean and their tests pass.

**Do not deploy `97cde89` yet.** See the migration below: the read path looks in `photos/`
and `pronunciation/` while every existing object is still under `people/` and `families/`,
so a deploy now serves a directory with no photos in it at all.

## The design

Every blob is content-addressed: `photos/<sha256>.<ext>` and
`pronunciation/<sha256>.<ext>` in `gs://heliosian-media`. The bucket says nothing about
who owns what — **the sheet is the index**:

- `Photos` tab (`Email`, `Photo Name`) lists a person's uploads, N rows each.
- Overrides carries `Veracross Photo`, `Primary Photo`, `Pronunciation`, `Family Photo`
  and `Family Pronunciation`, each holding one object name.
- Clients fetch `/photos/{name}` and `/pronunciation/{name}`. No folder, no owner, no
  `/blob` prefix.

A name with no object behind it is fatal: index and bucket disagreeing is a bug, not an
absence. Uploads never overwrite, so one person's upload cannot destroy another's photo
even when two people share an identical image.

## What is done

The read path, the write path, the sheet structure, and vcexport's output. `createtabs`
has been run against the live sheet, so the `Photos` tab and all five columns exist in
both Overrides and Change Log.

## What is next, in order

**1. Bucket migration — this blocks everything.** Every existing object still lives under
`people/` and `families/`, which the store no longer lists, so *the directory currently
resolves no photos at all*. A one-off tool needs to: list those two prefixes, fetch each
object, hash it, write it to `photos/` or `pronunciation/`, then record the name — a
`Photos` row for a person photo, the matching Overrides cell for the other three kinds.
Do not deploy before this runs.

**2. `tools/import` uploads photos.** It currently leaves them in a temp directory. It
should put each into the bucket and sync `vcexport`'s new `Veracross Photos.csv` into the
`Veracross Photo` column.

**3. The detail-page switcher.** `web/static/directory/app.js` renders `p.photoUrl` in
about fourteen places, which keeps working untouched — `photoUrl` is the primary. Only
the person page needs new UI: let any viewer flip through `p.photos`, and let the person
themselves post `field=primary-photo` to `/api/directory/edit` to choose. This is visual
work; build it to an assessable state and hand off rather than iterating on look.

## Identifiers

`Directory` sheet `1CDU0bk8IjV3KQt72VMGKVgWr8tB-YB3fezP9GWwrx8A`, `Preferences` sheet
`1GNFxv5EaOZ2zTBI9ZYJm36dazjSjOednRefX_9vOVAw`, media bucket `gs://heliosian-media`.

## Verifying

`go vet ./... && go test ./...`, then
`go run ./tools/loadcheck -sheet <directory> -preferences <preferences>` for the real
thing. A healthy load reports 540 people — 182 students, 300 parents, 62 staff — 153
families, and nine classrooms with eight crews across Condors, Ospreys, Egrets and Herons.
