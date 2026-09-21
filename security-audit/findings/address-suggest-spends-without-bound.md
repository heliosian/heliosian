Description: `GET /api/address/suggest` is on every app, asks Google's Places Autocomplete on the server's key for each call, and counts nothing, so any signed-in account can run up the Maps bill in a loop; the call also has no timeout.
Status: open
Severity: low
---
`RegisterSuggest` (`internal/geocode/suggest.go:48-67`) is wired onto all eight muxes (`internal/app/app.go:1021-1028`) and needs only a query of three to two hundred characters. `Suggest` (`internal/geocode/geocode.go:63-75`) posts to Places through `http.DefaultClient`, which never times out, so a stalled upstream holds the request for as long as it likes.

Fix: a per-user window of the kind `feedback.Queue` and Helios Ask keep - an address box needs a few dozen calls a minute at most - and a client with a timeout of a few seconds.
