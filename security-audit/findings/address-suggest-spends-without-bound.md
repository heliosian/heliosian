Description: `GET /api/address/suggest` is on every app, asks Google's Places Autocomplete on the server's key for each call, and counts nothing, so any signed-in account can run up the Maps bill in a loop; the call also has no timeout.
Status: fixed
Severity: low
---
`geocode.Suggestions` (`internal/geocode/suggest.go`) is built once and registered on all eight muxes, so its state is shared across apps. A query is normalised (lowercased, whitespace collapsed) and answered from an in-memory LRU (`internal/lru`) when it has been asked before; otherwise it counts against a per-account window of 60 calls a minute (`internal/ratelimit`, keyed on the real signed-in address so a super admin's spoof does not move the count), and a call past the limit gets an empty list. `Lookup` and `Suggest` (`internal/geocode/geocode.go`) go through a client with a five-second timeout.

`internal/geocode/suggest_test.go` checks that a repeated query reaches Google once, that the sixty-first uncached query in a minute is refused, and that cached queries still answer past the limit.
