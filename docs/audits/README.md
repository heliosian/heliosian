# Audits

How every audit of this codebase is run and how its findings are kept. What each audit looks for and leaves alone is in its own file:

- [code.md](code.md) — code quality: duplication, drift, layering, complexity, second paths, style
- [security.md](security.md) — what a person can do here that they should not be able to

The layout of the system is in `README.md` and `docs/dev.md`.

## Before starting

Read every issue relevant to the audit, open and closed, before looking at the code. Earlier findings say what is already known, what was fixed and how, and what was decided and left, so the audit builds on them instead of rediscovering them and nothing is filed twice. Each audit names the issue types it files under; each type is one search:

    gh issue list --repo heliosian/heliosian --search "type:<Type>" --state all

`gh issue view <number> --repo heliosian/heliosian --comments` shows an issue whole, with the comments that record why it was closed. A finding already filed, open or closed, is not filed again; new evidence goes on the existing issue as a comment, and a closed one that has come back is reopened.

## Doing the audit

- An audit's lists say where to look, not what is wrong. None of their lines is a claim about the code.
- Every finding is read against the current tree before it is filed. A count or a line quoted from memory, or from an earlier pass, is re-checked.
- Files another session has dirty in the working tree describe where that work is headed. A finding against them waits until they land, or says which state it describes.
- The platforms underneath - Go, the browser, Google's APIs, Cloud Run, chromedp, Mailgun, Anthropic - are out of scope for every audit. How this code uses them is in scope; they are not.

## Filing

A finding is a GitHub issue on this repository, with the issue type its audit names. `gh api orgs/heliosian/issue-types` lists the types, and `gh label list --repo heliosian/heliosian` lists the labels. An issue carries the `app:<key>` label of every app whose code it touches. A finding confined to shared packages or `tools/` carries none. No labels are made up for an audit.

The issue carries:

- **Title.** What is wrong, in a few words.
- **Opening.** One or two sentences saying what is wrong and where.
- **Where.** Files and functions, without line numbers, since those go stale.
- **How to see it.** A path and steps a person can follow, or a test that reproduces it.
- **Fix.** What would close it, when that is plain.

Each audit says what its issues add to this and what they leave out. Details are brief: a few short paragraphs at most.

One finding per issue:

- The same mistake in several places is one finding naming them all.
- Two different mistakes in one place are two findings.
- Findings that depend on each other link each other by number.

The body describes the finding as it stands, not the story of how it changed; that is the issue's timeline and the commits'.

## Closing

An issue stays open while the finding stands, including one that waits on something before it can be worked on, with what it waits on in the body. It closes one of three ways, with the body or a comment saying what was done or why not:

- **fixed** - closed as completed once a change closes it, describing the fix.
- **wontfix** - closed as not planned when it is understood and left, or the behaviour turns out deliberate, giving the reason.
- **invalid** - closed as not planned when it turns out not to be a finding, giving the reason.

A closing comment names the commits that fixed it or decided it, and a fix's commit message names the issue.
