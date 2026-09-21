Description: The image import (`imagesearch.ServeImport`) fetches whatever URL the request names, redirects followed, so any signed-in account on the portal, Celebrate or When can make the server request internal or arbitrary addresses and read back what answers as an image.
Status: open
Severity: medium
---
`internal/imagesearch/imagesearch.go:275-284` checks only that the URL is http or https with a host, then fetches it with `imageClient` (`:70`), a plain client that follows redirects. `docs/dev.md` says the three stock libraries are the whole of it and a search asked for anywhere else is refused; the import has no such list. The routes are `POST /api/team/images/import` (`internal/team/team.go:83`), Celebrate's (`internal/celebrate/handlers.go:86`) and When's (`internal/calendar/handlers.go:126`), none asking for a role; Heliosian's is admin-only.

`{"url":"http://169.254.169.254/"}` or any address inside the project's network is requested from the server. The answer tells the caller the upstream status ("that site answered 403") against "not a supported image", which makes it a probe, and anything that sniffs as an image is stored in the bucket and handed back by name.

Fix: allow only the image hosts of Unsplash, Pexels, Pixabay and Wikimedia Commons, check the host again in `CheckRedirect`, and ask for an editor on the three open routes.
