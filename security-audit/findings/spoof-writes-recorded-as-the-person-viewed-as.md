Description: Every app takes the actor of a write from `auth.Email`, the person viewed as, so what a super admin changes or sends in Spoof Mode is recorded in the Change Logs, the `Added By` cells and the mail it sends as that person's doing; only the request log, which expires, names the admin.
Status: open
Severity: medium
---
`auth.RealEmail` is read in two places outside sign-in itself: the spoof routes and `logging.Requests` (`internal/logging/logging.go:149`). Every app's `who(r)` resolves `auth.Email(r)` (`internal/who/who.go:39-41`, `internal/loop/handlers.go:110`, `internal/birthday/handlers.go:114`, and the same in team, celebrate, calendar and home), and that one address is the Change Log's Actor, a group's `Created By`, a sign-up's `Added By`, an RSVP's answered-by, and the sender named in mail: Who?'s "<name> shared a tag with you" with Reply-To the spoofed person (`internal/who/mail.go:99-107`), the calendar's invitations and messages from a host.

`docs/toolbar.md` has reads and writes alike run as the person viewed as, by design; what the audit asks is that the permanent record says who really did it, and that mail says who really sent it. Today neither does.

Fix: where an actor is written to a sheet, write "admin as person" when `auth.Spoofing(r)` is set - one helper in `internal/auth` for every app's `logChange` - and hold back or relabel mail sent while a spoof is on.
