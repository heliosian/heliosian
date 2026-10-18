# Heliosian

Last verified: 882c6dc3

# Helios Staff Birthdays

# Helios When

# Helios Loop

# Helios Who?

# Data model

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
