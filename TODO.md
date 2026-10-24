# Heliosian

Last verified: bff8f634

# Helios Staff Birthdays

# Helios When

# Helios Loop

# Helios Who?

# Data model

- Image search writes STOCK rows on the request path
  `GET /api/<app>/images/search` records every hit as a `STOCK` row (`db.Stock.Remember`, `internal/db/stock.go`) before answering, and a Documents-sheet write takes about 10 s, so a search answers in 10 s or more (38 s and 50 s measured while another write ran). Answer with the library's hits at once and write the `STOCK` rows after, off the request path; picking a hit still imports it through its `STOCK` row.

- Remove credit and license data from the STOCK sheet
  `STOCK.credit` and `STOCK.license` (`internal/db/schema.go`) hold each image-search hit's credit line and license as the library worded them. Drop both columns, the code that fills them in `internal/imagesearch`, and their values in the production sheet.

- Family names are recomputed on every PERSON and MEMBER change
  `personFamilyNames` and `memberFamilyName` (`internal/db/trigger.go`) have no column guard, so a phone-number edit recomputes the names of every family the person is in and writes nothing. A guard on the columns `familyName` reads (`name_long`, `hidden`, `deactivated`, `consent`, `source`, the grade columns) would skip that; unmeasured.

# Issues

- Wiki: comments on specific passages of a page
  Readers can't respond to a wiki page. Let them comment on a selected passage or the whole page, with replies, resolving, and notifications to the author and the people in a thread.
  Issue: https://github.com/heliosian/heliosian/issues/185
- Wiki: keep each page's edit history
  Wiki saves overwrite the page's content and the old version is swept away. Record every save as a revision, with a History view, diffs between revisions, and restore.
  Issue: https://github.com/heliosian/heliosian/issues/184
- Give documents thumbnails
  Documents have no preview image. Make a thumbnail for images, PDFs and fetched pages in a worker off the request path, probably attached to `CONTENT`.
  Issue: https://github.com/heliosian/heliosian/issues/182
- Extract an image of each page of a PDF document
  No picture of any PDF page is kept. Render each page to an image, stored as a child document, using a pure-Go renderer.
  Issue: https://github.com/heliosian/heliosian/issues/179
- Add DOCUMENT.superseded_by, with a trigger that keeps it one hop
  Nothing marks a document as replaced by a newer one. Add a `superseded_by` reference, with a trigger that keeps chains flat, and refuse cycles on load.
  Issue: https://github.com/heliosian/heliosian/issues/178
- Add Mike Orlando's high school counseling Google Calendar as a source in Helios When
  Add the HS counseling Google Calendar as a Helios When source, with its events tagged and aimed at the high school classrooms. It needs the calendar's address and sharing confirmed by Mike.
  Issue: https://github.com/heliosian/heliosian/issues/176
- Show a loop email's actual message to the list's members and managers
  Loop pages show only an email's metadata, though the raw message is stored. Show the rendered message and its attachments to the list's members and managers.
  Issue: https://github.com/heliosian/heliosian/issues/168
- Add public view for Celebrate
  Give Celebrate a public view so alumni with tickets can get in.
  Issue: https://github.com/heliosian/heliosian/issues/25
