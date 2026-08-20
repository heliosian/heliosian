# Data model

The entities the directory serves, and how they are assembled. Structured data lives in one Google Sheet; blobs (photos, audio) are files beside it in the community shared drive, discovered by naming convention. Access rides on drive membership: the service account is a content manager of the drive, never granted through project IAM. The organized model is held in memory — nothing computed is ever written back to the sheet.

## Spreadsheet layout

| Tab | Written by | Purpose |
|---|---|---|
| Veracross Import | import tool | The raw Veracross student export, rewritten wholesale by the import tool on each refresh. Read-only to the serving app; no local edit survives here. |
| Name to Email | hand | Maps a student's name to their school email for import rows where the email cell is empty or wrong, so every person can be keyed by email downstream. |
| Overrides | hand + app | The entire local layer: admin corrections, app-written self-service text, and added people, in canonical model columns keyed by email. |
| Change Log | app | Append-only audit trail mirroring the Overrides columns: one row per change, holding the previous values. |

School structure lives nowhere in the sheet: membership and the classroom and crew names themselves derive from person records, and the remaining fixed structure — band identities, grade progression, department order — is code constants (see Classrooms and grades).

## Load pipeline

1. Read the raw import.
2. Transform each import row into canonical person and household records (explosion, below). The Name to Email mapping applies during this step, since it fixes the key everything else uses.
3. Apply the Overrides tab onto the canonical records by email: unflagged rows patch existing records, flagged rows create new ones.
4. List the media drive and attach photo and pronunciation blobs to records by filename convention.
5. Hold the organized result in memory; the server refuses to start if the load fails, and the model reloads periodically.

Because Overrides rows are authored post-transform, their values skip import normalization — so canonical-value validation runs on every layer, not just the import.

## Veracross Import format

One row per student, 29 columns, with the student's households denormalized into the row: up to two households (separated parents), up to two adults each.

| Columns | Content |
|---|---|
| `entry_sort_name`, `student_full_name` | Sort key; student name in `Preferred (Legal) Last` form when a preferred name is set, plain `First Last` otherwise |
| `student_classifications` | JSON with exactly `grade_level` (`Kindergarten`, `Grade 1` … `Grade 8`) and `homeroom` (compound `Crew Classroom` string) |
| `student_email`, `student_phone_mobile` | Empty email for a substantial minority of students (roughly a fifth, across grades) — these rows require a Name to Email entry |
| `household_N_phone`, `household_N_address` | N ∈ {1, 2}; address arrives at whatever granularity the family shares with the school: street-level, city-only, or empty |
| `household_N_person_M_full_name`, `_email`, `_email_2`, `_phone_mobile`, `_phone_business` | M ∈ {1, 2}; adult emails are always `@heliosschool.org` but arrive mixed-case; `email_2` is unused in practice |

Staff do not appear in this export. They enter either through a dedicated staff import run through the same pipeline, or as flagged rows in Overrides — this choice is open.

## Transform

The explosion turns each import row into one student record plus up to four adult records and one or two household records:

- **Person identity**: email, lowercased, is the key everywhere. Rows with a blank or wrong `student_email` get theirs from Name to Email; a mapping that matches zero or multiple import rows is fatal.
- **Names**: `Preferred (Legal) Last` parses into preferred name, legal name, and display name; plain names pass through. Source rows with swapped or malformed name fields are repaired in Overrides, not by transform heuristics.
- **Roles**: derived from where a person appears — a row's student is a student, a household adult is a parent, staff sourcing marks staff. Combinations are valid (staff who are also parents).
- **Adults deduplicate** across sibling rows by email; conflicting values across a parent's appearances are fatal rather than silently last-one-wins.
- **Homeroom** splits positionally: classrooms are single-word bird family names, so the last word is the classroom and everything before it is the crew (`Great Blue Herons` → crew `Great Blue`, classroom `Herons`; a single-word homeroom like `Hummingbirds` is a crewless classroom). The school's own naming convention backs this — crew + classroom compounds are real species names, classrooms the one-word family.
- **Households** group by the set of adult emails in them, order-insensitively — `person_1`/`person_2` ordering is Veracross's choice and must not affect identity.

## Overrides

Canonical model columns, keyed by lowercased email, one row per person. Three kinds of content share the tab, distinguished only by authorship and one flag:

- **Corrections** (hand): fix anything the import gets wrong — swapped name fields, bad phone numbers — and carry person flags with no import source, like room-parent assignments and the new-to-Helios marker.
- **Self-service text** (app): facts, pronouns, preferred name, phone, address — the latter two hideable via the `-` clear — and the Opted Out flag. Every self-service edit warns that it doesn't affect the values shown in Veracross. The app writes these cells directly; moderating a contribution is the same act as any other correction. Photo and pronunciation uploads go straight to the media drive and never touch the sheet.
- **Additions** (hand, flagged): people with no import row at all. The flag inverts the source expectation.

Cell semantics are sparse: an empty cell contributes nothing, `-` clears the underlying value. An addition is just an override applied to an empty base record, so the merge logic is uniform; the flag selects the validation instead:

| Flag | Loader expects | Violation |
|---|---|---|
| unset | a matching import person | fatal: orphaned override |
| set | no matching import person | fatal: Veracross now covers this person — unflag the row and delete cells the import supplies |

Flagged rows must supply every field the model requires; unflagged rows can be a single cell. `-` on a flagged row is meaningless (nothing beneath to clear) and reported as useless.

Family-level fields (address, family photo caption, family phone) ride on a parent's row and apply to that parent's household, so a two-household student's families are addressed independently through their respective adults.

**Opted Out** removes the person entirely at load: their record, their membership in families and parent-contact lists, and any room-parent assignment all vanish from the model. Because viewing the directory requires being in it, opting out also locks the person out — they get a permissions error until the school clears the flag. People set it from their own page, parents set it for their kids (each with a confirmation spelling out the consequences), or an admin sets the cell by hand.

Every change to Overrides appends a Change Log row: timestamp, actor, the row's email, then the previous value of each column that changed — `-` marking a previously empty cell, untouched columns left blank. Media uploads are not logged here; the drive archive is their history.

## Media blobs

Photos and pronunciation recordings are files in the media shared drive, named by convention: `<email local part>-photo` and `<email local part>-pronunciation` for people, `<family key hash>-photo` and `<family key hash>-pronunciation` for families. Presence means existence — no sheet cell records a filename — and freshness comes from the file's modified time. Uploads replace the file and archive the previous version; superseded versions stay in an archive folder.

## Person

One record per person, all roles in one shape:

- **Key**: school email, lowercased. Everyone has one — including students whose import row omits it (supplied via Name to Email) — though the youngest students don't yet have access to theirs.
- **Names**: display name, legal name, preferred name, parsed from the import or overridden.
- **Roles**: student, parent, and staff booleans, derived from sourcing; combinations are valid. Display strings derive from the flags.
- **New to Helios**: override-carried flag marking people who just joined the community; drives the matching filter toggle.
- **Pronouns**: optional; a curated list plus a freeform escape hatch.
- **Pronunciation**: optional audio recording of the person's name, from the media drive.
- **Photo**: official portrait or personal upload, from the media drive; people may opt for an illustrated avatar instead.
- **Facts**: optional about-me text — first-person blurbs for students, professional bios for staff.
- **Student fields**: grade, classroom, and crew from the homeroom split; parent contact emails derive from the student's household adults.
- **Staff fields**: job title, department, grade band, and classroom/crew assignment for teaching staff.
- **Contact**: email always; phone optional.
- **Year rollover**: next-year grade and band derive from the grade progression constant, so the directory can flip to the new school year.

## Family

A household groups adults and kids; a student belongs to one household normally, two when parents keep separate households:

- **Key**: a hash of the sorted emails of every member — students and adults alike — so identity is order-insensitive and derives from nothing but membership. Any membership change (new student, student leaves, parent change) produces a new key, deliberately: the family's URL and photo association reset along with its composition.
- **Members**: the adults in the household and the students whose rows name it.
- **Photo and caption**: the family photo from the media drive plus a who's-who description naming everyone in it.
- **Pronunciation**: optional audio recording of the family name, from the media drive.
- **Address**: as much as the family chooses to share — full postal address or just city and state, seeded from the import and updatable via self-service.
- **Phone**: optional household phone.

## Classrooms and grades

Membership and the classroom and crew names derive entirely from person records via the homeroom split; the remaining fixed structure — band identities, grade progression, department order — is code constants. The current shape:

| Band | Grades | Classrooms | Crews |
|---|---|---|---|
| Hummingbirds | K | Hummingbirds | — |
| Halcons | 1–2 | Falcons, Hawks | — |
| Jayvens | 3–4 | Jays, Ravens | — |
| Cospreys | 5–6 | Condors, Ospreys | Pinnacles/Big Sur, River/Sea |
| Hegrets | 7–8 | Egrets, Herons | Snowy/Great, Great Blue/Green |

- **Classroom**: mascot artwork lives on disk under the classroom's name; a classroom has crews exactly when its homerooms carry crew prefixes.
- **Crew**: classroom subdivision with its own logo; its teachers derive from staff records carrying a classroom/crew assignment.
- **Grade band**: pairs of grades share a band with a combined identity, used for browsing, room-parent organization, and band-colored styling (see `docs/design.md`). Grade → next-grade is positional in the ordered grade list, and next band follows from next grade.
- **Room parents**: parent assignments per grade band, carried as an Overrides column on the parent.
- **Departments**: membership derives from staff records; the display order organizing the staff view is a code constant.

## Validation

The loader hard-fails — no fallbacks, server refuses to start — on:

- a missing or duplicated expected header in any tab
- a duplicate key within a tab
- a Name to Email entry matching zero or multiple import rows
- an unflagged Overrides row matching no person (orphaned override)
- a flagged Overrides row colliding with an imported person
- conflicting values for the same adult across import rows
- an invalid canonical value from any layer

The import procedure is manual today: `tools/writetab` writes the Veracross CSV export into the Veracross Import tab (header-checked), and `tools/loadcheck` re-runs the full pipeline against the sheet and prints a model summary. A single import tool that also reports the local layer's health beyond the fatal checks — useless overrides (value identical to what the record has anyway), `-` on flagged rows, name mappings or additions that Veracross has since made redundant, and media files whose name matches no current person or family, including family blobs orphaned by a membership change — is planned (`docs/plan.md`). `tools/findsheet` lists the spreadsheets visible to the service account; `tools/sheets` dumps a sheet's tabs, headers, and rows.

## Sourcing

Records are imported from the school's systems and enriched by families themselves (photos, facts, pronunciation recordings, address preferences), with freshness read from media file timestamps and the change log so refresh cadence can be enforced.
