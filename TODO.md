# Heliosian

Last verified: bff8f634

# Helios Staff Birthdays

# Helios When

# Helios Loop

# Helios Who?

# Data model

- Keep every stored byte in CONTENT
  Only documents and mail keep their bytes as `CONTENT` rows under `content/`. Every other Blob cell names an object in a folder of its own: `PHOTO.original`, `reencode`, `crop` and `thumbnail` under `photos/`, `PERSON` and `GROUP` `pronunciation_original` and `pronunciation` under `pronunciation/`, `GROUP.flyer` in an app image folder, `REPORT.screenshot` under `feedback/`. Image search keeps its records, previews and imported pictures under `stock/` by source ID, wiki pages name their pictures under `wiki-images/`, and the app image folders (`imageFolders`) hold uploads a page posts again to `/api/do/photo`, plus `-thumb.jpg` objects beside them.
  Make each of those a reference to a `CONTENT` row: Blob columns become `CONTENT` refs, the stock record holds the `CONTENT` IDs of its preview and picture, wiki pages name pictures by `CONTENT` ID, and uploads become `CONTENT` rows, so the app image folders, `imageFolders`, the per-app folder routes and the `-thumb.jpg` objects go. `CONTENT`'s read policy gains a clause for each referencing column, keeping `PHOTO.original` and `pronunciation_original` private. The content sweeper then deletes whatever nothing references, which also clears the 1,735 unreferenced objects in `photos/`. Flyers and wiki pictures, today served exactly as uploaded with any EXIF and GPS tags, get `PHOTO`'s treatment: a private original and a served re-encode with every tag dropped (issue #159). Move the existing objects with a one-off `local/` tool run after the deploy.
- Family names are recomputed on every PERSON and MEMBER change
  `personFamilyNames` and `memberFamilyName` (`internal/db/trigger.go`) have no column guard, so a phone-number edit recomputes the names of every family the person is in and writes nothing. A guard on the columns `familyName` reads (`name_long`, `hidden`, `deactivated`, `consent`, `source`, the grade columns) would skip that; unmeasured.

# Issues

- Add sample pronunciation recordings in WebM/Opus and MP4/AAC
  The sample data has no pronunciation recordings, so nothing in development has a play button. Add synthesized recordings in WebM, MP4 and MP3 for a few sample people and one family group, and document them in `docs/dev.md`.
  Issue: https://github.com/heliosian/heliosian/issues/194
- Word the approval page for the iPhone app, which changes things
  The OAuth approval page says the client "wants to read" and "can change nothing", which is untrue of the iPhone app, since it makes edits. Word the page per client from `appRedirects`, so the app's page says it can change what you can change.
  Issue: https://github.com/heliosian/heliosian/issues/192
- Keep the queries released iOS builds send parsing
  Released iPhone builds send fixed queries, and a renamed or removed column makes them fail with a 400. Check in each released version's queries under `internal/db` testdata and add a test that parses them all with `db.Parse`.
  Issue: https://github.com/heliosian/heliosian/issues/189
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
