# Config sheet

The `Config` spreadsheet in the community shared drive holds a super admin list, a handful of settings and colors, and when each address last signed out; the server reads only the opt-in link and the sign-out times (Where it is read, below). `CONFIG_SHEET` names it, and `tools/createtabs` reads that variable to lay it out from an empty spreadsheet titled `Config`. The tabs, keys, and validation rules are in `internal/model/config.go`; this file carries only what reading that code cannot tell you.

## Tabs

- `Settings` — Key, Value. One row per setting, every key required: the staleness thresholds in years for photos, facts, and family photos; the two privacy links My Privacy sends someone to; the staff color.
- `Super Admins` — Email. Platform-wide, across every app. Never empty.
- `Grade Colors` — Grade, Color. Keyed by grade name, `#rrggbb`.
- `Classroom Colors` — Classroom, Color. Keyed by classroom name, `#rrggbb`. Classrooms change year to year, so a row for one that no longer exists is simply unused.
- `Signed Out` — Email, Time. When each address last signed out, one row per address, the time in RFC 3339. A session cookie is the signed address and the moment it was issued; sign-in refuses one issued at or before the address's row here, so signing out ends every copy of the session - another browser's, a lost phone's - and not only the one that clicked. A row older than a session's length (`sessionLength` in `internal/auth`) is dead weight, since no cookie that old verifies anyway.
- `Change Log` — what every changed cell held before, written by the store alone (`docs/storage.md`, Change Log).

## Where it is read

The sheet is the config part of the one store (`docs/storage.md`), `Model.Config`, re-read at every refresh like every other sheet. The apps read none of it: their settings are the data model's `SETTING` rows (the toolbar's staleness thresholds are the `platform` settings `Photo Stale Years` and `Facts Stale Years`, `docs/toolbar.md`, The alerts) and their colors each classroom's and grade's `GROUP.color` (`docs/datamodel.md`). The server reads two things from it: the opt-in link, which `/optin` on every host redirects to, and the sign-out times, which every app's sign-in (`auth.Sessions`, which is `model.Store` itself: `SignedOut` and `SignOut`) checks on every request and which are serialized to nobody. The rest is loaded and checked, and read by nothing.

## Editing

No app edits `Super Admins`, `Settings`, `Grade Colors` or `Classroom Colors`. They are edited by hand in the sheet, and a bad edit (an empty super admin list, an unknown key, a non-hex color, a non-https link) refuses the next load rather than being read as a default.

The `Signed Out` tab is written by sign-out itself: every app's `POST /auth/logout` records the moment for the address really signed in - the admin, under a spoof, never the person viewed as - as a commit made by that address, and answers 500 with the browser's cookies left alone when the record refuses, so a session is never half ended.

## Super admins and app admins

The platform's super admins are the data model's `super-admins` group (`db.Model.SuperAdmin`, and `super_admin` in the policies, `docs/datamodel.md`), which Spoof Mode, the feedback queue and every app's admin tools check; the `Super Admins` tab here gates nothing. An app's admins are the super admins and the effective members of its `APP` row's `admins` group (`admin_of` in the policies, `docs/datamodel.md`, Permissions).

## Sample data

`sampledata/config/` holds one CSV per tab, the Change Log included.
