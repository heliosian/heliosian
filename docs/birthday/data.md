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
- `Settings` — Key, Value: `Default Charity`, `Year Start`, `Email Subject`, `Email Body`, `No Newsletter Note`, all required. Rows under the old theme keys (`Sidebar Color` and the like, from the Appearance panel that existed on 2026-09-16) are passed over if the tab still holds them.
- `Admins` — Email.
- `Reminders` — Email, Year, Kind, Sent On, Sent To. One row per reminder the app has sent (`ask`, `late`, `donation`), written by the app alone.
- `Team` — Email, Role. One row per person per role: `Volunteer` (offered when a birthday is assigned) or `Comms Team` (carries the donations into the newsletter). Anyone, in the directory or not, by address.
- `Change Log` — appended on every change; never read back.

## Keys are names

Nothing carries an opaque id. A staff member is their email, a charity is its name, and a year is its label. Renaming a charity rewrites every donation naming it and the default-charity setting in one write batch. Emails are stored lowercase, and the load refuses one that is not.

## Years and dates

Every date is a day, `2026-09-24`, at the school; nothing carries a time. Year is `2026 - 2027`, the birthday year starting on the `Year Start` day (`08-14`) of the first calendar year. A birthday's own year is whatever the source knew, and only its month and day are read.

## Yes and No

Allowed is `Yes`, `No`, or blank, and blank means No.

## Settings

`Default Charity` must name an allowed charity; `Outreach CC` may be blank or missing, and is copied on the outreach letter a reminder hands over; `Request Lead Days`, how many days before the newsletter the request is due, may be blank or missing too, and then is eight. `Email Subject` and `Email Body` are the outreach draft, with `{first name}`, `{name}`, `{birthday}` (the day without its year, September 26), `{newsletter date}`, `{default charity}`, `{sender}` (the signed-in person's name), and `{last year}` (three lines - `*Last Year's Charity*`, the charity, and the staff member's note from last year - or nothing when there is no last year) filled in by the client; `No Newsletter Note` goes where `{no newsletter note}` sits in the body, or at the end when the body has no such place, for anyone whose participation is `No Newsletter`, and is blank for everyone else. Blank lines an empty placeholder leaves behind are dropped.

## A load either succeeds whole or refuses

The same stance as the other apps: the server does not start, or does not refresh, on a sheet that breaks a rule, and never serves a page quietly missing a birthday. Every per-year row must name someone on the Birthdays tab with a birthday, and every donation must name a listed charity. Someone who opts out keeps the history recorded before they did; only new steps are refused.

## Why the Glide tables were not kept

The app this replaced copied the whole directory into two tables, stored assignment, outreach, and newsletter-use as append-only click logs keyed by opaque row ids, kept the current year in a one-row table, and computed each derived date in the client. The per-year tables keyed by email and year replace the click logs, the directory lookup replaces the copies, the year is derived from the date and one setting, and the derived dates are computed once on the server.

## Import

`cmd/birthdayimport` loads the Glide app's table exports (one CSV per Glide table, dropped into `imports/`, which git ignores) into the empty spreadsheet `BIRTHDAY_SHEET` names, keeping the latest row per staff member and year from each click log, resolving charity row ids to names, and refusing to write anything unless the result loads and every tab is still empty below its header.

## Sample data

`sampledata/birthdays/` holds one CSV per tab: a staff member in every stage, one unassigned and overdue, a newsletter override, a leap-day birthday, summer birthdays that land in the last issue, someone who asked to stay out of the newsletter, someone who opted out entirely, a staff member the directory no longer lists, last year's donations, and a prohibited charity, so every page and rule renders locally.
