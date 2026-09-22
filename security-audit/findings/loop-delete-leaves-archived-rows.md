Description: Deleting a Loop group leaves its `Archived` rows in the sheet, which the next load refuses, so any signed-in account can - with three requests - stop Loop refreshing and make the whole binary refuse to start on its next deploy or cold start.
Status: fixed
Severity: high
---
`deleteGroup` (`internal/loop/handlers.go`) deleted the group's rows from Managers, Rules, Additions, Excluded and Aliases but not `archivedTab`; the in-memory tables did drop them (`internal/loop/loop.go`, `withoutGroup`), so memory and sheet parted ways. `archivedTab` sat instead in `saveGroup`'s delete list, where every save wiped the sheet's Archived rows for the group while memory kept them. `BuildModel` refuses an Archived row naming a group the Groups tab lacks (`loop.go`, the Archived loop), and at startup that refusal is `logging.Fatal("load loop data")` (`internal/app/app.go`).

As any account: `POST /api/loop/group` with a new name, `POST /api/loop/archive {name, archived: true}`, `DELETE /api/loop/group {name}`. The five-minute refresh then failed, and the next revision never came up until someone removed the row by hand. A manager deleting a group anyone had archived did the same without meaning to.

Fixed by moving `archivedTab` from `saveGroup`'s delete list to `deleteGroup`'s, so a delete removes the group's Archived rows and a save leaves them alone; sheet and memory now agree in both handlers. `BuildModel`'s refusal and the fatal startup stay as they are, deliberately: a sheet that disagrees with itself is a logic bug, and being down is preferred to running on it.

A sheet where a group has already been deleted may hold orphaned Archived rows from before this change; those need removing by hand before the next deploy.
