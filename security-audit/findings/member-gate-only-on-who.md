Description: Only Who? sits behind `who.MemberGate`, so a school account the directory does not list - a family that opted out or never answered the consent form, someone who has left inside their 30-day session - reads the whole directory, and acts, through every other app.
Status: open
Severity: high
---
`internal/app/app.go:1440-1447` wraps `core.Gate` (the member gate) around Who? alone; home, team, birthday, celebrate, calendar, loop and ask take any valid session. Who? turns such a person away with the no-access page (`internal/who/who.go:60-77`); the others hand them the same data:

- `GET /api/team/people` (`internal/team/team.go:269`) and `GET /api/celebrate/people` (`internal/celebrate/handlers.go:187`): everyone's name, address, phone, parent contact addresses, spouses and children with grades (`app.go:335-355`, `:556-568`).
- `GET /api/loop/model` (`internal/loop/handlers.go:381`): `People`, the same list with each parent's children.
- Helios Ask: `viewer()` (`internal/ask/tools.go:103-117`) carries on with no person, and `find_people`, `get_person`, `get_family`, `get_classroom` and the document search answer in full.
- `/photos/{name}` on every one of those hosts (`internal/blob/blob.go:219-261`).

They can also write: make a Loop group and mail it, sign people up on the portal, work the birthday pipeline, spend on Claude.

Fix: wrap every app's handler in the member gate in `Production` (and the sample assembly), with the same exemptions Who? has for `auth.Public` paths, sign-out and each app's admin routes.
