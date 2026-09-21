Description: Staff Birthdays' assign route takes any well-formed address as the assignee, so any signed-in account can have the service mail a staff member's name, birthday, last year's charity and note - and later the outreach reminders - to an address outside the school.
Status: open
Severity: medium
---
`assign` (`internal/birthday/handlers.go:229-264`) holds `assignedTo` to `checkEmail` alone and asks nothing of the caller. `mailAssignment` (`internal/birthday/invite.go:54-67`, `:126-181`) then sends the invite with those details, and `internal/birthday/reminders.go:44-71` follows with the filled-in letter when the day comes.

`POST /api/birthday/assign {"email": "<staff>", "assignedTo": "x@elsewhere.example"}`.

Fix: require the assignee to be on the `Team` tab or an admin (or at the least in the directory), and the caller to be one too.
