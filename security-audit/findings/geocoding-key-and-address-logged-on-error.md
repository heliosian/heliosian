Description: The geocoder puts the server's Maps key in the request address (`geocode.Lookup`) and the directory logs the transport error whole, so a network failure writes the unrestricted server key and a family's street address into Cloud Logging for anyone who reads the project's logs.
Status: open
Severity: low
---
`internal/geocode/geocode.go:26-27` calls `http.Get` on `...geocode/json?address=<address>&key=<key>`. On a transport failure Go returns a `*url.Error` whose text is the method, the whole URL and the cause, and `internal/who/cache.go:574-577` logs it as `"error", err`. `docs/dev.md` says the server key is never rendered into pages and may be unrestricted; a log line is the one place it leaves the process.

Fix: in `Lookup`, unwrap the `*url.Error` and return its `Err` with the address left out; `Suggest` already keeps the key in a header.
