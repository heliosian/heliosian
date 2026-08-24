# Directory app

The directory ("Helios Who?") is the community's who's-who: students, parents, and staff, browsable by person, family, classroom, and grade. It exists to help people connect — put a face to a name after a conversation at pickup, find a classmate's parents to plan a party, get someone's name pronunciation right. Photos and facts are collected from families each year and are updatable at any time.

## Entities

- **Person** — first and last name; role (student, parent, staff); optional pronouns; optional nickname and pronunciation (an audio recording); photo (some people use an illustrated avatar instead); email; role-specific fields:
  - *Students*: grade, classroom, and crew (displayed as a chain, e.g. grade ▶ classroom ▶ crew), optional free-text "about me" written by or about the kid.
  - *Parents*: their kids (shown as context wherever the parent appears), optional room-parent assignments.
  - *Staff*: job title, displayed prominently; staff may have no family record.
- **Family** — the join between adults and kids: combined surname(s), family photo with a caption identifying everyone in it, an optional family-name pronunciation recording, member list split into adults and kids, address, phone. Lists show the city; the full address powers map actions. Families choose how much address to share (full postal address or just the city).
- **Classroom** — name and mascot artwork, the grade band it serves, and its students, staff, and parents. Classrooms nest crews that student rows reference.
- **Grade** — K through 8, grouped into bands (K, 1st/2nd, 3rd/4th, ...) for browsing.

## Navigation

On wide screens a persistent sidebar carries the app identity, the section list, and the signed-in user. On phones — the primary way the community uses the app — the sidebar gives way to a dark-teal top bar and a bottom tab bar holding the six everyday sections (People, Classrooms, My Family, Staff, Map, Email List), with the rest behind a hamburger menu. The top bar shows the page title while browsing and becomes a back arrow plus the record's name on detail pages. Tab strips collapse to the first tabs plus "More ▾", people grids drop to two columns and gain a per-card overflow menu, detail pages stack their blocks full-width, and family-band member rows pick up photo thumbnails. Same structure throughout — only the chrome changes.

Sections:

### People (home)

Four tabs, each with search and filter:

- **Everyone** — grid of circular photos. Each card: role label with pronouns (e.g. "PARENT (SHE/HER)"), name, and a context line — kids' names for parents, grade/classroom chain for students, job title for staff.
- **Students** — larger cards, first name prominent over last name, grade/classroom chain, pronouns badge.
- **Families** — family-photo cards with grade badges, surname combination, and kids' first names.
- **Staff** — grouped into sections (admin and office staff, teaching staff, ...), title over name.

### Person detail

Breadcrumb back to the list, tag control, photo, role label with pronouns, name with nickname/pronunciation line, grade/classroom chain for students, email and address rows with quick actions (message, mail, map). Students add the "about me" paragraph. Below, a contrasting family band: the person's family name, a narrative caption of who's who, kid rows (grade/team, email), adult rows, and a link to the family page.

### Family detail

Family photo with click-to-expand and its identifying caption, grade badges, family name, member first-names, city with message/map actions, and a members section split into adults and kids, each row linking to the person.

### Classrooms

Three tabs: browse classrooms by grade band (mascot art, student count, link to detail), the same grouped by classroom, and room parents (parent rows annotated with each of their kids' classroom and grade). Classroom detail shows the mascot, name, and tabbed member lists — students (grouped by crew, with parents' names above each student and the about-me blurb inline), staff, and parents — with per-tab counts.

### My Family

Goes straight to the signed-in user's own family page.

### Staff

The staff list as a top-level section — same content as the People staff tab.

### Map

A Google map of family locations: one brand-teal pin per geocoded family address, a popup card (family photo, name, address, family-page link) on pin click, and search and filters narrowing the pins. Below the map, an "update my address" self-service action.

### Email List

A copyable contact table for party planning and outreach: full name, email, role, grade, classroom. Tabs narrow to parents, students, both, or everyone the user has tagged. Filters select grades, classrooms, or one of the user's own tags.

## Behaviors

- Everything is cross-linked: parents ↔ kids ↔ families ↔ classrooms; any person reference navigates to that person.
- Search is per-list and immediate; filters cover role, class, grade, city, pronouns, and new-to-Helios.
- Tags file people under named groups — a person can be in several at once (soccer team, class party, carpool) — and feed the email list's My Tags tab and tag filter. A member's tags are private to them.
- Photos lazy-load; full-size view on click where the photo is the subject (family pages).
- All data is community-only, behind sign-in; opt-out removes a person on request.
- Self-service: viewing your own record, your kids', or your family page shows inline edit affordances — photo upload, pronunciation recording or upload, About Me text, preferred name, phone, address — plus opt-out for yourself or your kids. Media uploads overwrite the bucket object, which retains the previous version; sheet-backed edits write the Overrides tab and append a Change Log row (see `docs/data.md`). A facts edit or photo upload also stamps that item's updated date.
- Refresh reminders: every page carries a band listing content that has aged past its window — a photo past 0.75 years, an About Me past 0.6, a family photo past 1.5 — the cadence the previous app used. Only students are prompted about a photo or About Me: the signed-in user's kids, and the user themselves when they are a student. Adults and staff are never nudged about their own record, however old it is; the family photo is prompted to whoever can edit the family. Each row links to that record's edit view, and an Update Family Info action leads to the family page. Only aged content is prompted: a record with no photo or no About Me raises nothing, and an item whose updated date is unknown counts as aged.
