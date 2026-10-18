# Heliosian

- Birthdays widget's All tab heads each birthday with its newsletter issue date, unlabelled
  The All tab (admins only) groups rows by `newsletter` with the generic `dayBar` (`web/home/widgets.js:418`), so Mike Orlando sits under "Fri · Oct 30", the issue he is in, while his birthday is Mon Nov 2. Rows show assignee and charity, never the birthday itself, so the heading reads as the birthday. Label the heading as the issue, or show the birthday on the row.

# Helios Staff Birthdays

- My Jobs' empty Do Now reads backwards
  With no staff awaiting outreach or a response, Do Now says "There's work to be done!" (`web/birthday/pages/jobs.js:12`) above an empty list.

# Helios When

- Editing an event doesn't change its sharing
  The edit form shows the sharing picker and sends `sharing`, but `editEvent` (`web/when/writes.js:117`) never writes `listed`, `visible_to` or `members_visible_to`; only `addEvent` does. The picker also starts at Invite Only for any event whose `source` isn't `sheet` (`web/when/eventform.js:66`), so it doesn't show the event's real setting.
- The year-calendar PDF import never names its document
  `addCalendarPDF` (`tools/import/calendar.go:48`) sends only `kind`, `url` and `published`, so next year's calendar arrives nameless; this year's (`doc1vbgme8d9uv`) was named by hand.

# Helios Loop

- My Email Lists' intro says every list is an address
  "Each email list is an address at loop.heliosian.com…" (`web/loop/pages/groups.js:62`), but every group is a list and only those with a slug have an address, so the grid rightly shows cards with none (3D Printer, Coffee Carts, …). The sentence is what's wrong.
- New Email List button on the shared rail
  Loop's one allowed rail customization (`web/common/rail.js`); today it is a tab in Loop's tab bar (`web/loop/chrome.js:8`) and a header button on My Email Lists.
- Who?'s and Loop's group pages converging into one page, as the rail did
  Who?'s is `listPage` (`web/who/pages/list.js`), Loop's `groupPage` (`web/loop/pages/group.js`); they share only the rail and the route shape.

# Helios Who?

- Sidebar "Running" lists same-named groups with nothing to tell them apart
  Rows are names alone (`listName`, `web/common/rail.js:175`). "Tech Team" shows three times: `grpJluA6GycJvc` (group), `grpRXvo3HBnAPa` (HCA-Team activity) and `tech` (the list). "LARP Junior League" twice: `grp6nIVc9H5y08` and `grpykdcGhNGQEC`, both kind event with no `start` under the LARP category, the first with no one in it. "Heliosian Team" twice: `grpybWP2JsCH88` and `team`.
- Guest PERSON rows duplicating people in the directory
  128 `source = guest` people share a `name_show` with a non-guest, e.g. Sara Palanki: `perH2scCSkaDit` (the Ospreys 5th grader) and `perOzsspFugZFy` (guest, no email, no family, a guest of Ravi Palanki); Rhea Palanki likewise. Search returns both. `person-at` matches by address, so a guest made without one never finds the real person.
- `/groups/<id>` opens only directory groups and the viewer's own lists
  `groupKeyOf` (`web/who/state.js:80`) looks only in `tags` and `lists`, and `lists` holds only groups the viewer runs or is in (`state.js:282`). Every other group `Model.Link` sends here says "Not here": a plain group the viewer only sees (`/groups/grp9AomK6zhG7r`, Spelling Bee), and every Team, Schedule or Celebrate heading of kind `category` (`/groups/grpdwsPcjueNqv`, Activities).

# Data model

- A row's href depends on which columns the query asked for
  `Model.Link` (`internal/db/link.go`) picks the app from `kind`, and MCP (`internal/tools/tools.go:179`) and Ask (`internal/ask/turn.go:70`) call it on the row as `columns` trimmed it. `(from GROUP (where (= id "grp66iuvIijMm6")) (columns name))` gives Alumni Night `https://who.heliosian.com/groups/grp66iuvIijMm6`, a "Not here" page; with `kind` among the columns it gets its When page. Any event, activity or party read without `kind` links to Who?, and without `slug` to its ID rather than its slug.
- Committed triggers wake every worker on a table for any change to it
  `committedTriggers` (`internal/db/trigger.go`) deliver every change on a table to every inbox naming it, whatever the operation or columns, and fetch, extract, sweep, classify and digest take each delivery as a wake and scan the whole `DOCUMENT` table. The classifier setting `classified` on one email wakes all five, itself included. Declaring the columns each inbox cares about on its trigger would cut that; measure first, with a `took` log on each worker's pending scan read over a busy hour. Staged and delete triggers gain nothing from declared filters: all but two also test a row value (a `kind`, a value becoming yes), so their early returns stay.
- Family names are recomputed on every PERSON and MEMBER change
  `personFamilyNames` and `memberFamilyName` (`internal/db/trigger.go`) have no column guard, so a phone-number edit recomputes the names of every family the person is in and writes nothing. A guard on the columns `familyName` reads (`name_long`, `hidden`, `deactivated`, `consent`, `source`, the grade columns) would skip that; unmeasured.

# Helios Admin

- Resources' DOCUMENT tab takes seconds to load
  Its one query (`scan` in `web/admin/resources/start.js:31`) reads all ~60,000 DOCUMENT rows with every reference included and answers 24 MB (7 MB on the wire). Its trace: about 2.8s on the server, of which redact ~1.0s, include ~1.1s and encode ~0.5s; the policies take ~0.2s, and `document_visible` is never reached for a super admin. The browser saw ~4s waiting plus ~0.6s downloading.

# Search

- Search answers empty for about a minute after each start
  The index starts on `emptyView()` (`internal/db/search.go:169`) and `Search` answers from it with no readiness check until the first full pass has loaded every entry and trained its k-means cells.
