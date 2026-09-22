Description: Staff Birthdays' model sends every staff member's full date of birth, year included, to any signed-in account - a student, or the staff member themselves - and the edit form shows it to the team, where the docs say the year is never shown.
Status: fixed
Severity: medium
---
A birthday is a month and day, `08-20`, on the sheet, in the model and in every browser: `checkMonthDay` in `internal/birthday/birthday.go` refuses a `Birthday` cell with a year on it at load and at every write, `Year.Occurrence` takes a month and day, and the edit form in `web/birthday/edit.js` is a month select and a day select. No year of birth is kept anywhere; the production `Birthdays` sheet carries the same form.

The join question drawn only in the browser, which this finding also noted, is `birthday-join-question-only-in-browser`.
