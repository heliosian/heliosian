Description: Every image upload and import decodes the whole picture with no check of its declared dimensions, in `blob.Thumbnail` and `sharecard`, so any signed-in account can kill the single instance with one small crafted file.
Status: open
Severity: high
---
`blob.Thumbnail` (`internal/blob/blob.go:631`) calls `image.Decode` on the bytes as they arrive; `Store.Put` and `PutNamed` reach it through `writeWithThumbnail` (`blob.go:552-589`). The callers check only the first bytes' sniffed type and a byte cap (8 MB, 30 MB in Who?): `team.uploadImage` (`internal/team/team.go:1436`), Celebrate's `uploadImage` (`internal/celebrate/handlers.go:1671`), `imagesearch.ServeImport` (`internal/imagesearch/imagesearch.go:251`) on the portal, Celebrate, When and Heliosian, and Who?'s `upload` and `cropPhoto` (`internal/who/upload.go:468`, `:678`). None of the portal's, Celebrate's or When's routes asks for a role. `internal/sharecard/card.go:197` decodes a stored picture again on the public `/open/share/` routes.

A PNG of about 2 MB that declares 50000x50000 grey pixels, or a JPEG of a few hundred bytes whose frame header declares 65535x65535, makes Go allocate the whole frame - gigabytes - before it reads a pixel. The runtime dies of out-of-memory, which no `recover` catches; the instance restarts and whatever sat on the write queue is lost. `POST /api/team/image` with such a file is the whole of it.

Fix: `image.DecodeConfig` first in `Thumbnail` and in the share card's decode, refusing anything over a pixel budget (about 40 megapixels). One check covers every app.
