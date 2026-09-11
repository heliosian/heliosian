# Data model

The tabs, columns, and validation rules are in `internal/events`; this file carries only what reading that code cannot tell you.

The portal's data lives in one Google Sheet, `Events`, in the community shared drive, reached through drive membership like the other sheets. `EVENTS_SHEET` names it, and `cmd/createtabs` reads that variable to lay it out from an empty spreadsheet titled `Events`.

## Tabs

- `Categories` — Category ID, Event ID, Title, Description, Image, Allow Adding. Row order is display order within a scope. A blank Event ID makes a heading on the Opportunities page; an Event ID makes one of that root event's own categories, which the things under it are grouped by.
- `Activities` — Event ID, Year, Title, Parent, Category, Status, Description, Image, Timing, Start, End, Location, Spots, Co-Leader Needed, Volunteers Hidden, Direct Sign-Up, Added By, Added. Parent is an Event ID. Category is a Category ID: a page heading for a root, one of the root event's own for anything under it (or blank).
- `Volunteers` — Event ID, Email, Position, Note, Added By, Added.
- `Links` — Event ID, Title, URL, Image.

- `Settings` — Key, Value: `Expense Form URL` and `Intro`, both required.
- `Admins` — Email.
- `Change Log` — Timestamp, Actor, Action, Kind, Year, Activity, Title, Email, Details; appended on every change, never read back.

**Column order does not matter, column names do.** Every write places each cell under the column of that name wherever the tab keeps it (`Writer.AppendCells`), so columns may be rearranged by hand. The tabs that are read back must have exactly the columns listed - a missing one and an extra one both refuse the load, since a misspelt header would otherwise be ignored silently. Tabs the app never reads (`Change Log`, and any tab kept for backup such as the old `Roles`) are left alone.

## One table, one tree

There is no separate table of roles. A committee, a booth, or a shift is an activity like the event it sits under, distinguished only by naming that event in its `Parent` column; the roots — the things with no parent — are what the opportunities page lists. The tree nests to any depth: a booth can hold its own performance slot. `Group` is the sub-heading a child lists under on its parent's page and means nothing on a root. A child needs no `Category`: it takes its root's for tinting and filing.

`Parent` is the Event ID of another activity **in the same year**; the loader refuses a parent that does not exist, a row that is its own parent, a parent in another year, and a chain that loops. Deleting something with children is refused, since the next load would refuse the orphaned rows. Moving a root to another year takes its whole tree with it.

## Two kinds of category

A category row with no Event ID is a heading on the Opportunities page, and only a root activity may name one. A root whose Category is blank, or names an id that is not a heading, is shown under a built-in **Uncategorized** heading (id `uncategorized`), which appears last and only while something needs it; it cannot be edited, reordered or deleted, and saving a root as Uncategorized stores a blank. A child whose Category names anything but one of its own event's categories is treated as having none. A row with an Event ID belongs to that root event: the things under the event — at any depth — are grouped by these on its page, in their row order, with the uncategorised ones last. Each event manages its own from its page (the Edit Categories button while editing); the page's headings are managed from Admin Tools. Whoever runs an event may change its categories; the page's need an admin. A category never moves between scopes once made.

`Allow Adding` says whether people who do not run the thing may propose new items into a category — a booth into "Place & Culture Booths", an idea into "Just an Idea". Editors of the event (admins, for the page) can always add. The loader refuses a root naming an event's category, a child naming a page heading or another event's category, and a scoped category whose Event ID is not a root.

Copying an event to the next year copies its categories too, under fresh ids, and the copied children point at the copies.

## Keys are ids

Every activity has an `Event ID` and every category a `Category ID`, unique across the sheet, and everything that refers to one does so by id: a child's `Parent`, a root's `Category`, and the `Event ID` on every volunteer and link row. Titles are just titles — two booths under different events can both be "Set Up Crew", a rename touches one cell and nothing else, and every node lives at `/activities/{id}`. The app mints an id when it creates a row (eight characters from a 32-symbol alphabet); rows added by hand need one too, and the loader refuses a row without one or two rows sharing one.

The cost is that the sheet no longer reads as prose on its own — a volunteer row says `E017`, not "Clean Up Crew" — so the Change Log keeps writing titles alongside ids for the humans who read it.

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
