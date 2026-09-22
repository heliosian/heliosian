Description: Staff Birthdays' assign route takes any well-formed address as the assignee, so any signed-in account can have the service mail a staff member's name, birthday, last year's charity and note - and later the outreach reminders - to an address outside the school.
Status: fixed
Severity: medium
---
`assign` (`internal/birthday/handlers.go`) refuses an `assignedTo` that is not the caller, on the Team tab (`Model.OnTeam`) or an admin, with 400, before anything is written or mailed. That is the set the UI's picker offers (`web/birthday/edit.js`, `openAssign`), so the server now holds what the browser alone did. `TestPipeline` covers an address off the team and an admin.

The caller is still any signed-in account, since `joinTeam` lets anyone put themselves on the Team tab with one click; gating the caller would add a step, not a barrier.
