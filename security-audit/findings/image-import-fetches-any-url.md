Description: The image import (`imagesearch.ServeImport`) fetched whatever URL the request named, redirects followed, so any signed-in account on the portal, Celebrate or When could make the server request internal or arbitrary addresses and read back what answered as an image.
Status: fixed
Severity: medium
---
`ServeImport` (`internal/imagesearch/imagesearch.go`) no longer takes an address. Its body is an id, which must match `idPattern` (sixty-four hex digits), and anything else is answered 404 before any fetch. The address fetched is the one the server itself wrote into `stock/<id>.json` in the media bucket when a search returned that hit, so the only hosts the import reaches are the ones the three providers' APIs named. A body carrying a `url` field, or a provider's own id, is a 404 like any other unknown picture (`TestStockFetchedOnce`).

The routes on HCA-Team, Celebrate and When still ask for no role beyond sign-in, as the search routes beside them do; what a member can do with them now is spend the providers' search quota and add a stock picture to the bucket, not reach an address of their choosing.
