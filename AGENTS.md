# Agents

How code is written in this repository, for coding agents and the people driving them. What the project is and how it is laid out is in `README.md` and `docs/dev.md`.

## Questions

- This codebase has a lot of features and complex data, and the person answering a question won't have the code in front of them. Every question stands alone: someone reading only that question, not the rest of the message, can answer it. It starts with the background it needs - what part of the system is involved and how it works today - repeated inside the question rather than pointed at from a section above. Never refer to an earlier question, decision, option or message by a label ("A", "C", "option 1", "decision 2", "what we agreed", "the plan above"); state what it said, in full, every time it comes up.
- Explain every identifier where it appears: a table, column, header, enum value or file gets a plain sentence on what it is and does today. A mapping written in names alone ("List-Id → sent_to group") is not a question.
- Fetch real examples - the rows, records, pages or log lines in question - and put them inside the question they belong to, rather than describing data in the abstract.
- Be very clear about the broader context the change sits in: what the larger task is, why this question came up, and what depends on the answer. Each option says what changes for people: who can see what, who receives what, what someone reading the data would notice.
- Propose a path. Say which option you recommend and why, so the question can be answered with a yes or a correction.

## Answers

- Asked for a query, answer with a link to Helios Admin's Query page, `https://admin.heliosian.com/query?q=` and the query URL-encoded (`docs/admin/admin.md`, Query), and the query itself in a code block. Run it with `go run ./tools/q read` first to check that it parses and answers, but the answer is the link, not the tool's output.

## Code

- No comments in new code. The one exception is genuinely subtle logic a reader would misread, two lines at most. Design rationale goes in the commit message, and what a package is goes in `docs/`, not a package comment.
- Flags are liabilities. Hardcode settled values; prefer always-on behavior to switches, and add no opt-out for behavior that is always right.
- No fallback paths. Assume required resources exist and fail loudly; fatal is fine. One path through the code, not two.
- Converting from A to B means switching to B and deleting A: no interface with two implementations, no adapter, no translation layer keeping both alive.
- Production code never exists only for tests. No test wrappers, no interfaces or indirection added just so a test can swap in a fake, no test hooks, seams, exported-for-test helpers, or test-only parameters and branches. Tests exercise the real code as it ships. Anything only tests need lives in a `_test.go` file, or, when several packages' tests share it, in `internal/testkit`, which only `_test.go` files import.
- A client request that takes 750ms is far too slow. Move slow work off the request path (queue it, write it back later) rather than making the person wait.
- Nothing waits for the five-minute refresh or any other timer. The refresh exists only to pick up people's edits to the sheets; work that follows from a change runs when the change commits (a swap hook, `Tx.After`), never on a ticker or after a grace period.
- Prefer an early return to a nested if/else when the if branch exits.
- Always use braces on the body of `if`, `else`, `for`, even for a single statement.
- Only reformat lines you touch, gofmt aside.
- When something fails, add debugging output to find out why before changing code. Don't guess.
- Test a change before committing it.
- Tests and page captures run under `timeout 10`, every time: one package, or a few named tests, or one capture per run, so it fits.
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

- Docs describe how things are today, and cover what is hard to derive from the code. No history unless it matters to the present, no dates, and no changelog framing ("added", "changed", "corrected"); the change story lives in git history. Plans belong in GitHub issues, not docs.
- Point at where a value lives and how to read it - the command that prints it, the console page that holds it - never the value itself, which is stale the day it is copied.

## Git

- Commit messages are one short line summarizing the change, not every detail of it crammed into a single line. The diff carries the detail.
- Work that belongs to a GitHub issue references it in the commit message (`Fix login redirect (#123)`).
- Several agents work in this tree at once. A file another session has dirty is still open to you: edit it whenever your task needs it, alongside their changes, and leave their edits as they are.
