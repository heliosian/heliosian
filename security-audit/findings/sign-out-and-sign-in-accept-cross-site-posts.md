Description: `POST /auth/logout` takes no token, reads no cookie and checks no origin, so any page on the internet can sign a member out of every app by submitting a form, and `POST /auth/login` checks only Google's double-submit cookie, so the same page, once that cookie can be planted, signs them back in as someone else.
Status: open
Severity: medium
---
`logout` (`internal/auth/auth.go:216-227`) answers any POST with expired `session` and `spoof` cookies for the host and every parent up to the apex, then redirects to `/`. A form on another site auto-submitted to `https://who.heliosian.com/auth/logout` is a top-level navigation, so the browser takes the deletions though the request was cross-site; it cannot be done from a frame or a `fetch`, so the person sees the login page appear. Nothing is read or written, and they are a click from being back in.

`login` (`auth.go:162-192`) holds the exchange to `g_csrf_token` in a cookie equalling the one in the form, and nothing else about where the post came from. A site elsewhere cannot set that cookie; a page under `*.local.heliosian.com` can (`production-cookies-reach-local-hostnames.md`), and then posts its own Google credential for the victim, who is now signed in as the attacker's school account and whose edits, uploads and questions land there. The forced sign-out is what makes that swap look like a session that ran out.

Alone this is a nuisance; it is written up because it is half of that chain and the check costs a few lines.

Fix: one check in `internal/auth` on both routes - refuse unless `Sec-Fetch-Site` is `same-origin` (or `none`, a form the person submitted themselves), and where the header is missing require the `Origin` host to equal `r.Host`. The login page (`web/public/common/login.js`) uses Google's button in its default popup mode with a `login_uri`, where Google's script on the page itself submits the form, so the post is same-origin; that is worth confirming in a browser before the check ships, since the redirect mode posts from `accounts.google.com` and would need that one origin allowed.
