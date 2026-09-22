Description: The image pickers show search results straight from Unsplash's, Pexels' and Pixabay's CDNs, so each search sends the searcher's address and the portal's origin to three outside services and the content security policy has to name their hosts as image sources.
Status: open
Severity: low
---
`imagesearch.ServeSearch` (`internal/imagesearch/imagesearch.go:75`) keeps the keys on the server and answers with each hit's `thumb` as the provider returned it: `images.unsplash.com`, `images.pexels.com` and `pixabay.com` addresses. The pickers put those straight into `<img>` tags - `web/team/edit.js:1646`, `web/celebrate/edit.js:386`, `web/calendar/images.js:114`, `web/home/edit.js:773` - so the browser fetches twenty or so thumbnails from the CDN on every search, with the searcher's IP address, user agent and the app's origin as referrer. The picked image alone comes through the server (`ServeImport`) and is served from the bucket after that.

The policy in `internal/app/csp.go` names the three CDN hosts under `img-src` for it. Anything those hosts serve is then an image the pages may show, and each provider is one more party seeing who is editing and when.

Fix: serve the thumbnails through the server - a route that fetches a thumbnail from the three providers' image hosts only, redirects checked, and streams it back with a short cache - and have `ServeSearch` return that route's address as `thumb`, so `img-src` drops the CDN hosts and no editor's browser reaches a provider. Unsplash's API guidelines ask for its returned addresses to be used directly; the community's exposure comes first.
