Description: The portal lets any signed-in account sign up any well-formed address for an open activity (`team.saveVolunteer`), so a member can put other members on volunteer lists without their say, and make HCA-Team mail a thank-you, a calendar invite and the member's own six thousand characters to an address outside the school.
Status: open
Severity: low
---
`saveVolunteer` (`internal/team/team.go:276-400`) takes `email` from the body and holds it to `emailForm` (`:301-308`). The check that someone else's sign-up is for a co-chair, an admin or the household applies only to a row that already exists (`:379-382`); a new row for anyone passes. The portal then mails the person signed up (`internal/team/mail.go:450-487`), the chairs copied, with the note the caller wrote. The caller cannot remove the row and repeat, so the bound is the number of open activities.

`POST /api/team/volunteer {"id": "<open activity>", "email": "x@elsewhere.example", "position": "Volunteer", "note": "..."}`.

Fix: for a caller who neither runs the activity nor is an admin, require the address to be the caller's own or one of their household's, the rule the edit already keeps.
