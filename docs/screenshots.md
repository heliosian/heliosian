# Screenshots

`tools/screenshot` captures pages from the local dev server as PNGs, so humans and agents can verify visual changes. It drives a locally installed Chrome (or Chromium) headless via chromedp; no other browser tooling is required.

## Usage

With the server running:

    go run ./tools/screenshot -url http://localhost:8080/directory/ -out screenshots/directory.png -wait header

Flags:

- `-url` — page to capture (default `http://localhost:8080/directory/`)
- `-out` — output PNG path (default `screenshots/capture.png`); `screenshots/` is gitignored
- `-wait` — CSS selector that must be visible before capture (default `body`); pass a selector the page's JavaScript renders (for example `.card`) to capture after data loads

The capture is a full-page screenshot at a 1280×800 viewport.

## Capturing authenticated external sites

Some source material (like the production apps being ported) sits behind a login. The capture browser handles this:

    go run ./tools/capturebrowser

launches a headed Chrome with a dedicated profile in `~/.heliosian/capture-profile` and DevTools on `localhost:9222`. Log in to the target site in that window; the session persists in the profile across restarts. The browser stays out of the repo entirely — no cookies or credentials ever land here.

With the capture browser running, add `-remote` to attach to it instead of launching headless Chrome:

    go run ./tools/screenshot -remote -url https://example.com/some/page -out screenshots/existing/page.png -wait body

Each capture opens a fresh tab in the authenticated session, navigates, waits for the `-wait` selector, screenshots, and closes the tab. Exploring a site is a series of `-remote` captures over its URLs.

## Interactive exploration

`tools/browse` drives the capture browser one step at a time: each invocation attaches to the current tab, performs at most one action, then captures and reports the resulting URL and title. The tab survives between invocations, so state (login, SPA position) carries across steps.

    go run ./tools/browse -nav https://example.com/ -out screenshots/step1.png
    go run ./tools/browse -clicksel "a.next" -wait "h1" -out screenshots/step2.png
    go run ./tools/browse -click 640,300 -out screenshots/step3.png
    go run ./tools/browse -dump

Actions (at most one step's worth per invocation):

- `-nav <url>` — navigate the tab
- `-back` — history back
- `-clicksel <selector>` — click the first match; times out if the selector never appears
- `-click <x,y>` — click at viewport coordinates, which map 1:1 onto the screenshot (1280×800 viewport)
- `-type <text>` — insert text into the focused element
- `-key <name>` — press enter, tab, escape, backspace, or a literal character
- `-scroll <px>` — scroll vertically, negative for up
- `-wait <selector>` — block until this selector is visible before capturing
- `-dump` — print the page HTML (for finding selectors) instead of writing a PNG
- `-mobile` — emulate a phone viewport (390×844, touch) instead of the desktop 1280×800; click coordinates still map 1:1 onto the screenshot

The capture is the visible viewport, not the full page, so click coordinates read off a screenshot are directly usable. After an action that triggers cross-page navigation, always pass `-wait` with a selector expected on the destination page — the built-in settle delay is short, and without `-wait` the capture can race the navigation and show the previous page. The reported URL/title always reflect the final state; when a capture looks stale, re-run with no action to capture the current state.

Invocations must not linger: every run exits by itself within its 15-second internal timeout, leaving the browser and tab untouched.

At the end of a capture session, quit the browser:

    pkill -f capture-profile

The login session persists in the profile, so the next `tools/capturebrowser` launch is still signed in.

## Agent recipe

One self-contained command that starts the server, captures, and shuts down:

    go run . &
    go run ./tools/screenshot -out screenshots/directory.png -wait header
    kill $(lsof -ti :8080)

Then read `screenshots/directory.png` to inspect the result.
