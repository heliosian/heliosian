Description: Helios When gives a guest list set to "Helios guests" to any member who can open the event, invited or not (`calendar.invitesView`), so for a public event - or an invite-only one whose id they have - any member reads who is coming, who has not answered, their addresses and photos.
Status: open
Severity: medium
---
`invitesView` (`internal/calendar/invites.go:668-759`) authorises with `eventFor` (`internal/calendar/rsvp.go:81-95`), which finds every event the viewer's calendar shows and any invite-only or pending event by id. It then fills `Coming` whenever the viewer is a host or `GuestList == GuestListPublic` (`:742`), the default, never asking whether the viewer is on the list. `docs/calendar/calendar.md` words the setting as "Helios guests - everyone invited - or the hosts only". A party's list from Celebrate comes the same way.

`GET /api/calendar/invites?id=<event>` as a member who was not invited.

Fix: for a viewer who is not a host, fill `Coming` only when `view.Mine` is not empty - they, their child or their guest is invited.
