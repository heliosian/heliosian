# Code audit

What a code-quality audit of this codebase looks for, what it leaves alone, and how its findings are kept. Security has its own audit, `docs/audits/security.md`.

## The system

The layout is in `README.md` (Layout) and `docs/dev.md`. What matters to this audit is that the apps are built to one shape, in `internal/<app>` and `web/<app>`. So the main question is where the parallel pieces share code and where they are copies that have drifted apart. Most findings are one of those two.

The lists below say where to look, not what is wrong. None of their lines is a claim about the code.

## In scope

### Duplication across apps

- **Parallel files.** Compare the same file across every app (every `cache.go`, `share.go`, `mail.go`, `load.go`, and on the client every `dom.js`, `chrome.js`, `state.js`, `app.js`, `edit.js`, `pages/admin.js`, `style.css`, `index.html` and `login.html`). Note what is byte-identical, what is identical apart from names or wording, and what one app has that the rest lack.
- **Small helpers each package writes for itself.** Request-body decoding, JSON responses, the "who is asking" lookup, admin checks, the save helper, the site's base URL, the school time zone, ID generators, email cleaning, yes/no cells, URL and date checks, display names.
- **Whole features written once per app.** The admin list and its handlers, Settings-tab parsing, share cards, mail letters and their templates, `.ics` files, image upload and image search, the people picker, tabs, modals and forms, the Admin Tools shell, Super Admin Mode state, the router.
- **Shared helpers that exist and go unused.** `sharecard.Serve` and `ETag`, `mail.ICSEscape`, `config.NormalizeEmails`, `web/common/tabs.js`, `web/common/picker.js`, `signedIn` in `toolbar.js`. A package that re-implements one of these is a finding even when its copy is correct.
- **Helpers copied into `tools/` and `cmd/`** from `internal/app`: required environment variables and keys, bundled-file stubs, sheet lists, column schemas.
- **Test helpers copied between `_test.go` files.** Sample servers, recorders, image-checker stubs, time helpers.

### Near-duplicates that have drifted

- **Copies that disagree.** Two copies of one helper that now behave differently, such as a size limit, how a blank cell is read, whether spaces are trimmed, whether an address needs an `@`, a default duration, a timeout, or a status code. Each difference is either a parameter that should be explicit or a bug in one copy. The finding says which.
- **Values that vary by app when nothing chose them.** z-indexes, drawer widths, hover shades, toast durations, which log fields get written, which fields a response returns.
- **Rules re-derived in a second place.** Ask's tools, the share cards, the toolbar badges and the calendar's linked events each restate what the owning app decides: visibility, "full", date parsing, bands. Put the two side by side against the same input and check they give the same answer.
- **The same fact checked two ways.** Admin rights checked with alias resolution in one app and against the raw sign-in address in another. Permission rules decided separately in each copy instead of once.

### Layering

- **Too many layers.**
  - Wrappers that only forward (`X` → `XUnder`, `Build` → `build`).
  - Interfaces with one implementation.
  - Adapters that translate one model into a per-app copy of itself.
  - A type alias with no purpose.
  - Two routes to the same data, such as some apps going through a `Directory` interface while others read `who.Model` directly.
- **Too few layers.**
  - Business rules inside HTTP handlers, where only a request can test them.
  - Sheet-row manipulation scattered through callers.
  - Domain logic sitting in the wiring package, so a tool that needs one function has to import the whole server.
  - A client with no API layer, where each view calls `fetch` and handles errors its own way.
- **Wiring.**
  - Whether there is one list of apps, or the same list is written out in several places that must be kept in step.
  - Package-level variables set from outside.
  - `Register` functions with long lists of positional parameters.
- **Circular imports on the client,** and the dynamic imports and custom events used to work around them.

### Complexity

- **Files that hold several jobs,** each of which could be its own file: model, permissions, view, handlers, mail, image work.
- **Functions hundreds of lines long.** Name the pieces they already fall into.
- **The same sequence written out several times inside one file.** A lookup-or-404, a retry loop, a fan-out with error collection, a recipient loop.
- **Work redone per item** that could be done once per request: sorting all events per link, rebuilding a roster per tag.

### Second paths, fallbacks and switches

- **Dependencies production always provides but the code treats as optional.** A constructor that returns nil, followed by nil checks at every call site, often with different behaviour at each.
- **Fallbacks that hide failure.** An empty list on a failed load, a default when a value is missing, a `.catch(() => {})`, a `try`/`catch` around `localStorage`.
- **Settings nothing ever changes.** Environment variables the deploy never sets, flags always given the same value, policies with only one caller, sentinel defaults like `0` meaning "use the real default".
- **Code kept alive for retired data.** Old column names, skipped legacy rows, cleanup for sessions that have long expired. The fix is to migrate the data and delete the code.
- **An old implementation still wired in beside its replacement.**

### Operator-facing consistency

- **Logging:** `[ERROR]` on error-level lines, lowercase messages, log prefixes naming the app and not a sheet, one logging package throughout the server.
- **Mail senders, keys and configuration,** each passed in one shape, not several.
- **Tools that do the same job with different flag names or different dry-run behaviour.**
- **Tool output that doesn't match its name** or its docs (a `.png` that is a JPEG).

### Style (`AGENTS.md`)

- Comments and package docs.
- `make()` where a literal works.
- if/else where the if branch exits.
- Bodies without braces.
- Long flags written with one dash, in code, messages or docs.
- gofmt.

Each rule is one sweep. Findings name the files and give a count per file; they do not list every instance.

### Beyond structure

These areas are close enough to structure to belong to this audit. Each is its own pass with its own evidence:

- **Request-path latency.** Handlers that call a sheet, a bucket, Claude or mail before answering. A request the person waits 750ms on is a finding (`AGENTS.md`), and the fix moves the work off the request path.
- **Startup and memory.** What every store loads at boot, what is held twice (in the model and in a cache), and what is rebuilt on every refresh when only one tab changed.
- **Unused surface.** Routes no page or tool calls, response fields no page reads, exported functions with no caller outside tests, CSS selectors no page renders, icons no page draws, and anything in `go.mod` the code no longer uses.
- **Tests that line up with the rules.**
  - Rules that can only be tested through HTTP.
  - Each app's visibility function and whether its tests cover the kinds of member (student, parent, staff, admin).
  - Behaviour-level duplicates, where tests written separately in each app should run once against a shared implementation.
- **Docs against code.**
  - Docs that describe behaviour the code no longer has.
  - Docs that copy a value instead of pointing at where it lives.
  - Tools, routes and settings the docs never mention.
- **Schema agreement.** A tab's columns as `tools/createtabs` writes them, as the owning package reads them, and as `sampledata/` holds them should all match.
- **Concurrency outside security.**
  - Package-level variables written from request goroutines.
  - Maps shared across goroutines.
  - Anything `go test -race` would flag.
- **Error handling.**
  - Errors ignored (`json.Encode`, `Close`, `Write`).
  - Errors wrapped without context.
  - The same failure answered with different status codes in different apps.
- **Client behaviour the apps should share.**
  - What happens when a session expires mid-page.
  - Focus and Escape in modals.
  - Keyboard use of pickers and tabs.
  - `aria` roles on the controls each app built for itself.
- **Theme.** `tools/darkcheck --compare` across every app, and surfaces painted by name instead of by variable.
- **Wording.** App names, button labels and empty-state text written differently in different apps for the same thing.

## Out of scope

- **Security.** Anything a person can do that they should not be able to goes to `docs/audits/security.md` and is filed as a Security issue. A cleanup that happens to close a security gap is filed under Security as well, with the cleanup named in its fix.
- **Look and feel.** Whether a page looks right is judged by eye. A visual difference between apps is a finding only when it is a drifted copy of the same component, not a design choice.
- **Deliberate differences.** Each app's colours, words, icons and which features it has. A difference is a finding when nothing chose it.
- **Rewrites and frameworks.** A fix is shared code, a deleted copy, or a moved function. It is never a new framework, build step, service or store, and never a layer that keeps both copies alive.
- **Counts for their own sake.** A number of copies supports a finding; it is not one. Every finding names the files and functions.
- **Claims not checked against the code.** Every finding is read against the current tree before it is filed. A count or a line quoted from memory, or from an earlier pass, is re-checked.
- **Work in flight.** Files another session has dirty in the working tree describe where that work is headed. A finding against them waits until they land, or says which state it describes.
- **The platforms underneath.** Go, the browser, Google's APIs, chromedp, Mailgun, Anthropic. How the code uses them is in scope.

## Findings

A finding is a GitHub issue on this repository. Structural findings take the issue type Cleanup. Wrong behaviour found along the way, including a user-visible drift between copies, takes the type Bug. Each issue is one search:

    gh issue list --repo heliosian/heliosian --search "type:Cleanup" --state all
    gh issue list --repo heliosian/heliosian --search "type:Bug" --state all

Search before filing, so the same thing is not filed twice. `gh api orgs/heliosian/issue-types` lists the types, and `gh label list --repo heliosian/heliosian` lists the labels. An issue carries the `app:<key>` label of every app whose code it touches. A finding confined to shared packages or `tools/` carries none. No labels are made up for an audit.

The issue carries:

- **Title.** What is wrong, as a plain sentence.
- **Opening.** One or two sentences saying what is duplicated, drifted or tangled, and where.
- **Where.** Files and functions, without line numbers, since those go stale.
  - Every instance when there are about ten or fewer; otherwise a representative dozen and the total.
  - Drifted values side by side: the limit in one file against the limit in the other, the two readings of a blank cell.
- **How to see it,** for a Bug. A path and steps a person can follow, or a test that reproduces it.
- **Why it matters.** What the drift has already broken, or what a fix to one copy will miss. Briefly.
- **Suggested fix.** The shared function or package, its signature where that makes it concrete, and the copies it replaces.

One finding per issue:

- The same duplication across several apps is one finding naming them all.
- A bug inside that duplication is its own Bug, linked from the Cleanup.
- Two unrelated tangles in one file are two findings.
- Findings that depend on each other link each other by number.

An issue stays open while the finding stands. It closes as completed once a change removes the duplication or fixes the bug. It closes as not planned when the finding turns out wrong, or the difference turns out deliberate, with the reason in a comment. A fix's commit message names the issue.
