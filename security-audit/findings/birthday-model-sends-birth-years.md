Description: Staff Birthdays' model sends every staff member's full date of birth, year included, with their opt-out note and the team's notes about them, to any signed-in account - a student, or the staff member themselves - where the docs say the year is never shown and the page stops anyone not on the team at a question.
Status: open
Severity: medium
---
`model` (`internal/birthday/handlers.go:153-161`) renders for whoever asks; `StaffView.Birthday` is the sheet's cell as kept, `1980-08-20` (`internal/birthday/view.go:134`), beside `Level`, `LevelNote` and `Notes`. `docs/birthday/birthday.md` says the year "is kept when it is known and never shown". The join question a non-member "cannot get past" is drawn by `web/birthday/edit.js` alone; `GET /api/birthday/model` answers without it.

Fix: send the month and day only (`--08-20` or the two numbers) to everyone, keeping the year on the server; and send the pipeline only to the team and the admins, everyone else getting what the join question needs.
