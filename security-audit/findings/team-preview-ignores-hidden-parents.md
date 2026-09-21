Description: The volunteer portal's signed-out previews judge a thing by its own status alone (`team.previewable`), so anyone on the internet fetching the link or share card of an Open thing under a Hidden or Pending event gets its title, date, blurb and picture, and the hidden event's title in its lineage.
Status: open
Severity: low
---
`previewable` (`internal/team/share.go:70-72`) is Open or Done on the thing itself, where `Model.VisibleTo` (`internal/team/view.go:188-198`) walks every thing above it. It gates the Open Graph tags slipped into the sign-in page and `/open/share/{id}` alike, and `lineage` (`share.go:89`) names the parents. Hiding an event leaves what is under it Open, and `copyActivity` keeps the children's statuses, so the state arises in ordinary use. Celebrate does it the owner's way: `p.VisibleTo("", false)`.

A signed-out `GET /open/share/<child id>.png`, or the page address of the child, is the demonstration.

Fix: `previewable` is its own status test and `model.VisibleTo(a, "", false)`, as `docs/dev.md` asks of a caller with no viewer.
