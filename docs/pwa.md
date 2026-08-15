# Installable web app

The directory is used from phone home screens, so Heliosian ships as an installable web app (PWA), matching what the existing app does. These notes record how the existing app achieves it and what our server needs to serve.

## How the existing app is wired

- **Manifest** via `<link rel="manifest">`: `name`/`short_name` ("Helios Who?"), `description`, `display: "standalone"`, `start_url` on the app's own domain, `theme_color` and `background_color` both brand teal `#014E54`, and icons — 16/32 favicons plus 192 and 512 PNGs, each in both `purpose: any` and `purpose: maskable` variants.
- **Head meta**: `viewport` with `viewport-fit=cover` (edge-to-edge under notches) and `user-scalable=no`; `apple-mobile-web-app-capable: yes`; `apple-mobile-web-app-status-bar-style: black-translucent`; a page-level `theme-color` set to the light surface color (`#F6F6F6`) — the manifest's teal governs install/launch chrome while the meta tracks in-app surface.
- **iOS extras**: an `apple-touch-icon`, and a large battery of `apple-touch-startup-image` links with device-specific media queries — pre-rendered splash screens (the logo lockup on teal) for every iPhone/iPad size, because iOS ignores the manifest for splash.
- **Service worker**: the shell assumes one may control the page (its boot script checks `navigator.serviceWorker.controller` to drive reload and offline-retry behavior), giving offline shell support and Android install quality.

## What Heliosian serves

- `manifest.webmanifest` from the binary: name, short name, `display: standalone`, `start_url: /`, theme and background color `#014E54`, icons 192 and 512 as `any` plus maskable variants (maskable art keeps the lockup inside the safe zone on a full-bleed teal square).
- Base template meta: `theme-color`, `viewport` including `viewport-fit=cover`, the two `apple-mobile-web-app-*` tags, and a 180px `apple-touch-icon`.
- HTTPS comes with Cloud Run; installability requires it.
- Splash screens for iOS are pre-rendered at the device-size matrix like the original; until that exists, launches show a plain background, which is acceptable.
- A service worker is optional for install on current Chromium and adds offline shell caching; if added, it stays minimal — cache the static shell, never cache directory data (community data must not persist on shared devices beyond the session's needs).
- `start_url` must resolve for a signed-out user by landing on the sign-in flow, then into the app.

Icon and splash source files are derived from the brand assets (see `docs/design.md`).
