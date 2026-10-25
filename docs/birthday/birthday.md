# Birthday team

Helios Staff Birthdays is where the Helios Community Association's birthday team celebrates every staff member's birthday with a donation to a charity of their choice, announced in the school newsletter. It walks each birthday through a short pipeline: assign a volunteer, wait until the request is due, ask the staff member which charity they want, record the answer, and mark it once the newsletter has carried it. It serves at birthday.heliosian.com. It is an app over the data model (`docs/datamodel.md`, Staff Birthdays, for the tables and who may write what): the pages read and write through `/api/q`, the mail and the weekly copy are `internal/db/birthdaymail.go`, and `POST /api/do/describe-charity` is its one route of its own.

## Data

- **Staff** — every person but a guest with a `PERSON.birthday`, and every effective member of `staff` without one, who shows as missing a birthday. A guest's birthday is kept but never in the process, and their past years' records stay. The birthday is a month and day; its year is unused. `PERSON.birthday_participation` is the standing wish about taking part.
- **A birthday's year** — the generated `BIRTHDAY` row (its day, its newsletter issue, its days to ask by and give by) and the team's `BIRTHDAY_YEAR` record of it (who has it, the outreach, the charity and the staff member's words, the newsletter's use).
- **Charity** — name, donation link, the newsletter's sentence, EIN, and whether the association can give to it. A charity that cannot be given to keeps its reason, so the next volunteer who hears the same name knows why.
- **Newsletter issue** — a plain group under the series the `Newsletter` setting names, its day in `start`.
- **The team** — the Staff Birthdays activity (slug `bday`), whose members are the volunteers, and its leads group, the Comms Team. The Birthdays app is open to the Birthdays Viewers group, whose rules take in both and the Birthdays Admins group.

Names, titles, departments and photos are the person's own in the data model.

## The birthday year

A birthday year turns over on the day the `Year Start` setting names, and is written as both calendar years, `2026 - 2027`; the data model keeps it as the first, `2026`. A birthday's occurrence in the year is its month and day placed inside it, and February 29 lands on the 28th when there is no 29th.

## Derived dates

`BIRTHDAY` carries them (`birthdayRows` in `internal/db/birthdays.go`):

- **Newsletter date** — the last issue before the birthday within the year, so the announcement goes out ahead of the day and never on it: each issue carries the birthdays between it and the next. A birthday before the year's first issue lands in that first issue, late rather than lost, and a summer birthday in the last issue of the year. A year with no issue has no `BIRTHDAY` rows, and its staff wait.
- **Request by** — the `Request Lead Days` setting before the newsletter date, when outreach is due.
- **Due by** — the `Due By Lead Days` setting before the newsletter date, when the charity must be in; a birthday without one then is late.

## Stages

A birthday's stage this year follows from its record, in order: `Complete` once the newsletter has used it (or once a charity is recorded at all, for someone who asked to stay out of the newsletter); `Awaiting Newsletter` once a charity is recorded; `Awaiting Response` once contacted; `Awaiting Outreach` once the request-by day has arrived; `Wait` before then. Assignment is not a stage: an unassigned birthday sits in whatever stage its progress says, and the Process page groups the unassigned ones separately so they get picked up. The stages, the urgency chips, the next step and the late steps are worked out in the browser, the same for the app, Heliosian's widget and the toolbar (`web/common/birthdays.js`).

## Opting out

`PERSON.birthday_participation`, which anyone on the team sets:

- **Skip** — not in the process at all. They show on the Skipped page and nowhere else, and the policies refuse a year's record for them.
- **No Newsletter** — asked and given for as usual, but never included in the newsletter. Their page says so, their stage jumps to complete once the charity is recorded, and the outreach email carries the `No Newsletter Note` setting so they never have to remind the team.

## Who may do what

- **Someone not on the team** is asked, whenever the app opens for them, whether they want to be on the birthday team, and cannot get past the question: Join the Team signs them up to the Staff Birthdays activity, which takes joining directly, and so puts them in Birthdays Viewers; No thanks sends them back to the Heliosian home. The app knows who is on the team by `(birthday_team)`.
- **Anyone on the team** works the pipeline on any staff member: assigns, contacts, records charities, marks newsletter use, adds and corrects birthdays, records participation wishes, and adds charities, which are allowed when they add them. The charity form is the name, **Generate with Claude**, the donation link and the sentence: the button asks Claude (`internal/describe`, through `POST /api/do/describe-charity`, which answers only someone who may add a charity) to find the charity's site, return the page where a donation is made, and write the newsletter's one sentence about it, read from the site rather than guessed and in the register the list already uses, and fills the two fields below for the person to read over; nothing is saved until they save the form, and the fields can as well be typed. Past the hourly window every call to Claude shares (`internal/claude`; `docs/ask/data.md`) it asks the person to wait; a name and link longer than `maxPrompt` in `internal/describe/describe.go` are refused. The donation form shows the chosen charity's sentence, shortened with a read-more, above the staff member's note.
- **The comms team** - on the Comms Team, not an admin, and holding no birthday this year - sees the newsletters alone: the front page opens on them and the rail is the newsletters and the calendar.
- **Admins**, Birthdays Admins and the platform super admins, also decide which charities are allowed and remove one no year's record names and that is not the default, add, move and remove the newsletter issues, clear every issue from today on to start a list over, remove a year's record, change the settings, and keep the volunteers, the Comms Team and the admins from `/admin`.

## Mail

The mailer (`internal/db/birthdaymail.go`) runs on Cloud Run alone, woken by every committed change to `BIRTHDAY_YEAR`, `PERSON`, `GROUP` and `SETTING` and by the store's tick. Every message is a `MESSAGE` about the year's record, with a `RECIPIENT` for the assignee, written before it goes out (`docs/datamodel.md`, Mail), from `birthday@reply.heliosian.com`; every message about one birthday's year shares its subject and thread, so it reads as one thread.

- **Invites** — the assignee of a birthday not complete gets an email with a calendar invite, an all-day event on the day to ask, named for the staff member, carrying their birthday, the ask-by and newsletter dates, last year's charity and note, and a link to their page. One is owed whenever the record's `invite_day` is not the day to ask, or its last invite went to someone else: a new assignee, or a day to ask that moved with an issue, a birthday or a setting. Writing it sets `invite_day`, and the same birthday's invite replaces the event on the calendar.
- **Reminders** — between 7 in the morning and 9 at night, school time, the assignee gets the latest step that has come and that no reminder about the record has covered since its day: on the day to ask, the outreach letter filled in behind a Send the email button that opens it in their mail app; two days after, if the outreach is still not marked done, the same again; on the due-by day, if no charity is recorded, a nudge to record what came. Nothing is sent once the issue's day has passed.
- **A copy that no longer fits** - the record reassigned, completed or gone before it was sent - is stamped failed rather than sent.

## The weekly copy

Once Thursday 11:59 at night, school time, has passed, the mailer copies the birthdays of the week's issue - the first issue in the seven days after that Thursday - that are not yet published into the Newsletter tab of the association's `Staff Birthday List (Shared)` spreadsheet (`BIRTHDAY_SHARED_SHEET`), and then sets each record's `published`. A row is the columns `db.BirthdaySharedColumns` names, each placed under its heading wherever the tab keeps it. Everyone the issue carries goes but whoever asked to be skipped: No Newsletter staff too, their Preference saying so, since the association still gives for them. Whoever has no charity recorded by then gets last year's charity and note, or the default charity when they gave nothing last year or last year's charity is no longer allowed, recorded with nobody as its recorder. The rows are written first, so no birthday is marked published without its row; a server down at the hour copies the issue on its next pass.

## Pages

One shell, `web/birthday/index.html`, routes every page on the client. It is the shell HCA-Team and Helios Celebrate use: a rail on the left with the app icon and the pages, and across the top the toolbar every app shares (`docs/toolbar.md`). Its colors are the shared palette every app uses (`docs/who/design.md`, Palette).

- **My Jobs** (`/jobs`, and the front page once everyone has someone) — the viewer's assignments in three tabs: Do Now (awaiting outreach or a response), Wait, and All Done.
- **Process** (`/process`) — everyone, by stage, with filters by department, month and assignee. Every unassigned row offers Assign to Me.
- **Unassigned** (`/unassigned`, and the front page while anyone is unassigned; the tab shows only then) — the unassigned birthdays as a list beside the month they fall in, with a switch between the day to ask by and the birthday itself; picking one shows their card with Open Helios Who? and Assign to Me.
- **Staff** (`/staff/{slug}`, also by the person's ID or address) — the person, their three dates, the five steps with each step's actions, the donation band with this year beside last year (or the default charity), and their preference. Until a charity is recorded, the page names the one the weekly copy would use, Use Default records it, and Add Donation starts on it. Compose Email opens a draft from the settings' template with the person's details filled in.
- **Calendar** (`/calendar`) — a month grid of birthdays colored by stage, with newsletter days marked, and the same Show switch as Unassigned's month.
- **Charities** (`/charities`, `/charities/{name}`) — allowed and prohibited lists, and a page per charity showing who chose it.
- **Newsletters** (`/newsletters`, `/newsletters/{date}`) — the year's issues with who lands in each, and for an issue a copy of its donations for the communications team and Mark Used; admins add the next week, any date, or a week's run of dates.
- **Skipped** (`/skipped`, admins alone) — staff with no birthday on file, grouped by department, and staff who asked to be left out.
- **Admin Tools** (`/admin`) — the settings, the volunteers, the Comms Team and the admins, drawn by the Admin Tools builder every app shares (`web/common/admin.js`, `docs/toolbar.md`).

On phones the shared toolbar is pinned on top with a hamburger for the drawer, a teal strip under it carries the page title, and a bottom tab bar holds the first three pages of the rail.

## Brand

The cake-on-a-sun mark and the Staff Birthdays lockups live under `web/public/birthday/brand/`, cut from the app's exports (`docs/brand.md`): `appicon.png` for the home-screen icons; the bare mark (`favicon.png` there) for the favicons, `logo-mark.png` (the drawer's head), the app switch row (`web/public/common/brand/apps/birthday.png`) and the maskables at 62% on the icon's dark teal `#0e4d54`; the teal vertical lockup for the rail (`logo-lockup-vertical.png`, 170px wide); the white one for the login page (`logo-lockup-light.png`); the white horizontal for the admin header (`logo-lockup-light-horizontal.png`); the teal horizontal (`logo-lockup.png`) on the share card. The iOS splash screens are under `brand/splash/`.

## Sign-in and install

Everything sits behind Google sign-in restricted to the school domain, sharing a session with the other apps on the same tier. It installs to a home screen like the directory (`docs/who/pwa.md`), with its icons and splash art under `web/public/birthday/`.
