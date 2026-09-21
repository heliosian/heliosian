Description: Helios Ask's mail hook files a message as a school newsletter or announcement on the strength of headers its sender wrote (`artifacts.Channel`), checking nothing about who sent it, so one forged mail to the forwarding mailbox puts an outsider's text into every member's answers and system prompt.
Status: open
Severity: high
---
`internal/artifacts/inbox.go:66-164` verifies Mailgun's signature on the notification and then reads the message; no SPF, DKIM or DMARC result is looked at. `Channel` (`internal/artifacts/channel.go:22-45`) decides kind and audience from `List-Id`, `From` and the recipients, and `mailer` (`:54-61`) is a substring match over the whole lowered `From` header: `From: "veracross.com" <x@elsewhere.example>` or `x@veracross.com.elsewhere.example` files as a `newsletter`, which everyone reads. The title of a recent document goes into every conversation's system prompt (`internal/ask/prompt.go`), and `internal/ask/prompt.md` tells Claude a recent document can override the calendar. `known` (`inbox.go:173-180`) also skips a newsletter whose title and day are on file, so a forgery sent first displaces the real issue.

A mail to a forwarding parent's school address with such a `From` and the subject "School closed Monday" is then the latest word when a member asks whether there is school on Monday; it is also where injected instructions for the model would go. Nothing bounds its size short of 64 MB, each 6000 characters an embedding call and a vector kept in memory.

Fix: match mailers on the parsed `From` address's domain, by suffix; require the forwarding mailbox's own `Authentication-Results` to say `dmarc=pass` for that domain before filing; cap the message's length before embedding.
