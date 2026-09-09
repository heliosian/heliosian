# Config sheet

The `Config` spreadsheet in the community shared drive holds the platform settings every app shares: who the super admins are, and the handful of values an admin can change without a deploy. `CONFIG_SHEET` names it, and `cmd/createtabs` lays it out from an empty spreadsheet titled `Config`. The tabs, keys, and validation rules are in `internal/config`; this file carries only what reading that code cannot tell you.

## Tabs

- `Settings` — Key, Value. One row per setting, every key required: the staleness thresholds in years for photos, facts, and family photos; the two privacy links My Privacy sends someone to; the staff color.
- `Super Admins` — Email. Platform-wide, across every app. Never empty.
- `Grade Colors` — Grade, Color. Keyed by grade name, `#rrggbb`.
- `Classroom Colors` — Classroom, Color. Keyed by classroom name, `#rrggbb`. Classrooms change year to year, so a row for one that no longer exists is simply unused.

## Where it is read

`internal/config` loads the sheet into its own cache, re-read every five minutes like every other sheet, and serves it at `/api/config` to every signed-in user. No app holds config on its server side: the directory's client fetches `/api/config` beside its model and looks colors up by grade and classroom name; the servers consult the config cache only for the super admin list, which gates admin tools in every app and is never serialized to anyone but a super admin (`/api/config/super-admins`).

## Editing

The directory's admin page is the editor. Its settings, colors, and super admin panels post to the config API (`/api/config/…`), which mirrors the change into the cached tables, parses the result, and only then applies it in memory and writes the changed cells to the sheet through the shared write queue. A change the rules reject (an unknown key, a non-hex color, a non-https link, an empty super admin list) never reaches the sheet, so the next load cannot fail on it. The sheet can also be edited by hand; a bad edit refuses the next load rather than being read as a default.

Regular admins of the directory may change settings and colors. Only a super admin may change who the super admins are, and a regular admin asking gets the same 403 a non-admin gets.

## Super admins and app admins

Super admins are a separate, disjoint tier from each app's `Admins` tab, not a role flag on one list. An app's admin page shows both merged and indistinguishable, and an edit to that merged list never round-trips a super admin into the app's tab or out of the super admin list.

## Sample data

`sampledata/config/` holds one CSV per tab, listing the sample parent as the only super admin so every admin tool is testable locally.
