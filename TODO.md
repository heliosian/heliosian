# Heliosian

Last verified: bff8f634

# Helios Staff Birthdays

# Helios When

# Helios Loop

# Helios Who?

# Data model

# Issues

- Nested queries cause excessive response-formatting work
  A small, deeply nested query makes the shared query formatter do quadratic work, allocating gigabytes outside the query budget. Format in one pass using recorded subtree spans and keep deep subtrees flat past the line width.
  Issue: https://github.com/heliosian/heliosian/issues/210
- Shared notification mail repeats recording and pairs copies by slice offset
  Membership and guest-list notices each implement the record/send/record lifecycle and pair recipient IDs with people by index. Share the lifecycle in `delivery.go` and read each RECIPIENT row's ID and person together.
  Issue: https://github.com/heliosian/heliosian/issues/209
- Home audiences let portal admins take over other groups
  A Home-only admin can add members or rules to any group just by naming it as an app, widget or link audience, taking over its management. Limit those edits to unmanaged top-level plain groups.
  Issue: https://github.com/heliosian/heliosian/issues/208
- Directory email resolution is repeated in sign-in and session revocation
  Sign-in, directory lookup and session revocation each resolve an email to a non-guest person on their own. Use `Model.PersonOf` as the single resolver.
  Issue: https://github.com/heliosian/heliosian/issues/207
- School assignments let directory admins become super admins
  A Who? admin can set a classroom, crew or department override to a privileged group like Super Admins and be joined to it. Require each override to name a group of the matching kind.
  Issue: https://github.com/heliosian/heliosian/issues/206
- Editable host names inject invitation email headers
  Line breaks in a host's profile name inject headers such as Reply-To into invitation mail. Format the From header with `net/mail.Address`.
  Issue: https://github.com/heliosian/heliosian/issues/205
- Invitation routes bypass party ticket permissions
  Members and outside invitation holders can add unpaid party tickets, bypassing capacity, eligibility and paid-ticket rules, through the invitation routes. Apply the MEMBER policies as the inviting person to guest writes.
  Issue: https://github.com/heliosian/heliosian/issues/204
- Invitation routes erase request types at registration
  The JSON invitation routes register through `any`-typed factories and type assertions that can mismatch and panic. Replace them with one generic model-action HTTP helper.
  Issue: https://github.com/heliosian/heliosian/issues/203
- Stock writes repeat hits in parallel slices
  The stock-image write queue carries each hit in two parallel slices read by offset. Pass only the writes and read each one's hit and ID together.
  Issue: https://github.com/heliosian/heliosian/issues/202
- Mail headers store a name separately from their raw text
  `mail.HeaderLine` keeps `Name` and `Raw` as independent fields that can disagree. Make it a named string and derive the name and value from it.
  Issue: https://github.com/heliosian/heliosian/issues/201
- Members can make themselves super admins through borrowed RSVP groups
  Any member can create a group whose RSVP reference names Super Admins and then join it through the answer-membership grant. Enforce helper-group ownership in `Model.Authorize`.
  Issue: https://github.com/heliosian/heliosian/issues/200
- An OAuth code can mint a fresh token after sign-out
  An approved OAuth code can be exchanged again after the member signs out, yielding a token that outlives the sign-out. Sign the approval time into the code and reject codes approved before the last sign-out.
  Issue: https://github.com/heliosian/heliosian/issues/199
- Wiki: comments on specific passages of a page
  Readers can't respond to a wiki page. Let them comment on a selected passage or the whole page, with replies, resolving, and notifications to the author and the people in a thread.
  Issue: https://github.com/heliosian/heliosian/issues/185
- Wiki: keep each page's edit history
  Wiki saves overwrite the page's content and the old version is swept away. Record every save as a revision, with a History view, diffs between revisions, and restore.
  Issue: https://github.com/heliosian/heliosian/issues/184
- Extract an image of each page of a PDF document
  No picture of any PDF page is kept. Render each page to an image, stored as a child document, using a pure-Go renderer.
  Issue: https://github.com/heliosian/heliosian/issues/179
- Add DOCUMENT.superseded_by, with a trigger that keeps it one hop
  Nothing marks a document as replaced by a newer one. Add a `superseded_by` reference, with a trigger that keeps chains flat, and refuse cycles on load.
  Issue: https://github.com/heliosian/heliosian/issues/178
- Show a loop email's actual message to the list's members and managers
  Loop pages show only an email's metadata, though the raw message is stored. Show the rendered message and its attachments to the list's members and managers.
  Issue: https://github.com/heliosian/heliosian/issues/168
- Add public view for Celebrate
  Give Celebrate a public view so alumni with tickets can get in.
  Issue: https://github.com/heliosian/heliosian/issues/25
