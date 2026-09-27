# Agents

How code is written in this repository, for coding agents and the people driving them. What the project is and how it is laid out is in `README.md` and `docs/dev.md`.

## Code

- No comments in new code. The one exception is genuinely subtle logic a reader would misread, two lines at most. Design rationale goes in the commit message, and what a package is goes in `docs/`, not a package comment.
- Flags are liabilities. Hardcode settled values; prefer always-on behavior to switches, and add no opt-out for behavior that is always right.
- No fallback paths. Assume required resources exist and fail loudly; fatal is fine. One path through the code, not two.
- Converting from A to B means switching to B and deleting A: no interface with two implementations, no adapter, no translation layer keeping both alive.
- Production code never exists only for tests. No test wrappers, no interfaces or indirection added just so a test can swap in a fake, no test hooks, seams, exported-for-test helpers, or test-only parameters and branches. Tests exercise the real code as it ships; anything only a test needs lives in a `_test.go` file.
- A client request that takes 750ms is far too slow. Move slow work off the request path (queue it, write it back later) rather than making the person wait.
- Prefer an early return to a nested if/else when the if branch exits.
- Always use braces on the body of `if`, `else`, `for`, even for a single statement.
- Only reformat lines you touch, gofmt aside.
- When something fails, add debugging output to find out why before changing code. Don't guess.
- Test a change before committing it.

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
