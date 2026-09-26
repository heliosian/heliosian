Description: Sharing a tag in Who? mails the chosen person every time the route is called, already shared or not (`tagger.share`), so any member can send anyone in the directory - students included - unlimited mail from Helios Who? with forty characters of their own in the subject.
Status: fixed
Severity: low
---
`POST /api/directory/tag-share` with `on=1` (`tagger.share` in `internal/who/tags.go`) checked that the tag exists and the person is listed, never that they already manage it, then ran `notifyShared`: "<name> shared the tag "<tag>" with you", from the service's address. Sharing off and on again sent another each time, and nothing counted calls.

Fixed by sending no mail on a share at all: `notifyShared` and `internal/who/mail.go` are gone, and with them Who?'s mailer (`WhoMail`, `WHO_MAIL_FROM`), which sent nothing else. A shared tag shows up under the manager's Shared Tags.
