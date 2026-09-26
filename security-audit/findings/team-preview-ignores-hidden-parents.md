Description: The volunteer portal's signed-out previews judge a thing by its own status alone (`team.previewable`), so anyone on the internet fetching the link or share card of an Open thing under a Hidden or Pending event gets its title, date, blurb and picture, and the hidden event's title in its lineage.
Status: fixed
Severity: low
---
`previewable` in `internal/team/share.go` tested only the thing's own status, where `Model.VisibleTo` walks every thing above it. It gated the Open Graph tags slipped into the sign-in page, `/open/share/{id}.png` and the mail's card picture, and `lineage` and `shareCard` then named the parents and drew the nearest parent's flyer or banner. `needs` had the same gap, so the public "Volunteers needed" tags and card could list an Open thing under a hidden event.

`previewable` now also asks `model.VisibleTo(a, "", false)`, and `needs` skips what that refuses, as `docs/dev.md` asks of a caller with no viewer. `visibleStatus` in `internal/team/view.go` no longer matches an empty viewer against a blank Added By, so a Pending row with no proposer is not visible to nobody. `TestSharePreview` covers an Open child under a Hidden and under a Pending event.
