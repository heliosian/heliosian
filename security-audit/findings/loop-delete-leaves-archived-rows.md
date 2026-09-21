Description: Deleting a Loop group leaves its `Archived` rows in the sheet, which the next load refuses, so any signed-in account can - with three requests - stop Loop refreshing and make the whole binary refuse to start on its next deploy or cold start.
Status: open
Severity: high
---
`deleteGroup` (`internal/loop/handlers.go:714`) deletes the group's rows from Managers, Rules, Additions, Excluded and Aliases but not `archivedTab`; the in-memory tables do drop them (`internal/loop/loop.go:665`), so memory and sheet part ways. `archivedTab` sits instead in `saveGroup`'s delete list (`handlers.go:640`), where every save wipes the sheet's Archived rows for the group while memory keeps them. `BuildModel` refuses an Archived row naming a group the Groups tab lacks (`loop.go:563-567`), and at startup that refusal is `logging.Fatal("load loop data")` (`internal/app/app.go:956-959`).

As any account: `POST /api/loop/group` with a new name, `POST /api/loop/archive {name, archived: true}`, `DELETE /api/loop/group {name}`. The five-minute refresh now fails, and the next revision never comes up until someone removes the row by hand. A manager deleting a group anyone has archived does the same without meaning to.

Fix: move `archivedTab` from `saveGroup`'s list to `deleteGroup`'s. Making Loop's load failure keep only Loop down, as the portal's and Celebrate's do, would bound the next mistake of this kind.
