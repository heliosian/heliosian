Description: Staff Birthdays' model and every pipeline write answer any signed-in account - a student, or the staff member themselves - with every staff member's birthday, opt-out note and the team's notes about them, where the docs say someone not on the team cannot get past the join question; only the browser draws the question.
Status: fixed
Severity: low
---
`requireTeam` in `internal/birthday/handlers.go` - a role on the Team tab, or an admin - sits on every pipeline write, and `Render` in `internal/birthday/view.go` stops after the viewer, the year, the team and the toolbar's badges for anyone else, so a non-member's `GET /api/birthday/model` carries what the join question needs and nothing of the staff, the charities, the newsletter dates or the settings. `POST /api/birthday/team/join` stays open to anyone signed in, as the question is. `TestStrangerSeesOnlyTheJoinQuestion` holds it.

Heliosian's `Visibility` list is consulted nowhere on the app's host, by design (`docs/home/data.md`: visibility, never access); the Team tab is the app's own record of who may read it.
