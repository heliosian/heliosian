# Config sheet

The `Config` spreadsheet in the community shared drive holds the platform settings every app shares: who the super admins are, and the handful of values an admin can change without a deploy. `CONFIG_SHEET` names it, and `tools/createtabs` reads that variable to lay it out from an empty spreadsheet titled `Config`. The tabs, keys, and validation rules are in `internal/model/config.go`; this file carries only what reading that code cannot tell you.

## Tabs

- `Settings` — Key, Value. One row per setting, every key required: the staleness thresholds in years for photos, facts, and family photos; the two privacy links My Privacy sends someone to; the staff color.
- `Super Admins` — Email. Platform-wide, across every app. Never empty.
- `Grade Colors` — Grade, Color. Keyed by grade name, `#rrggbb`.
- `Classroom Colors` — Classroom, Color. Keyed by classroom name, `#rrggbb`. Classrooms change year to year, so a row for one that no longer exists is simply unused.
- `Signed Out` — Email, Time. When each address last signed out, one row per address, the time in RFC 3339. A session cookie is the signed address and the moment it was issued; sign-in refuses one issued at or before the address's row here, so signing out ends every copy of the session - another browser's, a lost phone's - and not only the one that clicked. A row older than a session's length (`sessionLength` in `internal/auth`) is dead weight, since no cookie that old verifies anyway.
- `Change Log` — what every changed cell held before, written by the store alone (`docs/storage.md`, Change Log).

## Where it is read

The sheet is the config part of the one store (`docs/storage.md`), `Model.Config`, re-read at every refresh like every other sheet. Helios Who? reads none of it: its settings are the data model's `SETTING` rows and its colors each classroom's and grade's `GROUP.color` (`docs/datamodel.md`). The apps still on the old model read the grade colors (Loop's, When's and HCA-Team's settings resources) and the classroom colors (When's), the toolbar's update count reads the staleness thresholds (`model.Alerts`), and `/optin` on every host redirects to the opt-in link; the servers consult the config part otherwise only for the super admin list (`Model.Config.SuperAdmins`, `Model.IsSuperAdmin`), which gates admin tools in every app and is never serialized to anyone but a super admin (the `super` admin list, `docs/api.md`, Admin lists), and for the sign-out times, which every app's sign-in (`auth.Sessions`, which is `model.Store` itself: `SignedOut` and `SignOut`) checks on every request and which are serialized to nobody.

## Editing

The super admin list is edited in Who?'s Admin Tools, its Super Admins card being the `super` admin list, which commits the changed rows as the admin; the Change Log keeps what each changed cell held. An empty list is refused whole and never reaches the sheet. Only a super admin may see or change who the super admins are; to anyone else the list does not exist.

No app edits `Settings`, `Grade Colors` or `Classroom Colors`: Who?'s settings and colors panels write the data model. They are edited by hand in the sheet, and a bad edit (an unknown key, a non-hex color, a non-https link) refuses the next load rather than being read as a default.

The `Signed Out` tab is written by sign-out itself: every app's `POST /auth/logout` records the moment for the address really signed in - the admin, under a spoof, never the person viewed as - as a commit made by that address, and answers 500 with the browser's cookies left alone when the record refuses, so a session is never half ended.

## Super admins and app admins

The platform's super admins are the data model's `super-admins` group (`db.Model.SuperAdmin`, and `super_admin` in the policies, `docs/datamodel.md`), which Spoof Mode and everything on the data model check; new code checks that group. The `Super Admins` tab here serves only the old apps' code that already reads it.

Super admins are a separate, disjoint tier from each app's `Admins` tab, not a role flag on one list. An app's admin page shows both merged and indistinguishable (the app's `admin-lists` resource, `docs/api.md`, Admin lists), and an edit to that merged list never round-trips a super admin into the app's tab or out of the super admin list.

## Sample data

`sampledata/config/` holds one CSV per tab, the Change Log included, listing the sample parent as the only super admin so every admin tool is testable locally.
