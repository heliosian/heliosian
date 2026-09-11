# Birthday team

Helios Staff Birthdays is where the Helios Community Association's birthday team celebrates every staff member's birthday with a donation to a charity of their choice, announced in the school newsletter. It walks each birthday through a short pipeline: assign a volunteer, wait until the request is due, ask the staff member which charity they want, record the answer, and mark it once the newsletter has carried it. It replaces the Glide app of the same name and serves at birthday.heliosian.com. The sheet is `Birthdays`, the package is `internal/birthday`, and the API is `/api/birthday/`.

## Entities

- **Birthday** — a staff member's row: their email, birthday, a newsletter override (the issue that should announce it when the usual pick is wrong), and their participation, a standing wish about taking part. The birthday's year is kept when it is known and never shown, and the birthday itself may be blank only for someone who opted out.
- **Assignment** — which team member is handling a staff member this year.
- **Outreach** — that the staff member was contacted this year, when, and by whom.
- **Donation** — the staff member's charity and note for the year, when it was recorded and by whom, and once the newsletter has carried it, when and by whom.
- **Note** — free text about a staff member from anyone on the team, kept across years.
- **Charity** — name, donation link, blurb, EIN, and whether the association can give to it. A charity that cannot be given to keeps its reason, so the next volunteer who hears the same name knows why.
- **Newsletter date** — one row per issue of the school newsletter.

Names, titles, departments, and photos come from the directory model at request time, like the volunteer portal; the sheet stores only emails. A row is looked up through the directory's email aliases, so one recorded under a secondary address still finds its person. A staff member the directory no longer lists has left the school: their row and history stay in the sheet, but they appear nowhere in the app.

## The birthday year

A birthday year turns over on the day the `Year Start` setting names, August 14, and is written as both calendar years, `2026 - 2027`. Every per-year record carries that label, so last year's donation is a lookup, not a guess. A birthday's occurrence this year is its month and day placed inside the year, and February 29 lands on the 28th when there is no 29th.

## Derived dates

Nothing stores what the sheet can compute:

- **Newsletter date** — the first newsletter date on or after the birthday within the year, or, for a birthday after the last issue, that last issue, so a summer birthday is announced in June rather than forgotten. The override on the birthday wins when set. With no issue in the year at all there is no newsletter date, and the staff member waits.
- **Request by** — ten days before the newsletter date, when outreach is due.

## Stages

A birthday's stage this year follows from what has been recorded, in order: `Complete` once the donation has been used in the newsletter (or recorded at all, for someone who asked to stay out of the newsletter); `Awaiting Newsletter` once a donation is recorded; `Awaiting Response` once contacted; `Awaiting Outreach` once the request-by day has arrived; `Wait` before then. Assignment is not a stage: an unassigned birthday sits in whatever stage its progress says, and the Process page groups the unassigned ones separately so they get picked up.

## Opting out

Two levels, both recorded in the Participation column of the staff member's Birthdays row and editable by anyone on the team:

- **Skip** — not in the process at all. They show on the Skipped page and nowhere else, and the pipeline refuses to act on them.
- **No Newsletter** — asked and donated for as usual, but never included in the newsletter. Their page says so, their stage jumps to complete once the donation is recorded, and the outreach email carries the `No Newsletter Note` setting so they never have to remind the team.

Someone who has opted out of the directory itself is not listed as staff there and so never appears among the missing birthdays.

## Who may do what

- **Anyone signed in** works the pipeline on any staff member: assigns, contacts, records donations and notes, marks newsletter use, adds and corrects birthdays, records participation wishes, and adds charities.
- **Admins**, the Admins tab plus the platform super admins (`docs/config.md`), decide which charities are allowed and remove them, manage the newsletter dates, remove birthdays, change the settings, and manage the admin list from `/admin`.

## Pages

One shell, `web/birthday/index.html`, routes every page on the client.

- **My Jobs** (`/`) — the viewer's assignments in three tabs: My Tasks (awaiting outreach or a response), Wait, and All Done.
- **Process** (`/process`) — everyone, in tabs: Unassigned, grouped by stage with the overdue ones first, then one tab per stage. Every row offers Assign to Me and, when outreach is due, Mark: Contacted.
- **Staff** (`/staff/{email}`) — the person, their three dates, the five steps as a dark band with each step's actions, the donation band with this year beside last year (or the default charity), and their notes. The Email button opens a draft from the settings' template with the person's details filled in.
- **Calendar** (`/calendar`) — a month grid of birthdays colored by stage, with newsletter days marked.
- **Charities** (`/charities`, `/charities/{name}`) — allowed and prohibited lists, and a page per charity showing who chose it.
- **Newsletters** (`/newsletters`) — the year's issues with who lands in each and a copy of the issue's donations for the communications team; admins add the next week or any date.
- **Skipped** (`/skipped`) — staff the directory lists with no birthday on file, grouped by department, and staff who asked to be left out.
- **Admin Tools** (`/admin`) — the settings and the admin list.

On phones the top bar carries the page title and a drawer for the rest, and a bottom tab bar holds My Jobs, Process, and Calendar, matching the app this replaces.

## Editing

As in the other apps: in place, through modals and one-click actions, with every change applied to the in-memory model first and then written to the sheet through the shared write queue and appended to the Change Log. A change the rules reject is refused before anything is written. A charity with donations cannot be deleted, only marked not allowed; a birthday cannot be removed while records name it.

## Sign-in and install

Everything sits behind Google sign-in restricted to the school domain, sharing a session with the other apps on the same tier. It installs to a home screen like the directory (`docs/who/pwa.md`), with the icons and splash art the Glide app used under `web/public/birthday/`; the splash battery is JPEG there rather than PNG because the art is a photographic gradient.
