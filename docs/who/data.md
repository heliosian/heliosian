# Data model

Why the data is shaped the way it is. The tabs, columns, pipeline steps and validation rules are in `internal/who`; this file carries only what reading that code cannot tell you.

Structured data lives in two Google Sheets in the community shared drive, reached through drive membership rather than project IAM; blobs are objects in the media bucket, reached through project IAM. Each has exactly one home, and the organized model is held in memory — nothing computed is ever written back. The staleness thresholds, privacy links, and grade and classroom colors are not directory data at all: they live in the platform `Config` sheet (`docs/config.md`), and the directory's client reads them from `/api/config`.

The service account can also see the Glide spreadsheets this app replaced. **They are read-only, permanently.** Nothing here writes to them; they are kept as history.

`imports/` is scratch. It is gitignored, its contents are whatever someone last dumped or generated, and it goes stale the moment the sheet changes. Never reason from a file there without regenerating it first — dump the tab you actually care about.

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

Where a two-household student's parents disagree, the stricter answer holds, so the outcome never depends on map ordering.

Only an affirmative opt-in puts a family in the directory. A household that never answered is dropped from the model exactly as an opt-out is, and silence is one of the answers the stricter-wins resolution weighs, so a two-household student whose other parent never answered is dropped too. Staff are the single exemption and only from silence: the form reaches them through being a parent, so a staff member nobody answered for stays listed, while one who opts out is removed like anyone else.

## Privacy decisions

**Student phone numbers are never shown, whatever any sheet says.** Not a preference, not a flag — not a family's choice to make.

**Tags never reach the model.** `/api/directory/model` serves one shared model to every member, so a tag table folded into it would hand everyone's private groupings to every reader. They are assembled per request from the caller's identity instead, and the model type has no tag field to leak. No tag is visible to anyone but its owner, including the school.

**Being out of the directory also locks the person out**, because viewing it requires being in it — an Overrides opt-out, a consent-form opt-out, and a family that never answered the form all get the no-access page in place of the app. Only the first of those is an admin's to clear; the other two are the family's own to change, so the page carries the consent form's link, served from the Config sheet through `/optin` so the school can move the form without a deploy. It is a consequence worth stating aloud before anyone sets the flag.

**Overrides values skip import normalization**, since they are authored after the transform — which is why canonical-value validation has to run on every layer rather than only on the import.

## Media is named by content, and the sheet is the index

Every object in the bucket is named for the hash of its own bytes, under `photos/` or `pronunciation/`. The bucket therefore says nothing about who owns what — the `Photos` tab, the `Families` tab, and the Overrides pronunciation column do, and a name recorded there with no object behind it is fatal rather than treated as an absence, because the two disagreeing is a bug and not a missing file.

The reason is that a name derived from a person could be written twice. Under the layout this replaced, a photo was stored at a name built from its owner's email, so an upload overwrote whatever was there. Content addressing makes an upload additive: two people who upload an identical image share one object, and neither can destroy the other's, because nothing is ever written to a name that already exists.

That a person's photos are a list rather than a slot follows from the same thing. The list is theirs in the order it was added, with the Veracross portrait first, and one of them is marked primary — uploading makes the new one primary, since uploading a photo is a statement about which one people should see.

## History that constrains the present

Freshness cannot be read from bucket object generations: moving the media into the bucket reset every generation at once, and now that an object is named for its bytes there is never a second generation to read. The refresh dates in Overrides and Families exist because of that, and were seeded from the legacy Glide spreadsheet, which holds the years this app's history does not cover.

A family's key is its alphabetically first parent's email address — an assertion, not a shorthand: an adult belongs to at most one household, so the key is unique, and the `Families` tab, the one home of family-level fields, is keyed by exactly that email. A row keyed by any other parent of the household refuses to load rather than silently opening a second place for the same family's fields to live. A kid in two households belongs to two families, each with its own row, photo, and page.
