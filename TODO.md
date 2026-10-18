# Heliosian

Last verified: bff8f634

# Helios Staff Birthdays

# Helios When

# Helios Loop

# Helios Who?

# Data model

- Family names are recomputed on every PERSON and MEMBER change
  `personFamilyNames` and `memberFamilyName` (`internal/db/trigger.go`) have no column guard, so a phone-number edit recomputes the names of every family the person is in and writes nothing. A guard on the columns `familyName` reads (`name_long`, `hidden`, `deactivated`, `consent`, `source`, the grade columns) would skip that; unmeasured.

# Helios Admin

- Resources' DOCUMENT tab takes seconds to load
  Its one query (`scan` in `web/admin/resources/start.js:31`) reads all ~60,000 DOCUMENT rows with every reference included and answers 24 MB (7 MB on the wire). Its trace: about 2.8s on the server, of which redact ~1.0s, include ~1.1s and encode ~0.5s; the policies take ~0.2s, and `document_visible` is never reached for a super admin. The browser saw ~4s waiting plus ~0.6s downloading.

# Issues

- Store pronunciation recordings in a format an iPhone plays
  Chrome uploads pronunciations as WebM/Opus, which an iPhone can't play. Convert WebM and Ogg recordings to MP4/AAC on upload through `/api/do/pronunciation`, and convert the ones already stored.
  Issue: https://github.com/heliosian/heliosian/issues/196
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
- Say "gradeband" everywhere the code, data, prompts and docs say "band" or "grade band"
  The grade-pair groups are called band, grade band, GradeBand and Gradeband in different places. Use "gradeband" everywhere, including renaming the `GROUP.kind` value `band` to `gradeband` and moving existing rows over.
  Issue: https://github.com/heliosian/heliosian/issues/183
- Give documents thumbnails
  Documents have no preview image. Make a thumbnail for images, PDFs and fetched pages in a worker off the request path, probably attached to `CONTENT`.
  Issue: https://github.com/heliosian/heliosian/issues/182
- Audit GCS buckets and object path layout
  The media bucket's prefixes have accreted over time. Check that each one still needs to be separate, that naming is consistent and nothing is orphaned, and that `docs/storage.md` matches.
  Issue: https://github.com/heliosian/heliosian/issues/181
- Audit /api/do endpoints
  The `/api/do/` verbs grew one at a time. Review them for consistent naming and shapes, who may call each, whether each should be a verb at all, OpenAPI coverage and tests.
  Issue: https://github.com/heliosian/heliosian/issues/180
- Extract an image of each page of a PDF document
  No picture of any PDF page is kept. Render each page to an image, stored as a child document, using a pure-Go renderer.
  Issue: https://github.com/heliosian/heliosian/issues/179
- Add DOCUMENT.superseded_by, with a trigger that keeps it one hop
  Nothing marks a document as replaced by a newer one. Add a `superseded_by` reference, with a trigger that keeps chains flat, and refuse cycles on load.
  Issue: https://github.com/heliosian/heliosian/issues/178
- Give documents page ranges
  Split PDF `pages` documents record their page range only in their name. Add page-range columns so search hits and Ask citations can point at a page.
  Issue: https://github.com/heliosian/heliosian/issues/177
- Add Mike Orlando's high school counseling Google Calendar as a source in Helios When
  Add the HS counseling Google Calendar as a Helios When source, with its events tagged and aimed at the high school classrooms. It needs the calendar's address and sharing confirmed by Mike.
  Issue: https://github.com/heliosian/heliosian/issues/176
- Return people to the page they asked for after a forced sign-in
  After signing in from the login splash, `Auth.login` always redirects to `/`, so the page they asked for is lost. Carry the requested same-origin path through sign-in and redirect back to it.
  Issue: https://github.com/heliosian/heliosian/issues/175
- Serve through an external HTTPS load balancer
  Mutual TLS for system clients like the Veracross import needs an external HTTPS load balancer, since Cloud Run never passes client certificates to the container. Move the domains onto it, restrict ingress to it, and set up a CA for client certificates.
  Issue: https://github.com/heliosian/heliosian/issues/174
- Show a loop email's actual message to the list's members and managers
  Loop pages show only an email's metadata, though the raw message is stored. Show the rendered message and its attachments to the list's members and managers.
  Issue: https://github.com/heliosian/heliosian/issues/168
- Add hca@ as a valid account for logging in
  Allow the hca@ account to sign in to Heliosian sites and host events, to help with the newsletter and sending invites.
  Issue: https://github.com/heliosian/heliosian/issues/165
- After 2026-10-08: stop deleting the host-only session cookie
  `setSession` and `logout` still delete the old host-only session cookie, which no live session uses after 2026-10-08. Remove that deletion and update the tests that cover it.
  Issue: https://github.com/heliosian/heliosian/issues/162
- Uploaded photos are stored and served with their EXIF metadata, including GPS location
  Uploaded images are served with their EXIF metadata, GPS location included. Keep the original unserved, serve a re-encoded copy with no metadata, and move cropping to the server.
  Issue: https://github.com/heliosian/heliosian/issues/159
- Add public view for Celebrate
  Give Celebrate a public view so alumni with tickets can get in.
  Issue: https://github.com/heliosian/heliosian/issues/25
