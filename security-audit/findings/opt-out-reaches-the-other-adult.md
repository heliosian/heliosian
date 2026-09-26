Description: Who?'s opt-out is authorised by `mayEdit`, which lets an adult edit every adult in their household, so a parent can remove their partner from the directory - and with it lock them out of Who? - where `docs/who/directory.md` says opting out is for yourself or your kids.
Status: fixed
Severity: low
---
`optOut` in `internal/who/upload.go` asked `mayEdit(model, me, "person", key)`, and `mayEdit` is true for any `AdultEmails` member of the caller's families. The override it wrote, `Opted Out`, takes the person out of the model, so `MemberGate` then refuses them, and only an admin can bring them back.

No page called `POST /api/directory/optout` once the privacy card was taken out of the person page, so the route and `optOut` are removed rather than narrowed. Opting out is by request to an admin, who hides the person from Admin Tools (`internal/who/admin.go`), and `docs/who/directory.md` says so.
