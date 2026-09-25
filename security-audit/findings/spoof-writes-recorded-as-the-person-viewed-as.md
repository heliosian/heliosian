Description: Every app takes the actor of a write from `auth.Email`, the person viewed as, so what a super admin changes or sends in Spoof Mode is recorded in the Change Logs, the `Added By` cells and the mail it sends as that person's doing; only the request log, which expires, names the admin.
Status: fixed
Severity: medium
---
Every app's Change Log has a Real Actor column holding `auth.RealEmail(r)`, beside Actor, which stays the person viewed as. Each app's `logChange` (home, birthday, calendar, celebrate, team) takes the request and writes it; Loop's takes the address, since unsubscribing by mail has no request, and its subscription toggle and unsubscribe link pass `auth.RealEmail(r)` in; Who?'s `changeLogRow` (`internal/who/upload.go`) fills it from the request. Real Actor is blank only for a change nobody signed in made: the calendar import, and a Loop unsubscribe by link or by mail. Each app's `ChangeLogColumns` and `cmd/createtabs` carry the column, so a live sheet refuses to load until `createtabs` has added it. `TestTheChangeLogNamesWhoIsReallySignedIn` in `internal/loop` holds it.

Mail and the ownership cells (`Created By`, `Added By`, an RSVP's `Answered By`) stay the person viewed as's on purpose: Spoof Mode acts as that person, and those cells decide who may change the row later. `docs/toolbar.md` says so.
