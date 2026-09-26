# Screenshots

`tools/screenshot` captures pages from the local dev server as PNGs, so humans and agents can verify visual changes. It drives a locally installed Chrome (or Chromium) headless via chromedp; no other browser tooling is required.

## Usage

For a sample-data page, one self-contained command serves, captures, and exits:

    go run ./tools/startserver --capture https://who.heliosiandev.com:8080/people --out local/screenshots/directory.png --wait .sidebar

It takes `--width` and `--height` too, so `--width 390 --height 844` is a phone; add `?mode=dark` to the URL to see the page in dark mode. It marks Quan mode as already shown, so the sample super admin is not switched onto it and the capture wears the mode the URL asks for. `--click` and `--settle` are the same as `tools/screenshot`'s below, and `--as <address>` views as that person through Spoof Mode, the way the sample super admin can from the toolbar, so a page can be captured as a plain member.

The capture serves on `PORT` (default 8080). When another dev server already holds that port, the capture can reach that server instead of its own, and shows whatever code it is running; give the capture a port nobody else is on, `PORT=8093 go run ./tools/startserver --capture …`.

With a server already running, capture against it directly:

    go run ./tools/screenshot --url https://who.heliosiandev.com:8080/people --out local/screenshots/directory.png --wait .sidebar

Flags:

- `--url` — page to capture (default `https://who.heliosiandev.com:8080/people`)
- `--out` — output PNG path (default `local/screenshots/capture.png`); `local/` is gitignored
- `--wait` — CSS selector that must be visible before capture (default `body`); pass a selector the page's JavaScript renders (for example `.card`) to capture after data loads
- `--width` / `--height` — the viewport, default 1280x800; `--width 390 --height 844` is a phone
- `--cookie` — cookies to set before navigating, `name=value`, several separated by semicolons: `--cookie heliosian-mode=dark` captures a page in dark mode, `heliosian-mode=quan` in Quan
- `--click` — CSS selector(s) to click once `--wait` is visible, several separated by `|` and each waited for before its click (for example `.editor-band button|.tab-strip-item:nth-child(4)|.image-find` opens an editor, switches tab, opens the image search); `--settle 3s` waits that long after the last click before capturing

The capture is a full-page screenshot at a 1280×800 viewport. "Full page" means the document's own scroll extent, so a page that sets `overflow: hidden` on `html` and scrolls an inner container yields only the viewport. Capture those with a tall `--size` viewport in `tools/browse` instead.

## Checking dark mode

A surface painted white or pale by name in an app's stylesheet, rather than from a palette variable (`docs/who/design.md`, Palette), stays so in dark mode, and the words on it - light, now - vanish. `tools/darkcheck` finds those without looking: it loads each page in dark mode, walks every visible element compositing its background up through its ancestors, and prints the words whose contrast falls short of what their size needs, any tab strip painted a different colour from what it sits on, and with `--light` every light opaque surface for reading by eye. `--compare` reads each page by day too and marks each finding `[dark only]` - dark mode's to fix - or `[by day too]`, a design choice to weigh on its own.

    go run ./tools/darkcheck --compare --url https://who.heliosiandev.com:8080/people,https://team.heliosiandev.com:8080/
    go run ./tools/darkcheck --compare --url https://home.heliosiandev.com:8080/ --click ".super-toggle|.tile .link-edit:not(.app-move)"

`--click` walks into a window first, as `tools/screenshot` does, and `--width 390 --height 844` reads the phone layout. It reports on the rendered page, so a hover state or a menu it has not opened is not read.

## Reading the console

A page that renders blank usually threw before it drew anything. `tools/console` loads a URL in headless Chrome and prints every console message and uncaught exception, with the file, line and column of a syntax error:

    go run ./tools/console --url https://celebrate.heliosiandev.com:8080/

It listens for four seconds after the load (`--wait` changes that) and prints `(nothing logged)` when the page was quiet. `--click ".host-actions .button|.tab-strip-item:nth-of-type(2)"` clicks its way to something first, so an error behind a button shows too, at the desktop viewport `tools/screenshot` uses; `--out x.png` saves a plain viewport screenshot of the page as it then stands - a full-page capture repaints some pages, so an in-place editor `tools/screenshot` shows closed can be seen open here.

## Capturing authenticated external sites

Some source material sits behind a login. The capture browser handles this:

    go run ./tools/capturebrowser

launches a headed Chrome with a dedicated profile in `local/capture-profile` and DevTools on `localhost:9222`. Log in to the target site in that window; the session persists in the profile across restarts. The profile holds that site's cookies, so it sits under `local/`, which is gitignored and never enters an image.

With the capture browser running, add `--remote` to attach to it instead of launching headless Chrome:

    go run ./tools/screenshot --remote --url https://example.com/some/page --out local/screenshots/existing/page.png --wait body

Each capture opens a fresh tab in the authenticated session, navigates, waits for the `--wait` selector, screenshots, and closes the tab. That suits a site with real URLs per page. A single-page app is `tools/browse` territory instead: its screens are reached by clicking rather than by URL, its images load lazily into a tab that has to persist, and its pages need a viewport tall enough to hold them.

## Interactive exploration

`tools/browse` drives the capture browser one step at a time: each invocation attaches to the current tab, performs at most one action, then captures and reports the resulting URL and title. The tab survives between invocations, so state (login, SPA position) carries across steps.

    go run ./tools/browse --nav https://example.com/ --out local/screenshots/step1.png
    go run ./tools/browse --clicksel "a.next" --wait "h1" --out local/screenshots/step2.png
    go run ./tools/browse --click 640,300 --out local/screenshots/step3.png
    go run ./tools/browse --dump

Actions (at most one step's worth per invocation):

- `--nav <url>` — navigate the tab
- `--back` — history back
- `--clicksel <selector>` — click the first match; times out if the selector never appears
- `--click <x,y>` — click at viewport coordinates, which map 1:1 onto the screenshot
- `--type <text>` — insert text into the focused element
- `--key <name>` — press enter, tab, escape, backspace, or a literal character
- `--scroll <px>` — scroll vertically, negative for up; scrolls the element with the most room to scroll rather than the window, so it works on apps that scroll an inner container. Reports `scroll: <top> of <height>, viewport <h>` so a walk down a long page knows when it has hit the bottom
- `--wait <selector>` — block until this selector is visible before capturing
- `--dump` — print the page HTML (for finding selectors) instead of writing a PNG
- `--eval <js>` — evaluate JavaScript in the page and print the JSON result instead of writing a PNG
- `--mobile` — emulate a phone viewport (390×844, touch) instead of the desktop 1280×800; click coordinates still map 1:1 onto the screenshot
- `--size <WxH>` — an explicit viewport, overriding the desktop default

The capture is the viewport, so click coordinates read off a screenshot are directly usable at whatever `--size` produced it. After an action that triggers cross-page navigation, always pass `--wait` with a selector expected on the destination page — the built-in settle delay is short, and without `--wait` the capture can race the navigation and show the previous page. The reported URL/title always reflect the final state; when a capture looks stale, re-run with no action to capture the current state.

Invocations must not linger: every run exits by itself within its 15-second internal timeout, leaving the browser and tab untouched. A full page load can exceed that: `--nav` to a cold URL often times out even though the tab does navigate, so follow it with an actionless invocation to capture the settled page. SPA navigation by click stays well inside the limit.

Leave the browser running between capture sessions — never kill it. In practice logins do not reliably survive a browser restart, and killing it forces a human to sign in again.

### Whole screens and long pages

The way to get a whole screen in one image is a tall `--size`:

    go run ./tools/browse --size 1280x3400 --out local/screenshots/page.png

Measure first, because guessing wastes a capture. The tallest scrollable element gives the content height:

    go run ./tools/browse --size 1280x800 --eval 'Math.max(...[...document.querySelectorAll("*")].map(el => el.scrollHeight))'

A result equal to the viewport height means the content fits; anything larger is the height to capture at. Measure at the default 1280×800 — lazily rendered collections grow as the viewport does, so a measurement taken at a tall viewport reads back only that viewport.

Past roughly 5000px a single image stops being readable. Capture those in parts at a fixed viewport, stepping with `--scroll` and watching the reported position for the point where it stops advancing:

    go run ./tools/browse --size 1280x4000 --scroll -40000 --out local/screenshots/page-1.png
    go run ./tools/browse --size 1280x4000 --scroll 4000 --out local/screenshots/page-2.png

`--eval` covers what the action flags do not. Horizontally scrolling boards and wide tables clip at a fixed width that a wider viewport does not change, so drive them by setting `scrollLeft` and capturing on the next invocation. Paginated collections are the same story — clicking the page buttons by coordinate is brittle because the control moves with the content, so click them by content:

    go run ./tools/browse --eval '[...document.querySelectorAll("button")].filter(b => b.textContent.trim() === "2")[0].click()'

## Agent recipe

    go run ./tools/startserver --capture https://who.heliosiandev.com:8080/people --out local/screenshots/directory.png --wait .sidebar

serves the sample community in-process, captures, and shuts down by itself — no background server to start or kill. Then read `local/screenshots/directory.png` to inspect the result.
