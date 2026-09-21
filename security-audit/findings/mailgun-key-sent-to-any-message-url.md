Description: `Mailgun.Stored` sends the account-wide Mailgun API key as basic auth to whatever URL it is handed - a hook's `message-url` field, or the `Source` cell of a Loop `Messages` row read at startup - so someone who can hand-edit the Groups sheet, or replay one signed notification, is sent the key.
Status: open
Severity: medium
---
`internal/mail/mailgun.go:160-171` builds a GET to the given URL and `do` (`:52-54`) adds `SetBasicAuth("api", m.Key)`. The URL is never checked. It comes from `fields["message-url"]` in the three hooks (`internal/loop/mail.go:406`, `internal/calendar/replies.go:74`, `internal/artifacts/inbox.go:81`), which Mailgun's signature does not cover - it signs the timestamp and token only (`mailgun.go:217-245`) - and, at startup, from the `Source` cell of any `Messages` row still `received` or `stored` (`internal/loop/mail.go:131-141`, `:302`).

A content manager of the shared drive adds a `Messages` row for a real group with State `received` and `Source=https://elsewhere.example/x`; the next start sends that host the key, which sends as every domain and reads every stored message. A hand edit making the server give up a credential is what the README keeps in scope.

Fix: in `Stored`, refuse a URL that is not `https` on a host ending `.mailgun.net` (the storage hosts Mailgun names).
