Description: Fetching a media object reset the idle clock that decided when it was dropped (`Store.serve` called `touch`), so any signed-in account that knew a photo's address kept it fetchable - on every app's host - after its owner deleted it, opted out, or the sheet stopped naming it, for as long as they asked every quarter hour.
Status: fixed
Severity: low
---
The blob store in `internal/blob/blob.go` has no idle clock. It holds what the last refresh named: `Store.Load` starts a fresh set of names each refresh, `has` and `Prefetch` add to it as the loaders build, and the swap `drop` removes every object outside it. `serve` and `Bytes` look an object up with `held`, which records nothing, so a browser or a share card asking for an object cannot keep it. The refresh is the single one `store.Queue` runs over every sheet (`docs/storage.md`), so an object leaves memory at the first refresh after the last sheet naming it stops.

`TestOnlyWhatARefreshNamesIsKept` in `internal/blob/blob_test.go` serves an unnamed object during a refresh and checks it answers 404 once the refresh swaps in.
