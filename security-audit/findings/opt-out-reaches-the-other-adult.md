Description: Who?'s opt-out is authorised by `mayEdit`, which lets an adult edit every adult in their household, so a parent can remove their partner from the directory - and with it lock them out of Who? - where `docs/who/directory.md` says opting out is for yourself or your kids.
Status: open
Severity: low
---
`optOut` (`internal/who/upload.go:422-439`) asks `mayEdit(model, me, "person", key)`, and `mayEdit` (`:807-834`) is true for any `AdultEmails` member of the caller's families. The override it writes, `Opted Out`, takes the person out of the model, so `MemberGate` then refuses them, and only an admin can bring them back.

`POST /api/directory/optout` with `key=<partner's address>`.

Fix: in `optOut`, require the key to be the caller or one of the caller's children.
