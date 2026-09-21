Description: Heliosian's audience rules keep whatever `owner` the request names (`home.checkRules`), so a Heliosian admin - who is nobody in Who? - reads any member's private tags by name: who is in them, through the preview, and which names exist.
Status: open
Severity: medium
---
`checkRules` (`internal/home/home.go:562-586`) fills `Owner` only when it is blank. A rule reads tags as its owner, and Loop guards exactly this with `ownRules` and `sharedRules` (`internal/loop/handlers.go:323-341`, `:396-410`): a rule owned by someone else must already be on the thing word for word. Heliosian has no such check.

`POST /api/apps/audience/preview` with `{"rules":[{"kind":"include","owner":"victim@heliosschool.org","tags":["Carpool"]}]}` answers the count and twelve names (`home.go:604-637`); adding search words walks the rest, and `tagLabels` tells a real tag from a guess. Saved on a link or a section, the rule goes on reading the victim's tag.

Fix: apply Loop's two checks in `checkRules` - better, move them into `internal/filter` so both callers share them.
