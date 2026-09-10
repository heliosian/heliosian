# Data model

The tabs, columns, and validation rules are in `internal/birthday`; this file carries only what reading that code cannot tell you.

The app's data lives in one Google Sheet, `Birthdays`, in the community shared drive, reached through drive membership like the other sheets. `BIRTHDAY_SHEET` names it, and `cmd/createtabs` reads that variable to lay it out from an empty spreadsheet titled `Birthdays`.

## Tabs

- `Birthdays` — Email, Birthday, Newsletter Override, Participation, Note. One row per staff member. Participation is blank, `Skip`, or `No Newsletter`; Birthday may be blank only when Participation is `Skip`, so someone who opted out before their birthday was ever collected still has a row. Note is about the participation wish.
- `Assignments` — Email, Year, Assigned To, Assigned On. One row per staff member per year; unassigning deletes it.
- `Outreach` — Email, Year, Contacted On, Contacted By. One row per staff member per year; undoing deletes it.
- `Donations` — Email, Year, Charity, Note, Recorded On, Recorded By, Used On, Used By. One row per staff member per year. Recorded By is blank on rows imported from the Glide app, which never kept it. Used On and Used By are set together once the newsletter has carried it and cleared together to take that back.
- `Notes` — Email, Note, Added By, Added.
- `Charities` — Name, Donation Link, About, EIN, Allowed, Why Not Allowed, Added On.
- `Newsletter Dates` — Date.
- `Settings` — Key, Value: `Default Charity`, `Year Start`, `Email Subject`, `Email Body`, `No Newsletter Note`, all required.
- `Admins` — Email.
- `Change Log` — appended on every change; never read back.

## Keys are names

Nothing carries an opaque id. A staff member is their email, a charity is its name, and a year is its label. Renaming a charity rewrites every donation naming it and the default-charity setting in one write batch. Emails are stored lowercase, and the load refuses one that is not.

## Years and dates

Every date is a day, `2026-09-24`, at the school; nothing carries a time. Year is `2026 - 2027`, the birthday year starting on the `Year Start` day (`08-14`) of the first calendar year. A birthday's own year is whatever the source knew, and only its month and day are read.

## Yes and No

Allowed is `Yes`, `No`, or blank, and blank means No.

## Settings

`Default Charity` must name an allowed charity. `Email Subject` and `Email Body` are the outreach draft, with `{first name}`, `{name}`, `{newsletter date}`, `{birthday}`, and `{default charity}` filled in by the client; `No Newsletter Note` is appended for anyone whose participation is `No Newsletter`.

## A load either succeeds whole or refuses

The same stance as the other apps: the server does not start, or does not refresh, on a sheet that breaks a rule, and never serves a page quietly missing a birthday. Every per-year row must name someone on the Birthdays tab with a birthday, and every donation must name a listed charity. Someone who opts out keeps the history recorded before they did; only new steps are refused.

## Why the Glide tables were not kept

The app this replaced copied the whole directory into two tables, stored assignment, outreach, and newsletter-use as append-only click logs keyed by opaque row ids, kept the current year in a one-row table, and computed each derived date in the client. The per-year tables keyed by email and year replace the click logs, the directory lookup replaces the copies, the year is derived from the date and one setting, and the derived dates are computed once on the server.

## Import

`cmd/birthdayimport` loads the Glide app's table exports (one CSV per Glide table, dropped into `imports/`, which git ignores) into the empty spreadsheet `BIRTHDAY_SHEET` names, keeping the latest row per staff member and year from each click log, resolving charity row ids to names, and refusing to write anything unless the result loads and every tab is still empty below its header.

## Sample data

`sampledata/birthdays/` holds one CSV per tab: a staff member in every stage, one unassigned and overdue, a newsletter override, a leap-day birthday, summer birthdays that land in the last issue, someone who asked to stay out of the newsletter, someone who opted out entirely, a staff member the directory no longer lists, last year's donations, and a prohibited charity, so every page and rule renders locally.
