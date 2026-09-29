# Data model

The tabs, columns, and validation rules are in `internal/model`'s `birthdays*.go`; this file carries only what reading that code cannot tell you.

The app's data lives in one Google Sheet, `Birthdays`, in the community shared drive, reached through drive membership like the other sheets. `BIRTHDAY_SHEET` names it, and `tools/createtabs` reads that variable to lay it out from an empty spreadsheet titled `Birthdays`.

The weekly export (`birthday.md`) writes to a second spreadsheet, the association's `Staff Birthday List (Shared)`, which `BIRTHDAY_SHARED_SHEET` names: its Newsletter tab takes one row per birthday copied (`model.SharedNewsletterColumns`, laid out by `tools/createtabs`). The app holds it as a store of its own (`sharedSpec` in `internal/model/birthdays_cache.go`) on the one write queue, read at startup and in every refresh, its tab append-only; a store whose every tab is append-only writes no Change Log rows and so needs no Change Log tab (`docs/storage.md`), and this spreadsheet has none. Its Birthdays tab is the association's own and untouched. The spreadsheet is not in the shared drive; it is shared with `directory@` as an editor, and `tools/devenv` finds it by its title. Sample mode writes the rows into memory over `sampledata/birthdayshared/`.

## Tabs

- `Birthdays` — Email, Birthday (`08-20`), Newsletter Override, Participation, Note. One row per staff member. Newsletter Override is blank or the Newsletter Date ID of the issue the birthday is pinned to. Participation is blank, `Skip`, or `No Newsletter`; Birthday may be blank only when Participation is `Skip`, so someone who opted out before their birthday was ever collected still has a row. Note is about the participation wish.
- `Assignments` — Email, Year, Assigned To, Assigned On. One row per staff member per year; unassigning deletes it.
- `Outreach` — Email, Year, Contacted On, Contacted By. One row per staff member per year; undoing deletes it.
- `Donations` — Email, Year, Charity, Note, Recorded On, Recorded By, Used On, Used By. One row per staff member per year. Charity is the charity's Charity ID. Recorded By may be blank. Used On and Used By are set together once the newsletter has carried it and cleared together to take that back.
- `Notes` — Email, Note, Added By, Added.
- `Charities` — Charity ID, Name, Donation Link, About, EIN, Allowed, Why Not Allowed, Added On. Names are unique, since a charity's page is `/charities/{name}`.
- `Newsletter Dates` — Newsletter Date ID, Date. Dates are unique.
- `Settings` — Key, Value: `Default Charity` (a Charity ID), `Year Start`, `Email Subject`, `Email Body`, `No Newsletter Note`, all required; `Outreach CC`, `Request Lead Days` and `Due By Lead Days`, optional.
- `Admins` — Email.
- `Reminders` — Email, Year, Kind, Sent On, Sent To. One row per reminder the app has sent (`ask`, `late`, `donation`), written by the app alone.
- `Invites` — Invite ID, Email, Year, Requested On, Requested By, Sent To, Ask Day, Sent On. One row per calendar invite, kept as history: added unsent by an admin resending (Requested By their address) or by the app's invite sender when a birthday's latest invite no longer matches (Requested By `invites`), and filled in by the sender once it goes - Sent To, Ask Day and Sent On together, blank together until then (`birthday.md`, Who may do what).
- `Team` — Email, Role. One row per person per role: `Volunteer` (offered when a birthday is assigned) or `Comms Team` (carries the donations into the newsletter). Anyone, in the directory or not, by address.
- `Change Log` — written by the store for every cell a change touches, with what the cell held before (`docs/storage.md`, Change Log); never read back. The Change Log from before the app moved onto the store is `Change Log (old)`.

## Keys

A staff member is their email and a year is its label. Emails are stored lowercase, and the load refuses one that is not.

A charity is its `Charity ID`, a newsletter date its `Newsletter Date ID` and an invite its `Invite ID`, minted by the app when it adds one (`id.New` in `internal/id`, checked against every ID and alias the resource API knows, `Registry.Taken`); rows added by hand need one too. The load refuses an id that is not well formed or that two rows share. References hold the id - a donation's Charity, the `Default Charity` setting, a birthday's Newsletter Override - so renaming a charity or moving a newsletter date changes one cell, and everything naming it follows without being rewritten. Removing a newsletter date, alone or with every date from today on, clears the Newsletter Override of each birthday pinned to it in the same change, and those birthdays go back to the usual pick.

## Years and dates

Every date is a day, `2026-09-24`, at the school; nothing carries a time. Year is `2026 - 2027`, the birthday year starting on the `Year Start` day (`08-14`) of the first calendar year. A birthday is a month and day in the same form, `08-20`, and the sheet refuses one with a year on it, so nobody's year of birth is kept.

## Yes and No

Allowed is `Yes`, `No`, or blank, and blank means No.

## Settings

`Default Charity` must be an allowed charity's id; `Outreach CC` may be blank or missing, and is copied on the outreach letter a reminder hands over; `Request Lead Days`, how many days before the newsletter the request is due, may be blank or missing too, and then is eight. `Due By Lead Days`, how many days before the newsletter the charity must be in before the birthday is late, may be blank or missing, and then is two. `Email Subject` and `Email Body` are the outreach draft, with `{first name}`, `{name}`, `{birthday}` (the day without its year, September 26), `{newsletter date}`, `{default charity}`, `{sender}` (the signed-in person's name), and `{last year}` (three lines - `*Last Year's Charity*`, the charity, and the staff member's note from last year - or nothing when there is no last year) filled in by the client; `No Newsletter Note` goes where `{no newsletter note}` sits in the body, or at the end when the body has no such place, for anyone whose participation is `No Newsletter`, and is blank for everyone else. Blank lines an empty placeholder leaves behind are dropped.

## A load either succeeds whole or refuses

The same stance as the other apps: the server does not start, or does not refresh, on a sheet that breaks a rule, and never serves a page quietly missing a birthday. Every per-year row must name someone on the Birthdays tab with a birthday, every donation must name a listed charity's id, and every newsletter override a listed newsletter date's id. Someone who opts out keeps the history recorded before they did; only new steps are refused.

## Sample data

`sampledata/birthdays/` holds one CSV per tab: a staff member in every stage, one unassigned and overdue, a newsletter override, a leap-day birthday, summer birthdays that land in the last issue, someone who asked to stay out of the newsletter, someone who opted out entirely, a staff member the directory no longer lists, last year's donations, and a prohibited charity, so every page and rule renders locally.
