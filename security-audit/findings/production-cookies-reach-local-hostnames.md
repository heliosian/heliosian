Description: Production's session and spoof cookies are set for `heliosian.com` whole (`auth.cookieDomain`), and `*.local.heliosian.com` under it resolves to the reader's own machine, so whatever listens on a loopback port there - another account on a shared family computer, any local program, a developer's own dev server - is same-site with production, can plant cookies for it, and is sent the session once its certificate is accepted.
Status: open
Severity: high
---
`cookieDomain` (`internal/auth/auth.go:125-135`) scopes the cookie one label up: `who.heliosian.com` gives `Domain=heliosian.com`, which covers every name under the apex, `anything.local.heliosian.com` on any port included - cookies know nothing of ports. Public DNS answers `127.0.0.1` for `*.local` (`docs/deploy.md`, Domain; `docs/dev.md`, Hosts and files). Three things follow, in order of how little they need:

- With no certificate at all, a plain-HTTP page at `http://x.local.heliosian.com:<port>` - which any web page can send the browser to - can set cookies with `Domain=heliosian.com`. It cannot replace the `Secure` session, but it can set `g_csrf_token`, the only thing `login` (`auth.go:162-167`) holds a sign-in exchange to, and so post its own Google credential to `/auth/login` for the victim: the victim is then signed in as the attacker's school account and what they type lands there.
- A page served over HTTPS from a local name is same-site with production, so `SameSite=Lax` - the whole of what stands against request forgery, there being no origin check - does not apply to it: it can post to every app's API with the victim's session.
- The `Secure`, `HttpOnly` session and spoof cookies are themselves sent to `https://x.local.heliosian.com:<port>`. That takes a certificate the browser accepts; a developer has clicked through exactly that warning for the dev server, so a production session goes to whatever is on their port 8080, the capture browser's profile included.

`appFor` (`internal/app/app.go:66-83`) also answers the `local` tier in the production binary, which nothing in production needs.

Fix: a second registered domain for development, pointing at loopback, with nothing of production under it; remove the `*.local` record from `heliosian.com`; take the development domain into `appFor`, `cookieDomain`, `logoutDomains`, `internal/devtls`, the OAuth client's origins and the browser Maps key's referers, and have the production binary answer production's names only. `__Host-` cannot be used while one sign-in covers every app, so the domain is the boundary.
