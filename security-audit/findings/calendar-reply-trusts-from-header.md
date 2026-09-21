Description: Helios When records a calendar reply received by mail as the attendee's answer on the strength of the message's `From` line alone (`calendar.takeReply`), so anyone who can send mail can accept or decline any event as any member or invited guest.
Status: open
Severity: high
---
`internal/calendar/replies.go:55-96` verifies Mailgun's signature on the notification - that Mailgun called - and nothing about the message. `takeReply` (`:114-139`) then needs only that the iCalendar `ATTENDEE` is in the directory (or on the guest list) and equals `fields["from"]`, the `From` header as the sender wrote it. No SPF, DKIM or DMARC result is read. `recordBy` writes the answer on any event `eventFor` finds, which is every public event and every invite-only one by id.

A mail to `when@reply.heliosian.com` with `From: victim@heliosschool.org` and a `text/calendar` part holding `METHOD:REPLY`, `UID:<event id>@calendar.heliosian.com` and `ATTENDEE;PARTSTAT=DECLINED:mailto:victim@heliosschool.org` marks the victim as not coming; `ACCEPTED` puts them on a guest list and, on the first yes, mails them the invite.

Fix: Loop already decides who a message is really from (`internal/loop/rewrite.go`, the topmost Mailgun `Authentication-Results` aligned with the `From` domain); hold a reply to the same test before `takeReply` reads it.
