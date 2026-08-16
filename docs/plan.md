# Plan

What remains to build. Current behavior is documented in `docs/dev.md`, `docs/data.md`, `docs/directory.md`, `docs/design.md`, and `docs/pwa.md`.

## Directory app

- Remaining sections: My Family, Map, Email List.
- Mobile chrome: brand-teal top bar and bottom tab navigation on narrow screens (see `docs/directory.md`); today only the desktop chrome is faithful.
- Filter button behavior — currently visual only; the original filters by grade and classroom.
- Favorites: the heart toggle and bookmarks, feeding the email list's bookmarks tab.
- Installable-app plumbing: manifest, icons, and meta tags per `docs/pwa.md`.
- Self-service flows: photo and pronunciation upload, address update, opt-out.

## Hosting and deployment

- Cloud Run service in the school's project, minimum one instance (media is held in memory; startup is too slow for scale-to-zero).
- GitHub as source of truth with automatic build and deploy to Cloud Run on pushes to main.
- Docker build producing the static binary in a minimal base image containing only tzinfo and CA certificates.
- Serving domain and Cloud project layout are undecided.

## Data

- Sample data mode: generated, non-production data so contributors can develop and test with no credentials and no real community data.
- Clean sheet layout: one spreadsheet per app, one tab per entity type, first row as schema — replacing the inherited spreadsheet the loader currently reads.
- Year rollover: flipping the directory to the next school year using the next-grade mapping.
- Veracross integration as a second data-source implementation replacing Sheets when an API becomes available.

## Later

- Subsequent apps hosted in the same binary.
- TypeScript adoption if client code grows.
