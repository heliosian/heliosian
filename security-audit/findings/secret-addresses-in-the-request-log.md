Description: The request log writes the whole request address (`logging.Requests`), so every feed token, outside guest's token, unsubscribe token and `?calendar=` token lands in Cloud Logging, where anyone who can read the project's logs can lift a feed of someone's private events or act as an invited guest.
Status: open
Severity: low
---
`internal/logging/logging.go:163-166` logs `r.URL.RequestURI()` for every request that is not media. `auth.Wrap` passes `/open/` and `/ext/` straight through to it, so `/open/feed/{token}.ics`, `/ext/{token}`, `/open/ext/{token}`, `/open/unsubscribe/{token}` and Heliosian's `?calendar=<token>` are all recorded, the token being the whole of the check on each. Feed and unsubscribe tokens never expire.

Cloud Run's own request log holds the same addresses, so the app's record is one of two copies; closing the app's alone does not close it.

Fix: log the route's pattern, or the path with the segment after `/open/feed/`, `/ext/`, `/open/ext/` and `/open/unsubscribe/` cut, and drop the query; and add a log exclusion (or a shorter retention) for those paths on Cloud Run's request log.
