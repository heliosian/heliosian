# Plan

What remains to build. Current behavior is documented in `docs/dev.md`, `docs/data.md`, `docs/directory.md`, `docs/design.md`, and `docs/pwa.md`.

## Directory app

- Remaining sections: My Family.
- Mobile chrome: brand-teal top bar and bottom tab navigation on narrow screens (see `docs/directory.md`); today only the desktop chrome is faithful.
- Installable-app plumbing: manifest, icons, and meta tags per `docs/pwa.md`.
- Self-service flows: photo and pronunciation upload, address update, opt-out.

## Hosting and deployment

- Cloud Run service in the school's project, minimum one instance (media is held in memory; startup is too slow for scale-to-zero).
- Docker build producing the static binary in a minimal base image containing only tzinfo and CA certificates.
