Description: Loop's reserved names leave out `unsubscribe` (and `team`, the address the portal and Loop's notices send from), so any signed-in account can make a group of that name and receive every mail-app unsubscribe - token, group and address - or the replies to the service's own mail.
Status: open
Severity: medium
---
`reservedNames` (`internal/loop/loop.go:89`) lists the postmaster kind only. `inbound` (`internal/loop/mail.go:399-412`) handles a message to `unsubscribe@loop.heliosian.com` as an unsubscribe and then still hands the same message to `received` for whatever group the recipient resolves to. A group named `unsubscribe` with its maker on it therefore gets a copy of each one: the subject is the token, which decodes to `group|address`, so they learn who leaves which group, hidden groups' names included, and hold a token that never expires and unsubscribes that person again after they come back.

`team@loop.heliosian.com` is the portal's and the feedback mail's default sender and the sender of Loop's bounce notices (`internal/app/app.go:1121-1126`, `mail.go:71`); `docs/deploy.md` expects a `team` group to hold it. Where that group does not exist, whoever makes it receives the replies.

Fix: add `unsubscribe` to `reservedNames`, return after `unsubscribeByMail` for that recipient, and reserve each local part the apps send from.
