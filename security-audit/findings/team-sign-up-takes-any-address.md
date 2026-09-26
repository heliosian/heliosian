Description: The portal lets any signed-in account sign up any well-formed address for an open activity (`team.saveVolunteer`), so a member can put other members on volunteer lists without their say, and make HCA-Team mail a thank-you, a calendar invite and the member's own six thousand characters to an address outside the school.
Status: fixed
Severity: low
---
`saveVolunteer` in `internal/team/team.go` refuses a new sign-up for an address the directory does not list (`Directory.Person`), whoever asks, chairs and admins included; an existing row for someone who has since left can still be edited or removed. The sign-up form's people picker in `web/team/edit.js` no longer takes a typed address that matched nobody.

The mail now names whoever wrote the text or took the action: `mailSignUp` in `internal/team/mail.go` labels a note someone else wrote "Note from" and their name instead of "Your note", and the co-chair offer notice says who added the person; `mailRemoved` says who removed a sign-up when it was not the person themselves.

Any member can still sign up any other member of the directory, who is told by name who did it.
