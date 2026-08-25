# Plan

What remains to build. Current behavior is documented in `docs/dev.md`, `docs/data.md`, `docs/directory.md`, `docs/design.md`, `docs/pwa.md`, and `docs/deploy.md`.

## The photo switcher on the person page

A person's photos reach the client as a list, with one of them named as primary, but `web/static/directory/app.js` renders `photoUrl` alone in about fourteen places. That keeps working untouched, since `photoUrl` is the primary — so only the person page needs new UI: let any viewer flip through the list, and let whoever may edit that record choose which one the directory shows. The edit handler has no field for it yet; it needs one that accepts only a name already among that person's photos and writes the `Primary Photo` override.

This is visual work. Build it to a state that can be judged and hand it over rather than iterating on the look.

## Import health report

`tools/import` ingests the Veracross export, uploads its portraits, syncs the import tabs and reloads the model. What it does not do is report the local layer's health beyond the checks that are already fatal: `-` on flagged rows, and name mappings or additions Veracross has made redundant.

It should also report media the sheet no longer names. That question is newly answerable — the sheet became the index, so an object nothing points at is now visible rather than merely unreferenced — and it covers both the family blobs a membership change orphans and anything a retired layout left behind.
