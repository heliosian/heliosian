Description: Sharing a tag in Who? mails the chosen person every time the route is called, already shared or not (`tagger.share`), so any member can send anyone in the directory - students included - unlimited mail from Helios Who? with forty characters of their own in the subject.
Status: open
Severity: low
---
`POST /api/directory/tag-share` with `on=1` (`internal/who/tags.go:152-184`) checks that the tag exists and the person is listed, never that they are already a manager of it, then runs `notifyShared` (`internal/who/mail.go:85-114`): "<name> shared the tag "<tag>" with you", from the service's address. Each call also appends another identical row to `Tag Managers` (`flushManager`, `tags.go:210-213`). Nothing counts calls.

Fix: return before the write and the mail when the person already manages the tag, and hold the route to a per-owner window of the kind `feedback.Queue` keeps.
