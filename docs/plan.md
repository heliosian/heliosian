# Plan

What remains to build. Current behavior is documented in `docs/dev.md`, `docs/data.md`, `docs/directory.md`, `docs/design.md`, `docs/pwa.md`, and `docs/deploy.md`.

## Import tool

- One command that ingests a Veracross CSV export, rewrites the Veracross Import tab, re-runs the full pipeline, and reports the local layer's health beyond the fatal checks: useless overrides, `-` on flagged rows, name mappings or additions Veracross has made redundant, and media files matching no current person or family (including family blobs orphaned by a membership change). Replaces the manual writetab + loadcheck procedure in `docs/data.md`.
