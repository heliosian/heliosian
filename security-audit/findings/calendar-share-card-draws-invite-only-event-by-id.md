Description: Helios When's public share card and banner, `/open/share/{id}.png` and `/open/banner/{id}`, draw any event's title, date, place and picture for anyone holding its id with no sign-in, so an invite-only event's words are readable by guessing or forwarding an eight-character id.
Status: open
Severity: low
---
`shareCard` (`internal/calendar/share.go:226`) and `banner` (`internal/calendar/invites.go:2182`) resolve the id with `event` (`share.go:103`), which falls through to `Model().Event(id)` for anything not on the public calendar, and both sit under `/open/`, where the token in the address is meant to be the whole check (`docs/dev.md`). For a public event nothing is lost. For an event shared as Invite Only the card carries the title, the day, the hours, the place and the flyer or picture to anyone with the id - not the guest list, and not the description.

`curl https://when.heliosian.com/open/share/<id>.png` signed out.

No plain fix: the card exists so a link previews in a chat app, and an outside invitee's link, `/ext/{token}`, previews through the same card by the event's id - the preview fetcher carries no session and the page's `<meta>` tags name the card by id. Drawing the card by the invite's token instead would tie each preview to one invitee's secret and leave the event's own page with no preview. Left open for the day the trade is worth making.
