Description: Fetching a media object resets the idle clock that decides when it is dropped (`Store.serve` calls `touch`), so any signed-in account that knows a photo's address keeps it fetchable - on every app's host - after its owner deleted it, opted out, or the sheet stopped naming it, for as long as they ask every quarter hour.
Status: open
Severity: low
---
`serve` (`internal/blob/blob.go:604-607`) finds the object with `s.touch(key)`, which sets `e.used` to now (`:292-300`). `sweep` (`:272-285`) drops only what has gone unasked-for longer than `maxIdle`; its comment says an object a sheet stopped naming is one nobody asks for, which holds for the loaders and not for a browser. The object then lives until the next deploy. Names are content hashes served `immutable` for a year, so anyone who once saw the photo has the address, and `/photos/{name}` answers on all eight hosts, seven of them outside the member gate (`member-gate-only-on-who.md`).

Fix: have `serve` read the entry without refreshing `used`, leaving the loaders' `Has` and `Prefetch` as the only things that keep an object alive.
