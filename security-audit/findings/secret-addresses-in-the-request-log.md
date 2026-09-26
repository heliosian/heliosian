Description: The request log writes the whole request address (`logging.Requests`), so every feed token, outside guest's token, unsubscribe token and `?calendar=` token lands in Cloud Logging, where anyone who can read the project's logs can lift a feed of someone's private events or act as an invited guest.
Status: invalid
Severity: low
---
`logging.Requests` in `internal/logging/logging.go` logs `r.URL.RequestURI()` for every request that is not media, so `/open/feed/{token}.ics`, `/ext/{token}`, `/open/ext/{token}`, `/open/unsubscribe/{token}` and Heliosian's `?calendar=<token>` are recorded, as they are in Cloud Run's own request log.

Reading the project's logs takes access to the Google Cloud project, and only super admins have that. The same access reaches the sheets, the buckets and the service account the tokens are minted from, so a token in the log gives a log reader nothing they do not already hold, and far less than it. That is an attacker who has already won, which the audit leaves out of scope.
