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

## Agent recipe

One self-contained command that starts the server, captures, and shuts down:

    go run . &
    go run ./tools/screenshot -out screenshots/directory.png -wait header
    kill $(lsof -ti :8080)

Then read `screenshots/directory.png` to inspect the result.
