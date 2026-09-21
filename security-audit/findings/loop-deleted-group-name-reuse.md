Description: A deleted Loop group's sent mail stays keyed by its name alone, in Loop's history and in Helios Ask's documents, so any signed-in account that makes a new group of that name reads the old group's subjects, senders, recipients and - through Ask - whole posts.
Status: open
Severity: medium
---
Anyone may make a group of any free name and becomes its manager (`internal/loop/handlers.go:569-596`). `deleteGroup` (`:691-725`) removes the group's own rows and nothing of `Messages`, `Deliveries` or the documents. `history` (`internal/loop/history.go:85-125`) matches both tabs by group name, and `messages` (`:137-152`) asks only whether the caller manages the group that holds the name now. Posts are filed into Ask's documents with `Channel` the group's name (`internal/artifacts/inbox.go:113-122`), and `canRead` (`internal/ask/access.go:14-32`) asks `MailReadableBy` of whichever group holds that name now - true for its manager.

After `board` is deleted, `POST /api/loop/group {name: "board"}` then `GET /api/loop/messages?name=board` lists every old post with each recipient's address and delivery state, and Ask's document search reads the old posts in full. `repliesTo` (`internal/loop/mail.go:245`) also takes the old group's Message-IDs as the new one's threads.

Fix: refuse a new name or alias that `Messages` still holds rows for; or compare the group's `Created` with the message's date in `history` and in `canRead`.
