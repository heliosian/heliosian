Description: Only Who? sits behind `who.MemberGate`, so a school account the directory does not list - a family that opted out or never answered the consent form, someone who has left inside their 30-day session - reads the whole directory, and acts, through every other app.
Status: fixed
Severity: high
---
Directory membership is now checked by sign-in itself, for every host: `auth.Wrap` (and the sample server's `auth.Fixed`) admits a request only when the directory lists the effective identity - the spoof target under a spoof - and otherwise serves the no-access page, or a bare 403 on `/api/` and `/blob/` paths (`internal/auth/auth.go`, `admit` and `deny`). The only paths that still answer someone unlisted are `auth.Public`, `/auth/logout` and `/optin`; the `/admin` and `/api/admin/` exemptions are gone, so an admin without a Person row is refused too. `who.MemberGate` and `Core.Gate` are deleted, so there is one check and no host to forget it; the check sits outside `Files`, so each app's own shell is gated as well.

The no-access page moved to `web/public/common/no-access.html` so every host serves it, and `/optin` is registered on every mux. Birthday team members without a directory row lose access, by decision.

`TestWrapAdmitsOnlyMembers` in `internal/auth/auth_test.go` covers the member, the unlisted, the exempt paths, the spoof and the fixed sign-in.
