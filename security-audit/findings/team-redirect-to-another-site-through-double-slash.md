Description: The portal's pre-sign-in redirect carries the rest of a path onto a redirect's target by joining strings (`team.Model.moved`), so where a `Redirects` row sends an old prefix to `/`, anyone can write a `team.heliosian.com` link that lands on a site of their choosing.
Status: open
Severity: low
---
`moved` (`internal/team/load.go:444-454`) answers `r.New + at[len(r.Old):]` for a prefix match. With a row `Old=/v/fair`, `New=/` - which `saveRedirect` accepts (`internal/team/team.go:1575-1597`) - a request for `/v/fair/elsewhere.example` gives `//elsewhere.example`. `Destination` (`load.go:461-489`) returns it, and `Redirected` (`team.go:116-131`) hands it to `http.Redirect`, which leaves a target that parses with a host alone; the browser reads `//elsewhere.example` as another site. It answers before sign-in, to anyone. The page repeats the move with `location.replace` (`web/team/app.js`).

It needs a row whose target is the bare `/`; an admin sending an old address to an `https://` site is what the tab is for and is not this.

Fix: in `moved`, join with exactly one slash, and have `Destination` refuse a path that starts `//` or `/\`.
