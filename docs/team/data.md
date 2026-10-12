# Data model

The portal keeps everything in the data model (`docs/datamodel.md`): its pages read it in one `/api/q` batch of queries (`loadModel` in `web/team/state.js`) and write it in `/api/q` batches (`web/team/writes.js`), checked by the policies like any other write (`internal/db/policy.go`, the joining, adding and manager clauses and Team's admin grants). The Google Sheet `Events` is the portal as it stood before: nothing writes it, and Heliosian's Team widget, When's linked events and the `activity:` Magic Tags still read it (`internal/model`'s `activities*.go`) until they read the data model. `EVENTS_SHEET` names it.

## The tree

Everything the portal shows is a `GROUP` under the top-level `Community` category:

- **A school year** is a `kind=activity` group directly under `Community` (or under nothing, as in the sample data), named for its span (`2026 - 2027`). Its `notify` names the group whose members hear of every sign-up and addition in that year (Email in `team.md`).
- **A heading** is a `kind=category` group. Under a year it is a heading on the Opportunities page, its `listed` saying whether its events show there among everyone else's (it stays in the toolbar with its count either way). Under an activity it is one of that event's own categories, which the things under it are grouped by on its page.
- **An activity** is a `kind=activity` group under a heading or under another activity, to any depth: an event, a committee, a booth, a shift. A root is one whose nearest activity above it is a year, and the roots are what the Opportunities page lists; an activity under a year with no heading lists under Uncategorized.

`order` places a group among its siblings (`docs/storage.md`, Order), written by drag-and-drop on the page and in Volunteer settings. Moving an activity is a change of its `parent`, which its managers may make only to somewhere else they run.

## An activity's cells

- `name`, `description` (markdown, rendered with `/markdown.js`; its links and the highlight it once had are part of it), `location`, `start` and `end` (wall-clock, `2026-09-24 16:00` or `2026-09-24` for a whole day), `timing` (words in place of a date, "All Year").
- `slug` is its short name: its page is `/v/<slug>` and its email list `<slug>@loop.heliosian.com` (every activity is a mail list, `posting` and `replying` `members`). Slugs are unique across every group, so last year's are cleared by hand in the summer before next year's can take them; a copy into the next year has none. Renaming one leaves the old as a hidden `ALIAS` of the group (`groupSlugMoved` in `internal/db/trigger.go`), which the page and the server's redirects both resolve.
- `status`: `open`, `pending` (a suggestion waiting for approval), or `done`. Hidden is not a status: a hidden activity has a blank `visible_to`, and one shown has the `everyone` group there.
- `visible_to` and `members_visible_to`: the `everyone` group or blank. A blank `members_visible_to` is Show volunteers set to Hidden: only its managers see who signed up, and each person their own and their household's rows.
- `join` is Allow volunteers: `others` lets whoever sees it sign up themselves, their household or anyone they see; `below` takes sign-ups only on the things under it (No More); `none` is Volunteers Complete, which no control sets and the popup shows only where it is already set. `capacity` is the number of spots, blank for unlimited; full is the `member=yes` count against it, never a flag.
- `adding` is Allow new activities: `anyone` (Allowed, live), `approval` (Approval needed: anyone but a guest adds one that waits as `pending`), or `managers` (Prohibited). Every year's "Just an Idea" heading is `approval`, which is where Suggest an Idea files; its other headings are `managers`. Each group's `join` and `adding` are its own, never read from above.
- `priority` marks something the community most needs hands for; only a Team admin sets it.
- `flyer` is the poster, a blob shown whole in the page's rail and on the share card ahead of any picture. The banner is the group's first `PHOTO` by `order`: the image picker stores the chosen picture under `activity-images/` (`/api/team/image`, `/api/team/images/import`), and the page sends it on to `/api/do/photo`, which makes the `PHOTO` row.
- `managed_by` is the activity's managers group, `<name> Managers`, made the first time someone is made a co-chair; its members and those of every managers group above run the activity (`manages` in the policies).
- `rsvp_yes` and `rsvp_no` are the answers groups of an invite made on Helios When; each face wears its answer.
- `added_by` and `added`, shown to whoever runs it as "Proposed by".

## Sign-ups

A sign-up is a `MEMBER` row of the activity, `member=yes`, with `note`, `added_by` and `added`. A co-chair is `lead` on that row plus a place in the activity's managers group; making or unmaking one writes both in one batch. Moving a sign-up to another activity is a delete and an insert in one batch. A person is on a thing once.

## Hiding and approving

Hiding an activity or a heading blanks `visible_to` on it and everything under it, headings included, in one batch; showing it again sets `everyone` on all of them, so something hidden on its own before shows with the rest. Approving a pending activity sets it `open`; turning it down hides it. A pending activity is seen by its managers, the Team admins and whoever suggested it.

## The email list

An activity's list reaches its own members. Its settings' "Email list reaches everything under it" adds two `RULE` rows targeting the activity itself with `under` set, the second with `replace_with` `parents`, so the list reaches its co-chairs, everyone signed up for anything under it and their students' parents; unticking it deletes both.

## Everything else

- `SETTING` rows of app `team`: `Intro` (shown under the Sign Up heading) and `Expense Form URL`.
- `REDIRECT` rows of app `team`: `old` a path here (a whole link is read as its path, a bare word as `/v/<word>`), `new` a path here or a whole `https://` address, `added` the day it was made (Redirects in `team.md`).
- `ALIAS` rows: the ids activities and headings had in the `Events` sheet, and old short names, each naming its group.
- `APP` row `team`: its `admins` group is `team-admins`.

## Sample data

`sampledata/datagroups/` holds the 2026 - 2027 year with a Headline Events heading and International Night under it, run by its managers group, with a sign-up; `sampledata/dataconfig/SETTING.csv` holds the two settings.
