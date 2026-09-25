# Config sheet

The `Config` spreadsheet in the community shared drive holds the platform settings every app shares: who the super admins are, and the handful of values an admin can change without a deploy. `CONFIG_SHEET` names it, and `tools/createtabs` reads that variable to lay it out from an empty spreadsheet titled `Config`. The tabs, keys, and validation rules are in `internal/config`; this file carries only what reading that code cannot tell you.

## Tabs

- `Settings` — Key, Value. One row per setting, every key required: the staleness thresholds in years for photos, facts, and family photos; the two privacy links My Privacy sends someone to; the staff color.
- `Super Admins` — Email. Platform-wide, across every app. Never empty.
- `Grade Colors` — Grade, Color. Keyed by grade name, `#rrggbb`.
- `Classroom Colors` — Classroom, Color. Keyed by classroom name, `#rrggbb`. Classrooms change year to year, so a row for one that no longer exists is simply unused.
- `Signed Out` — Email, Time. When each address last signed out, one row per address, the time in RFC 3339. A session cookie is the signed address and the moment it was issued; sign-in refuses one issued at or before the address's row here, so signing out ends every copy of the session - another browser's, a lost phone's - and not only the one that clicked. A row older than a session's length (`sessionLength` in `internal/auth`) is dead weight, since no cookie that old verifies anyway.
- `Change Log` — what every changed cell held before, written by the store alone (`docs/storage.md`, Change Log).

## Where it is read

`internal/config` holds the sheet as a store (`docs/storage.md`), re-read every five minutes like every other sheet, and serves it at `/api/config` to every signed-in user. No app holds config on its server side: the directory's client fetches `/api/config` beside its model and looks colors up by grade and classroom name; the servers consult the config cache only for the super admin list, which gates admin tools in every app and is never serialized to anyone but a super admin (`/api/config/super-admins`), and for the sign-out times, which every app's sign-in (`auth.Sessions`, the cache itself) checks on every request and which are serialized to nobody.

## Editing

The directory's admin page is the editor. Its settings, colors, and super admin panels post to the config API (`/api/config/…`), which commits the changed rows as the admin; the Change Log keeps what each changed cell held. A change the rules reject (an unknown key, a non-hex color, a non-https link, an empty super admin list) is refused whole and never reaches the sheet, so the next load cannot fail on it. The sheet can also be edited by hand; a bad edit refuses the next load rather than being read as a default.

Regular admins of the directory may change settings and colors. Only a super admin may change who the super admins are, and a regular admin asking gets the same 403 a non-admin gets.

The `Signed Out` tab is written by sign-out itself: every app's `POST /auth/logout` records the moment for the address really signed in - the admin, under a spoof, never the person viewed as - as a commit made by that address, and answers 500 with the browser's cookies left alone when the record refuses, so a session is never half ended. A super admin can end anyone's sessions with `POST /api/config/sign-out` and a body of `{"email": "..."}` on the directory's origin, which commits that address's row as the super admin; a regular admin gets the 403.

## Super admins and app admins

Super admins are a separate, disjoint tier from each app's `Admins` tab, not a role flag on one list. An app's admin page shows both merged and indistinguishable, and an edit to that merged list never round-trips a super admin into the app's tab or out of the super admin list.

## Sample data

`sampledata/config/` holds one CSV per tab, the Change Log included, listing the sample parent as the only super admin so every admin tool is testable locally.
