Description: Helios Ask opens every document that is not a Loop group's to every asker (`viewer.canRead`), so mail sent to one class's parents list or to `parentsonly` answers a parent of another class, a student, or anyone else signed in.
Status: invalid
Severity: high
---
`canRead` (`internal/ask/access.go`) returns true for every kind but `group`, and nothing compares a `list` or `announcement` document's `Channel` - `hummingbirds.parents`, `falcons.students`, `parentsonly` and the rest (`internal/artifacts/channel.go`) - with the asker. That is the design, not a gap in it.

Ask holds two kinds of mail. A Helios Loop group's mail has a membership, rules and a visibility on file, and `canRead` holds a reader to them through `MailReadableBy`. The rest arrives through the forwarding hook, which has no membership to check against and does not need one: `artifacts.Channel` files a message only when it went to everyone or to a whole class, and that mail is not secret. What went to one class's parents may be read by another class's, and nothing on these lists is kept from students, so a parent, a student and a staff member are meant to read the same documents. The gate is what gets filed, not who reads it.

`docs/ask/artifacts.md`, What is in it and what is not, and `security-audit/README.md`, What a non-admin can read, both say so.
