Description: Loop's model sends each visible group's whole `Excluded` list - addresses, the manager's notes, and "Unsubscribed by mail from <other address>" - to every member who can see the group, where the page shows a non-manager nothing of it.
Status: fixed
Severity: medium
---
`groupView` embeds `Group` (`internal/loop/handlers.go`, `type groupView`), whose `Excluded []Excluded` is `{email, note, when}` (`internal/loop/loop.go`, `type Excluded`). `view` in `handlers.go` trims a non-manager's copy - the rules and each member's reasons go - and the excluded list now goes with them: the branch sets it empty, so `GET /api/loop/model`, `POST /api/loop/subscription` and `POST /api/loop/archive` answer a viewer who neither manages the group nor is an admin with an empty list and their own `unsubscribed` flag, which is computed on the full group before the trim. The page's only reader of the list, the rule card and the editor, runs for managers alone, so it needs nothing.

The notes were the weight of it: a manager's hand-typed reason, and for a mail unsubscribe the sending address (`internal/loop/unsubscribe.go`, `unsubscribeByMail`), which tied the excluded address to a second one.

Helios Ask copies group fields by hand into its own card (`internal/ask/tools_loop.go`) and never carried the list, so the model endpoint was the one path.

`TestTheExcludedListGoesToManagersAlone` in `internal/loop/subscription_test.go` holds it: a member who unsubscribes gets an empty list back and in the model, and the group's manager sees the entry with its note. `docs/loop/loop.md`, Visible, says what a non-manager sees.
