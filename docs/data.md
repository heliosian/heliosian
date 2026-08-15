# Data model

The entities the directory serves. Structured data lives in the data source (see `docs/plan.md`); blobs (photos, audio) are URL references into blob storage.

## Person

One record per person, all roles in one shape:

- **Key**: school email, lowercased. Everyone has one, including the youngest students (their addresses exist but they don't have access yet).
- **Names**: full name, legal name, preferred name.
- **Roles**: student, parent, and staff booleans — combinations are valid (staff members are often also parents). Display strings derive from the flags.
- **Pronouns**: optional; a curated list plus a freeform escape hatch.
- **Pronunciation**: optional audio recording of the person's name.
- **Photo**: official portrait or personal upload; people may opt for an illustrated avatar instead.
- **Facts**: optional about-me text — first-person blurbs for students, professional bios for staff.
- **Student fields**: grade (`Kindergarten`, `Grade 1` … `Grade 8`), classroom section, and the parent contact emails for the student.
- **Staff fields**: job title, department, grade band.
- **Contact**: email always; phone optional.
- **Year rollover**: next-year grade and band, so the directory can flip to the new school year.

## Family

A family groups adults and kids:

- **Key**: shared by all members. (Currently a parent's email; minting stable family IDs is a planned migration.)
- **Photo and caption**: the family photo plus a who's-who description naming everyone in it.
- **Pronunciation**: optional audio recording of the family name.
- **Address**: as much as the family chooses to share — full postal address or just city and state.
- **Phone**: optional family phone.

## Classrooms and grades

- **Classroom**: name, mascot artwork, and the grade band it serves. Some classrooms subdivide into sections (teams); sections have their own logos.
- **Section**: classroom subdivision with up to a few teachers, a sort order, and named schedule blocks.
- **Grade band**: pairs of grades share a band with a combined identity — `Hummingbirds` (K), `Halcons` (1st/2nd), `Jayvens` (3rd/4th), `Cospreys` (5th/6th), `Hegrets` (7th/8th) — used for browsing, room-parent organization, and band-colored styling (see `docs/design.md`). A grade → next-grade mapping drives year rollover.
- **Room parents**: parent assignments per grade band.
- **Departments**: ordered list organizing the staff view into sections.

## Sourcing

Records are imported from the school's systems and enriched by families themselves (photos, facts, pronunciation recordings, address preferences), with each contributed item carrying a last-updated stamp so refresh cadence can be enforced.
