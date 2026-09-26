Description: Heliosian's front page model carried each section's and link's audience `Rules` to everyone in that audience, not only to admins, so a member read the owning admin's address, the names of their private or shared tags, and the search words that decide who sees what.
Status: fixed
Severity: low
---
`model` in `internal/home/home.go` sends a section's and a link's `Rules` only to an admin and empty to everyone else, and `appViews` attaches an app's `Visibility`, rules and all, only for an admin. Only the admin's editor reads them; the page uses `ForMe`.

`TestOnlyAdminsGetTheRules` in `internal/home/audience_test.go` holds it.
