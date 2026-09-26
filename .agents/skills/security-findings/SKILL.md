---
name: security-findings
description: Record, update, search and list the security audit's findings under security-audit/findings/. Use when auditing this codebase for security issues, when writing up or changing the status of a finding, when asked to fix a finding or to pick the next one to work on, or when asked what findings exist, which are open, or whether something has already been found.
---

# Security findings

Read `security-audit/README.md` first, every time. It says what the audit looks for and what it leaves alone, and its Findings section is the whole of how a finding is written: the file's format and name, each field, the statuses, the severities, and what `go run ./tools/findings` and its flags do. This skill is only the order of the steps.

## Create a finding

1. Search first with `go run ./tools/findings --text <words>`, so the same thing is not written twice. If it is already there, edit that file.
2. Write `security-audit/findings/<name>.md` with the Write tool, starting from `security-audit/TEMPLATE.md`, filled in as the README's Findings section says.
3. Run `go run ./tools/findings --text <a word from it>` to see that it parses.

## Edit a finding

Edit the file with the Edit tool, to the README's Findings section.

## Pick a finding to work on

Asked to fix a finding, or the next one, without being told which, take the one `go run ./tools/findings --first` names rather than choosing. Read its details with the Read tool, fix what it describes, then set its status to `fixed` with the details naming the change. Add `--with-revisit` when the person asks what is waiting or says the wait is over, and narrow it with the filters when the work is kept to one area. When the person names a finding, or a severity or an app to start from, that is the pick and `--first` is not asked.

## Search and list findings

Run `go run ./tools/findings` with the filters the README's Findings section lists, and read a finding's details with the Read tool on the path the listing gives.
