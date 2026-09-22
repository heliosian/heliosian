Description: Staff Birthdays' model and every pipeline write answer any signed-in account - a student, or the staff member themselves - with every staff member's birthday, opt-out note and the team's notes about them, where the docs say someone not on the team cannot get past the join question; only the browser draws the question.
Status: open
Severity: low
---
`model` (`internal/birthday/handlers.go:153`) renders for whoever asks, and `saveBirthday` and the other pipeline routes check nothing past sign-in; `docs/birthday/birthday.md`, Someone not on the team, says a non-member "cannot get past" the join question, but that question is drawn by `web/birthday/edit.js` alone, after the model has arrived. Joining is a click for anyone signed in, so the bypass gains no access the Team tab would refuse; what it loses is the record of who is reading staff birthdays and notes, since a reader who never joins is on no tab.

Fix: a `requireTeam` beside `requireAdmin`, on `model` and every pipeline write, with a non-member's model carrying only what the join question needs; or reword the doc to say the question is a nudge and not a control.
