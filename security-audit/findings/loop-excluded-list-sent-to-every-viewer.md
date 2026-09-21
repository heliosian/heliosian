Description: Loop's model sends each visible group's whole `Excluded` list - addresses, the manager's notes, and "Unsubscribed by mail from <other address>" - to every member who can see the group, where the page shows a non-manager only a count.
Status: open
Severity: medium
---
`groupView` embeds `Group` (`internal/loop/handlers.go:184-195`), whose `Excluded []Excluded` is `{email, note, when}` (`internal/loop/loop.go:100-104`, `:124`), and `view` (`handlers.go:347-354`) fills it the same for a manager and for anyone `VisibleTo` lets see the group. The notes hold a manager's hand-typed reasons and, for a mail unsubscribe, the address it came from (`internal/loop/unsubscribe.go:211`). The page gives a non-manager "N addresses are kept off" (`web/loop/pages/group.js:966-968`).

`GET /api/loop/model` as any member, then `groups[].excluded` on a group everyone sees.

Fix: in `view`, for a viewer who neither manages the group nor is an admin, send the count and the viewer's own `unsubscribed` and leave the list empty.
