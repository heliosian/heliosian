---
name: security-findings
description: Record, update, search and list the security audit's findings under security-audit/findings/. Use when auditing this codebase for security issues, when writing up or changing the status of a finding, when asked to fix a finding or to pick the next one to work on, or when asked what findings exist, which are open, or whether something has already been found.
---

# Security findings

`security-audit/README.md` says what the audit looks for and what it leaves alone. Read it before writing a finding; something out of scope is not written up.

A finding is one markdown file in `security-audit/findings/`:

    Description: one sentence
    Status: open
    Severity: medium
    ---
    brief details

Line one is `Description: `, line two is `Status: `, line three is `Severity: `, line four is `---`, and the rest is the details. Dates come from git, so a file carries none.

## Create a finding

1. Search first (below), so the same thing is not written twice. If it is already there, edit that file.
2. Write `security-audit/findings/<name>.md` with the Write tool, starting from `security-audit/TEMPLATE.md`. The name is what the finding is about in a few lowercase words joined by hyphens: `open-feed-token-never-expires.md`.
3. Fill it in:
   - `Description:` one sentence, on one line, saying what is wrong, where, and who (anyone, a link holder, a member, an admin) can do what because of it.
   - `Status:` `open`.
   - `Severity:` `critical`, `high`, `medium` or `low`, as `security-audit/README.md`, Findings, defines them: who can do it and what they get. It says how bad the finding is while it stands, so it stays as the status changes.
   - Details, after the `---`: the files and lines, how to see it, and the fix if one is plain. A few short paragraphs at most.
4. Run `go run ./cmd/findings --text <a word from it>` to see that it parses.

One finding per file. The same mistake in five handlers is one finding naming the five; two different mistakes in one handler are two.

## Edit a finding

Edit the file with the Edit tool. Status is one of:

- `open` - true and not yet dealt with.
- `fixed` - a change closes it; the details name the change.
- `wontfix` - understood and left; the details say why.
- `invalid` - not a finding after all; the details say why.

A finding that stops being true has its status changed and its details brought up to date; the file is never deleted, so the record of what was looked at stays. The details describe the finding as it stands, not the story of how it changed - git has that.

## Pick a finding to work on

Asked to fix a finding, or the next one, without being told which, take the one the tool names rather than choosing:

    go run ./cmd/findings --first

It answers with one open finding - the first by file name at the highest severity any open finding has - and with the same one every time until that finding's status changes, so two sessions asked the same thing start on the same file and nobody weighs thirty findings against each other to begin. Read its details with the Read tool, fix what it describes, then set its status to `fixed` with the details naming the change; `--first` then moves on to the next. Narrow it with the filters when the work is kept to one area: `--first --text loop`. When the person names a finding, or a severity or an app to start from, that is the pick and `--first` is not asked.

## Search and list findings

    go run ./cmd/findings

prints each finding's status, severity, path, the days git says it was created and last changed (`uncommitted` before its first commit) and its description, then a count. Filters combine:

- `--status open` or `--status open,wontfix` - only those statuses.
- `--text cookie` - only findings with those words anywhere in the file, in any case.
- `--since 2026-01-31` - only findings changed on or after that day; an uncommitted finding always passes.

`--first` keeps one of what the filters left: the open finding first by file name among those at the highest severity any open one has (Pick a finding to work on, below).

`--stat` prints, in place of the list, a coloured grid counting the kept findings by severity and status, with totals: `go run ./cmd/findings --stat`, or `--stat --since 2026-01-31` for what has moved lately.

The tool refuses a file that does not follow the format, naming the file and the line, so a listing that runs clean is also the check that every finding is well formed. Read a finding's details with the Read tool on the path the listing gives.
