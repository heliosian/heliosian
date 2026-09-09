# Data model

The tabs, columns, and validation rules are in `internal/events`; this file carries only what reading that code cannot tell you.

The portal's data lives in one Google Sheet, `Events`, in the community shared drive, reached through drive membership like the other sheets. `EVENTS_SHEET` names it, and `cmd/createtabs` reads that variable to lay it out from an empty spreadsheet titled `Events`.

## Tabs

- `Categories` — Title, Description. Row order is display order.
- `Activities` — Year, Title, Category, Status, Description, Image, Timing, Start, End, Location, Spots, Co-Leader Needed, Volunteers Hidden, Direct Sign-Up, Added By, Added.
- `Roles` — Year, Activity, Parent, Title, Group, Status, Description, Image, Start, End, Spots, Co-Leader Needed, Volunteers Hidden, Added By, Added.
- `Volunteers` — Year, Activity, Role, Email, Position, Note, Added By, Added. A blank Role is the activity itself.
- `Links` — Year, Activity, Role, Title, URL, Image.
- `Settings` — Key, Value: `Expense Form URL` and `Intro`, both required.
- `Admins` — Email.
- `Change Log` — appended on every change; never read back.

## Keys are names

Nothing carries an opaque id. An activity is its year and title; a role is its year, activity, and title; a volunteer is those plus an email. The sheet reads as prose, and a person fixing something by hand needs no lookup. The cost is that a rename cascades, and the app pays it: renaming an activity rewrites every role, volunteer, and link naming it, and renaming a role rewrites its sub-roles, volunteers, and links, all in one write batch through the queue.

A role title is unique within its activity across every level of nesting, which is what lets a volunteer row name its role by title alone. The Parent column names another role of the same activity; the loader refuses a parent that does not exist and a chain that loops.

## Why the Glide tables were not kept

The app this replaced held everything in one tree table keyed by row id, with sub-tasks and sub-sub-tasks as rows of the same shape as events, and eleven boolean and rank columns standing in for a status. Volunteers pointed at any node by id, names and photos were copied from the directory into four more tables, and the current year lived in a one-row "key info" table. The status column, the two tabs, the derived school year, and the directory lookup each replace one of those, and the row-id keys became names.

## Dates

Start and End are wall-clock, `2026-09-24 16:00` or `2026-09-24` for a whole day, with no time zone: an event at four o'clock is at four o'clock at the school. The client formats them as local time and the calendar file it downloads uses floating times. Timing is the free text shown when there is no date ("All Year", "Late February"). Added is a date, `2026-09-24`.

## Yes and No

Flag cells are `Yes`, `No`, or blank, and blank means No, so a hand-added row needs only the flags it turns on.

## Status and spots

Status is one of `Pending`, `Open`, `Done`, `Hidden`, spelled exactly so. Spots is a positive count or blank for unlimited; whether something is full is the sign-up count against it, never a flag, so the two can never disagree.

## Images

An Image cell is an object under `activity-images/` in the media bucket, content-addressed and written by the image picker, or a path to a bundled file under `web/hca/` or `web/public/hca/`. A name that resolves to neither refuses the load. A role with no image shows its activity's.

## A load either succeeds whole or refuses

The same stance as the other apps: the server does not start, or does not refresh, on a sheet that breaks a rule, and never serves a page quietly missing an event.

## Sample data

`sampledata/events/` holds one CSV per tab: two school years, a nested role, a pending suggestion, a hidden activity, a private sign-up list, capped spots, and a done event, so every page and rule renders locally.
