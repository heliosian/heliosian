# Data model

Why the data is shaped the way it is. The tabs, columns, pipeline steps and validation rules are in `internal/directory`; this file carries only what reading that code cannot tell you.

Structured data lives in two Google Sheets in the community shared drive, reached through drive membership rather than project IAM; blobs are objects in the media bucket, reached through project IAM. Each has exactly one home, and the organized model is held in memory — nothing computed is ever written back.

## Veracross is a moving target

Two import tabs mirror what Veracross exports, and the shape of that export is not ours to control.

**Homeroom is no longer exported.** It appears in no student export and on no rendered page, surviving in the portal only as the identity of the by-homeroom directories. The export tool reconstructs it by crawling those directories, which recovers the classroom but not the crew — the portal has no directory at crew granularity. So a field that once arrived as a compound `Crew Classroom` string now arrives as the classroom alone, and every crewed student's crew has to be carried in Overrides by hand. Getting Veracross to restore the field would delete both the crawl and that hand-maintenance.

**Student emails are blank for roughly a fifth of students**, across all grades, so the school's own address is not a reliable key on its own. Name to Email closes that, and a row there with a blank address is an affirmative record that the person is deliberately left out — the tab needs an entry for them precisely because silence is indistinguishable from nobody having looked.

**The homeroom split is positional, and the school's naming convention is what makes it safe**: crew + classroom compounds are real species names and classrooms are the one-word family, so the last word is always the classroom and anything before it is the crew.

**Adult emails arrive mixed-case**, and household addresses arrive at whatever granularity each family chose to give the school — street-level, city-only, or nothing.

## What the staff import deliberately does not supply

Veracross carries a department for every staff member, and it disagrees with the school's own filing often enough, and unsystematically enough, that importing it would silently refile people. Department, grade band, classroom and crew therefore stay in Overrides. The import supplies only what the export knows for certain: name, job title, email, business phone.

People whose faculty type is `Vendors` are dropped — contractors running a club appear in Veracross but are not community staff.

Staff who are also parents arrive from both imports, and the household copy of such a name often carries a redundant parenthetical the faculty export omits. The two merge on resolved names rather than raw strings for that reason alone.

## The consent form is authored outside this repository

The `Preferences` sheet belongs to a Google Form, and its wording *is* the data. Every value is matched verbatim and anything unrecognized is fatal — a reworded consent sentence, a renamed option, a new option, an unknown column. A form edit surfaces as a refused startup rather than as a family's preference read the wrong way. Two tolerances are deliberate: an empty permission cell legitimately means "share neither", and a response matching nobody is skipped, because the form is open to the whole Workspace domain and a stray answer must not be able to stop the server.

Last submission wins per family **by timestamp, not sheet order**. The 33 seconds in which one family opted out and back in are the whole reason that comparison exists.

Where a two-household student's parents disagree, the stricter answer holds, so the outcome never depends on map ordering.

A family that never submitted is a distinct third state rather than an assumed opt-in. The flag exists so that default can be inverted later without touching resolution.

## Privacy decisions

**Student phone numbers are never shown, whatever any sheet says.** Not a preference, not a flag — not a family's choice to make.

**Tags never reach the model.** `/api/directory/model` serves one shared model to every member, so a tag table folded into it would hand everyone's private groupings to every reader. They are assembled per request from the caller's identity instead, and the model type has no tag field to leak. No tag is visible to anyone but its owner, including the school.

**Opting out also locks the person out**, because viewing the directory requires being in it. They get a permissions error until the school clears the flag, which is a consequence worth stating aloud before anyone sets it.

**Overrides values skip import normalization**, since they are authored after the transform — which is why canonical-value validation has to run on every layer rather than only on the import.

## History that constrains the present

Freshness cannot be read from bucket object generations: moving the media into the bucket reset every generation at once. The refresh dates in Overrides exist because of that, and were seeded from the legacy Glide spreadsheet, which holds the years this app's history does not cover.

A family's key is a hash of its members' addresses, so any membership change produces a new key and resets the family's URL and photo association along with it. That is deliberate — a family that gains or loses a member is a different family.
