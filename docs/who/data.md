# Data model

Why the data is shaped the way it is. The tabs, columns, pipeline steps and validation rules are in `internal/who`; this file carries only what reading that code cannot tell you.

Structured data lives in two Google Sheets in the community shared drive, reached through drive membership rather than project IAM; blobs are objects in the media bucket, reached through project IAM. Each has exactly one home, and the organized model is held in memory — nothing computed is ever written back. The staleness thresholds, privacy links, and grade and classroom colors are not directory data at all: they live in the platform `Config` sheet (`docs/config.md`), and the directory's client reads them from `/api/config`.

`local/imports/` is scratch. It is gitignored, its contents are whatever someone last dumped or generated, and it goes stale the moment the sheet changes. Never reason from a file there without regenerating it first — dump the tab you actually care about.

## Veracross is a moving target

Two import tabs mirror what Veracross exports, and the shape of that export is not ours to control.

**Homeroom is no longer exported.** It appears in no student export and on no rendered page, surviving in the portal only as the identity of the by-homeroom directories. The export tool reconstructs it by crawling those directories, which recovers the classroom but not the crew — the portal has no directory at crew granularity. So a field that once arrived as a compound `Crew Classroom` string now arrives as the classroom alone, and every crewed student's crew has to be carried in Overrides by hand. Getting Veracross to restore the field would delete both the crawl and that hand-maintenance.

**Student emails are blank for roughly a fifth of students**, across all grades, so the school's own address is not a reliable key on its own. Name to Email closes that, and a row there with a blank address is an affirmative record that the person is deliberately left out — the tab needs an entry for them precisely because silence is indistinguishable from nobody having looked.

**The homeroom split is positional, and the school's naming convention is what makes it safe**: crew + classroom compounds are real species names and classrooms are the one-word family, so the last word is always the classroom and anything before it is the crew.

**Adult emails arrive mixed-case**, and household addresses arrive at whatever granularity each family chose to give the school — street-level, city-only, or nothing.

## What the staff import deliberately does not supply

Veracross carries a department for every staff member, and it disagrees with the school's own filing often enough, and unsystematically enough, that importing it would silently refile people. Department, grade band, classroom and crew therefore stay in Overrides. The import supplies only what the export knows for certain: name, job title, email, business phone.

Veracross's department is carried anyway, in the staff import tab's own `person_department` column, and read by nothing. It is there to be compared with the filing in Overrides — the disagreement above is the reason for the arrangement, and a column nobody has to parse JSON to read is what makes it visible. The Staff page's groupings come from Overrides alone, as they always have.

People whose faculty type is `Vendors` are dropped — contractors running a club appear in Veracross but are not community staff.

Staff who are also parents arrive from both imports, and the household copy of such a name often carries a redundant parenthetical the faculty export omits. The two merge on resolved names rather than raw strings for that reason alone.

## The school's own staff page is a third source

The school publishes a bio, a title and a headshot for its staff on its public website, and `webexport` carries them into the `Website Staff Import` tab. It is the only source of a bio the directory has, and the bios arrive as the school's own HTML — the import flattens them to text, since nothing downstream renders markup.

It is an import like any other and runs before Overrides, which behave exactly as they do over Veracross: a Veracross title stands, an override wins, an override's `-` clears, and an override that only restates what the page publishes is the dead weight the load refuses to carry. That last rule is what makes the import clear the Facts and Job Title overrides the page has caught up with — the ones saying the same thing as the published bio, or that the bio has grown past — the same bargain it strikes with `Name to Email` when Veracross learns an address. An override still saying something of its own survives and keeps winning.

Running before Overrides also means the page reaches nobody it adds: a staff member Veracross does not carry exists only once Overrides has created them, which is after the layer has run.

The page is not the directory's roster. It carries vendors the directory drops and people who have left, and it publishes no address for a few. Those are matched by name: through `Name to Email` for anyone Veracross has no address for either, and against the staff import itself for anyone it has, since `Name to Email` refuses to restate an address Veracross exports. A name two staff share is refused rather than guessed at. An entry matching nobody is counted and skipped, since the alternative is the whole directory refusing to load over somebody else's web page.

## One person, several addresses

Several staff have more than one address in the school's Workspace, and the sources disagree about which one is theirs: Veracross exports one, the staff page publishes another, and the consent form records whichever the person typed. The address Veracross exports is the one the directory keys a person by, and `Email Aliases` maps every other address onto it. It is applied to the sources authored outside this app — both Veracross imports, the staff page, and the consent form — before anything matches on an address, so every layer downstream sees one address per person. The app's own tabs (Overrides, Families, Tags, Photos, Admins) are keyed by that resolved address and are never rewritten, so a row the app writes can never drift from the key it was written under.

Sign-in goes through the same table. Google vouches for whichever address the person's Workspace account calls primary, and which one that is was never anyone's deliberate choice, so the directory resolves the signed-in address before it asks whether they are a member, whose tags to show, or who a write is attributed to. A person is one address to every part of the app, whichever one they arrived under.

An alias is a claim about a source, and a claim nothing bears out is refused: an alias matching no row in any import fails the load, for the same reason a stale `Name to Email` entry does. An alias of an alias is refused too, so resolution is one lookup with nothing to chase.

The headshot sits behind the Veracross portrait in a person's photos, so importing it never changes the picture the directory already shows. The departments the page files people under are carried in the tab and read by nothing: the school's own filing lives in Overrides, for the reason above.

## The consent form is authored outside this repository

The `Preferences` sheet belongs to a Google Form, and its wording *is* the data. Every value is matched verbatim and anything unrecognized is fatal — a reworded consent sentence, a renamed option, a new option, an unknown column. A form edit surfaces as a refused startup rather than as a family's preference read the wrong way. Two tolerances are deliberate: an empty permission cell legitimately means "share neither", and a response matching nobody is skipped, because the form is open to the whole Workspace domain and a stray answer must not be able to stop the server.

Last submission wins per family **by timestamp, not sheet order**. The 33 seconds in which one family opted out and back in are the whole reason that comparison exists.

The form's answer is per family, and a kid's family is every household they belong to. The households a kid links are one family to the form: every submission from any of their adults goes into one pool, the latest wins, and it governs each of those households alike, membership and permissions both. One household opting in therefore lists the other household's parent too, and a later opt-out from either household removes them all, exactly as it does between two parents of one household.

Only an affirmative opt-in puts a family in the directory. A family none of whose adults answered is dropped from the model exactly as an opt-out is. Staff are the single exemption and only from silence: the form reaches them through being a parent, so a staff member nobody answered for stays listed, while one who opts out is removed like anyone else.

## The invite templates live in a sheet

The Invite List Builder's templates are data, not code: its own sheet holds a manifest tab, `_Services`, naming one tab per destination system (Greenvelope, Evite, ...), each carrying that system's own column header and a template row of `{{ parameter }}` tokens the client fills in per family. Adding a system, or matching a system's own header quirk, is a sheet edit rather than a deploy. That quirk - a header naming one column twice - is why those tabs are read as they stand (`Raw` in `internal/data`) rather than by column name, and why they sit outside the store: the app only reads them, on startup and every five minutes after, and a startup that cannot read them refuses to start.

The greetings people add are the one thing the app writes to that sheet. `_Greetings` is a store of its own (`Invites` in `internal/who/invites.go`) with its own `Change Log`, keyed by Name; a greeting is changed or deleted only by the address in its Email column, and a row with none is one of the sheet's own.

## Privacy decisions

**Student phone numbers are never shown, whatever any sheet says.** Not a preference, not a flag — not a family's choice to make.

**Tags never reach what the model serves.** `/api/directory/model` serves one shared model to every member, so a tag table in it would hand everyone's private groupings to every reader. The model holds the Tags and Tag Managers rows only in unexported fields, which the encoder never writes, and each caller's tags are assembled per request from their identity. No tag is visible to anyone but its owner and the managers they name, including the school - except as its owner or a manager discloses it by naming it in a rule on Loop, Heliosian or When, which shows whoever sees that thing who the tag holds (`docs/loop/loop.md`, A rule discloses the tags it names).

**Lists are assembled the same way**, per request from the caller's identity, and never stored: the directory asks the Celebrate and Events caches in the same process (`internal/app/lists.go`, the mirror of the directory adapters those apps read Who through) which parties the caller hosts and which activities they co-chair, using each app's own rule for who runs a thing, so the directory can never show a group the source app would not show that person. Stored host and volunteer addresses are resolved through `Email Aliases` first, which the source apps do not do for their own rows. An app whose sheet has not loaded contributes nothing, as its own pages answer 503 - the directory is not the place to say so. Room-parent lists come from the directory's own `Room Parent` column. The groups the caller manages on Helios Loop come the same way, each evaluated by that app's own rule reader against the directory as it stands (`GroupLists` in `internal/app/lists.go`); the groups' rules never see these lists in turn, so a group cannot name another group.

**Being out of the directory also locks the person out of every app**, because each one shows the directory's people and viewing them requires being among them — an Overrides opt-out, a consent-form opt-out, and a family that never answered the form all get the no-access page in place of every app, sign-in making the check on each request (`docs/dev.md`). Only the first of those is an admin's to clear; the other two are the family's own to change, so the page carries the consent form's link, served from the Config sheet through `/optin` on every host so the school can move the form without a deploy. It is a consequence worth stating aloud before anyone sets the flag.

**Overrides values skip import normalization**, since they are authored after the transform — which is why canonical-value validation has to run on every layer rather than only on the import.

## Media is named by content, and the sheet is the index

Every object in the bucket is named for the hash of its own bytes, under `photos/` or `pronunciation/`. The bucket therefore says nothing about who owns what — the `Photos` tab, the `Families` tab, and the Overrides pronunciation column do, and a name recorded there with no object behind it is fatal rather than treated as an absence, because the two disagreeing is a bug and not a missing file. The server never lists the bucket; it fetches exactly the names the sheets record. The classroom and grade tiles are no exception: an admin's upload is a content-named object under `photos/`, and the `Images` tab (Kind, Name, Image) names it for its classroom or grade, so replacing a tile is a row change with a Change Log entry rather than an object overwritten. A classroom or grade with no row shows the illustration bundled with the app.

The reason is that a name derived from a person could be written twice. Under the layout this replaced, a photo was stored at a name built from its owner's email, so an upload overwrote whatever was there. Content addressing makes an upload additive: two people who upload an identical image share one object, and neither can destroy the other's, because nothing is ever written to a name that already exists.

That a person's photos are a list rather than a slot follows from the same thing. The first in the list is the one the directory shows. Its order is each `Photos` row's `Order` sort key (`docs/storage.md`, Order), never its place in the tab: an upload goes last, a drag writes the moved rows' keys, and a delete removes one row. The Veracross portrait and the staff page's headshot lead the list without a row of their own until someone moves, crops or deletes around them, when they get one.

## Geocoding is cached in the sheet

Every family address on the map was once a Geocoding API call, and a call per address per instance start cost more than the instances did. The `Geocode` tab — Address, Lat, Lng — is that cache. The model build only reads it: a family whose address has a row gets its coordinates, and one without is listed as unlocated and has none for now. The build never waits on the API, so neither does a request. A worker (`Cache.Locate`) wakes after each load and each commit, asks the API for the unlocated addresses, and commits what it finds as `geocoder`, and the map has them from that commit on. The key is the address string exactly as the family record carries it, so an edited address is simply a miss and gets its own row; the row for the old address stays behind, harmless. An address the API cannot answer is asked again at the next wake. The tab is append-only: its rows are never edited, so the Change Log carries none of them. A row with a malformed coordinate refuses the load, since somebody edited it by hand.

## Every write is a commit

The directory spreadsheet is a store (`docs/storage.md`): its tabs and the preferences form's `Sheet1`, read alongside and never written, are the Spec in `internal/who/cache.go`, and every handler, the geocoder and the import state their changes as row operations on it. The Change Log tab holds each cell's previous value.

A person added by hand, flagged `Added` in Overrides, is known by nothing but that row, so the row carries the rest with it: renaming their address renames it on their Tags and Tag Managers rows, as owner, tagged person and manager, and on their Photos rows; deleting the row deletes all of those. A person Veracross carries loses nothing when their Overrides row goes, since they are still in the import.

Renaming or deleting a tag takes its Tag Managers rows along in the same commit. Tags and managers are matched without regard to case, so two tags whose names differ only in case are one tag to every change.

The import (`tools/import`) reads the sheet and the exports, works out the rows that differ in the Veracross tabs, the staff page tab and `Name to Email`, the Overrides cells the staff page has caught up with and the `Name to Email` entries Veracross now has an address for, and commits all of it at once as `import` through a store of its own, waiting for the write. A result the model would refuse is refused whole and writes nothing. The server sees it on its next refresh.

## Refresh dates and family keys

Freshness cannot be read from bucket object generations: an object is named for its bytes, so there is never a second generation to read. The refresh dates in Overrides and Families exist because of that.

A family's key is its alphabetically first parent's email address — an assertion, not a shorthand: an adult belongs to at most one household, so the key is unique, and the `Families` tab, the one home of family-level fields, is keyed by exactly that email. A row keyed by any other parent of the household refuses to load rather than silently opening a second place for the same family's fields to live. A kid in two households belongs to two families, each with its own row, photo, and page.

That email stays inside the loader and the `Families` tab. The model every member fetches, and the family page's address, know the family by a masked form of it instead — `familyID` in `internal/who/load.go`, an HMAC of the email under a key derived from `SESSION_KEY` — since the first parent may since have been withheld, as the silent partner of a staff member listed by default or through the per-person Opted Out cell, and the family stays keyed by them all the same. Keyed rather than hashed, so nobody can check a guessed address against the model. A new `SESSION_KEY` changes every family's address; nothing but a bookmark holds one.
