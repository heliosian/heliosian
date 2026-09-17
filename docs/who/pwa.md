# Installable web app

The directory is used from phone home screens, so Heliosian ships as an installable web app (PWA). These notes record what the server serves for that.

## What Heliosian serves

- `manifest.webmanifest` at `/manifest.webmanifest`, a file in `web/public/who/`, the tree served without sign-in, because browsers fetch manifests without credentials. `scope` and `start_url` are `/`; name, `display: standalone`, theme and background color `#014E54`; icons 192 and 512 as `any` plus a 512 maskable (the maskable art keeps the lockup inside the safe zone on a full-bleed teal square). The server registers the `application/manifest+json` MIME type.
- Both pages (app and login) carry the manifest link, `theme-color` (light surface on the app page, teal on login - the manifest's teal governs install and launch chrome while the meta tracks the in-app surface), `viewport` including `viewport-fit=cover` (edge-to-edge under notches) and `user-scalable=no`, `apple-mobile-web-app-capable: yes`, `apple-mobile-web-app-status-bar-style: black-translucent`, 16/32 favicons, and the `apple-touch-icon`.
- HTTPS comes with Cloud Run; installability requires it.
- Splash screens for iOS, which ignores the manifest for splash: both templates carry a full `apple-touch-startup-image` battery — 32 pre-rendered PNGs (the logo lockup on teal) with device-specific media queries covering every iPhone/iPad class in both orientations, served from `/brand/splash/` out of `web/public/who/brand/splash/`.
- A service worker is optional for install on current Chromium and adds offline shell caching; if added, it stays minimal — cache the static shell, never cache directory data (community data must not persist on shared devices beyond the session's needs).
- `start_url` must resolve for a signed-out user by landing on the sign-in flow, then into the app.

Icon and splash source files are derived from the brand assets (see `docs/who/design.md`).
