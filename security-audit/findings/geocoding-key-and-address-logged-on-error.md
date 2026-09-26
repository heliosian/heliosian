Description: The geocoder puts the server's Maps key in the request address (`geocode.Lookup`) and the directory logs the transport error whole, so a network failure writes the unrestricted server key and a family's street address into Cloud Logging for anyone who reads the project's logs.
Status: fixed
Severity: low
---
`Lookup` in `internal/geocode/geocode.go` called the legacy Geocoding API with the address and the key in the query string. On a transport failure Go returns a `*url.Error` whose text carries the whole URL, and the geocode worker in `internal/who/cache.go` logs it as `"error", err`. A non-OK status also came back as an error naming the address.

Fixed: `Lookup` calls the Geocoding API v4, `POST https://geocode.googleapis.com/v4/geocode/address` with `X-HTTP-Method-Override: GET`, the address as a form-encoded `addressQuery` in the body and the key in `X-Goog-Api-Key`, as `Suggest` sends it. The URL is a fixed string, so a transport error carries neither, and the errors for no results or an API error leave the address out.
