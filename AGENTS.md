# Agents

How code is written in this repository, for coding agents and the people driving them. What the project is and how it is laid out is in `README.md` and `docs/dev.md`.

## Questions

- This codebase has a lot of features and complex data, and the person answering a question won't have the code in front of them. Every question stands alone: someone reading only that question, not the rest of the message, can answer it. It starts with the background it needs - what part of the system is involved and how it works today - repeated inside the question rather than pointed at from a section above. Never refer to an earlier question, decision, option or message by a label ("A", "C", "option 1", "decision 2", "what we agreed", "the plan above"); state what it said, in full, every time it comes up.
- Explain every identifier where it appears: a table, column, header, enum value or file gets a plain sentence on what it is and does today. A mapping written in names alone ("List-Id → sent_to group") is not a question.
- Fetch real examples - the rows, records, pages or log lines in question - and put them inside the question they belong to, rather than describing data in the abstract.
- Every option comes with an example. Each option, alternative, proposal or explained behavior is shown worked through on that real input: exactly what it produces from it. Options are compared on the same input side by side - one input, then each option's output - never one worked example followed by prose on how the others would differ. An option described only in prose is not an option; fetch the data and write its example, or drop it.
- A phrase standing for a mechanism ("its text", "the cluster", "a copy") is defined concretely where it is used: which column, which rows, which comparison.
- Be very clear about the broader context the change sits in: what the larger task is, why this question came up, and what depends on the answer. Each option says what changes for people: who can see what, who receives what, what someone reading the data would notice.
- Propose a path. Say which option you recommend and why, so the question can be answered with a yes or a correction.

## Answers

- Asked for a query, answer with a link to Helios Admin's Query page, `https://admin.heliosian.com/query?q=` and the query URL-encoded (`docs/admin/admin.md`, Query), and the query itself in a code block. Run it with `go run ./tools/q read` first to check that it parses and answers, but the answer is the link, not the tool's output.
- A command written for the person to run assumes their shell already holds `tools/devenv`'s variables: write it without `eval "$(go run ./tools/devenv)"`, and don't mention devenv unless they ask or an error it would explain comes up.

## Code

- No comments in new code. The one exception is genuinely subtle logic a reader would misread, two lines at most. Design rationale goes in the commit message, and what a package is goes in `docs/`, not a package comment.
- Flags are liabilities. Hardcode settled values; prefer always-on behavior to switches, and add no opt-out for behavior that is always right.
- No fallback paths. Assume required resources exist and fail loudly; fatal is fine. One path through the code, not two.
- Converting from A to B means switching to B and deleting A: no interface with two implementations, no adapter, no translation layer keeping both alive.
- Production code never exists only for tests. No test wrappers, no interfaces or indirection added just so a test can swap in a fake, no test hooks, seams, exported-for-test helpers, or test-only parameters and branches. Tests exercise the real code as it ships. Anything only tests need lives in a `_test.go` file, or, when several packages' tests share it, in `internal/testkit`, which only `_test.go` files import.
- A client request that takes 750ms is far too slow. Move slow work off the request path (queue it, write it back later) rather than making the person wait.
- Nothing waits for the periodic refresh or any other timer. The refresh exists only to pick up people's edits to the sheets; work that follows from a change runs when the change commits (a committed trigger, a swap hook, `Tx.After`), never on a ticker or after a grace period. The one exception is the store's tick (`Store.OnTick`), and only for what no commit announces: retrying a send that failed, work that falls due with the date, and search's full pass, which catches what its incremental pass misses.
- Share cards and link previews (`/open/share/`, Open Graph tags) are public and drawn for every event, party and activity by ID, invite-only and pending included, with no visibility check: sharing depends on it. Settled; never ask about it or offer to narrow it (`docs/dev.md`).
- The data model's endpoints, files and system readers are named for what they do with the data model (`draft-members`, `listmail.go`, `mailer`), never for the app that uses them.
- Prefer an early return to a nested if/else when the if branch exits.
- Always use braces on the body of `if`, `else`, `for`, even for a single statement.
- Only reformat lines you touch, gofmt aside.
- When something fails, add debugging output to find out why before changing code. Don't guess.
- Test a change before committing it.
- Tests run under `timeout 30`, every time: one package, or a few named tests, so it fits. Page captures run under `timeout 20`, one capture per run.
- One-off tools - a data migration, a single investigation - go in `local/<name>/`, which git ignores, and run as `go run ./local/<name>`. `tools/` is for tools that stay.

## Go

- `go run`, never `go build`, so no binaries land in the tree. Use `go vet` to check compilation.
- Literal initialization (`map[string]bool{}`, `[]string{}`) over `make()`. `make` only for a slice that needs its length up front: slots written by index or a byte buffer, never for a capacity hint.
- `any`, never `interface{}`.
- All-lowercase log messages, through `slog` at the level that fits; severity is the level, never a prefix in the message.
- Keep every Go file gofmt-clean.
- The server is pure Go: no cgo, no external binaries or subprocesses, and no Debian (or other distro) base image. Don't propose any of them, not even as an option.
- Long flags take double dashes (`--dry-run`), everywhere: flags a tool defines, commands run, and commands written in docs. Single dashes are for single-letter flags.

## Docs

- Docs describe how things are today, and cover what is hard to derive from the code. No history unless it matters to the present, no dates, and no changelog framing ("added", "changed", "corrected"); the change story lives in git history. Plans and follow-up work belong in TODO.md, not docs. Agents file GitHub issues only when Ian asks for one.
- Point at where a value lives and how to read it - the command that prints it, the console page that holds it - never the value itself, which is stale the day it is copied.

## Verifying

- Production is Cloud Run service `heliosian`, built by Cloud Build, in project `heliosian`, region `us-west1` (`docs/deploy.md`). The live commit is the one the serving revision's image was built from: the revision taking traffic from `gcloud run services describe heliosian --region us-west1 --project heliosian`, its image digest from `gcloud run revisions describe <revision> --region us-west1 --project heliosian`, and the build in `gcloud builds list --region us-west1 --project heliosian` whose image has that digest, its commit in `substitutions.COMMIT_SHA`.
- Verify the work the live commit carries first. Then wait for every build still queued or running (`gcloud builds list --region us-west1 --project heliosian --filter "status=QUEUED OR status=WORKING"`; waiting on one is in `docs/deploy.md`, Verifying a deploy) to finish, and verify the work those deploys carry against the new live commit.
- Check data with `go run ./tools/q read`.
- Preview imports with `go run ./tools/import --dry-run`; omitting `--dry-run` writes to production. The import runs both exporters itself. `VCEXPORT` and `WEBEXPORT` name their source checkouts, not their output directories. Inspect the proposed calendar batch before judging its edit count: a reading prompt change updates the `GROUP_SOURCE.hash` reading fingerprint on every PDF source even when the calendar data is identical.
- For an authorized production document catch-up, run `go run ./tools/fetchsignin` before `go run ./tools/pdfthumbnails`: the first fetches documents through the capture browser's Google session; the second renders missing PDF pages, including newly fetched PDFs, on the client and waits for the server to thumbnail them. Both write to production. Their setup and behavior are in `docs/dev.md`, Tools. PDF documents with the same `content` ID hold identical bytes; reuse their page images and server thumbnails by creating page children over both existing `CONTENT` references through ordinary write batches. `pdfthumbnails` leaves pacing to the server and waits out server restarts itself.
- Check pages with byre-hands `chrome` in byre's Chrome on iso, where ian is signed in to every heliosian site.
- A change only some viewers see (a room parent, a manager, a guest) is checked by viewing as one of them: Spoof Mode in the top bar's eye menu on pages, `go run ./tools/q --spoof <address> read` for data. Find someone who fits with `go run ./tools/q read`, and stop spoofing when done. "The signed-in person can't see it" is never a reason to leave a check unverified.

## Git

- Commit messages are one short line summarizing the change, not every detail of it crammed into a single line. The diff carries the detail.
- Commit/merge titles for work that fixes a GitHub issue use a closing keyword (`Fix login redirect (fixes #123)`). A bare `(#123)` is only for work related to an issue that doesn't fix it.
- Several agents work in this tree at once. A file another session has dirty is still open to you: edit it whenever your task needs it, alongside their changes, and leave their edits as they are.
