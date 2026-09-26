Description: The portal's pre-sign-in redirect carries the rest of a path onto a redirect's target by joining strings (`team.Model.moved`), so where a `Redirects` row sends an old prefix to `/`, anyone can write a `team.heliosian.com` link that lands on a site of their choosing.
Status: fixed
Severity: low
---
`moved` in `internal/team/load.go` answered `r.New + at[len(r.Old):]` for a prefix match. With a row `Old=/v/fair`, `New=/` - which `saveRedirect` accepts - a request for `/v/fair/elsewhere.example` gave `//elsewhere.example`, which `Destination` returned and `Redirected` handed to `http.Redirect`, which leaves a target that parses with a host alone; the browser reads it as another site. It answered before sign-in, to anyone, and the page repeated the move with `location.replace` (`redirectTarget` in `web/team/state.js`). A doubled slash inside the request (`/v/fair//elsewhere.example`) reached the same place however the join was made, since `Redirected` runs ahead of the mux's path cleaning.

`moved` now drops a trailing `/` from the target before carrying the rest along, so `/v/fair/x` goes to `/x`, and `Destination` answers nothing for a path that does not start with a single `/` followed by something other than `/` or `\` (`onSite`). `moved` and `redirectTarget` in `web/team/state.js` do the same. `TestRedirects` covers a `/v/fair` → `/` row with both shapes.
