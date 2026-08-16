# Directory app

The directory ("Helios Who?") is the community's who's-who: students, parents, and staff, browsable by person, family, classroom, and grade. It exists to help people connect — put a face to a name after a conversation at pickup, find a classmate's parents to plan a party, get someone's name pronunciation right. Photos and facts are collected from families each year and are updatable at any time.

## Entities

- **Person** — first and last name; role (student, parent, staff); optional pronouns; optional nickname and pronunciation (an audio recording); photo (some people use an illustrated avatar instead); email; role-specific fields:
  - *Students*: grade, classroom and team assignment (displayed as a chain, e.g. grade ▶ team ▶ subteam), optional free-text "about me" written by or about the kid.
  - *Parents*: their kids (shown as context wherever the parent appears), optional room-parent assignments.
  - *Staff*: job title, displayed prominently; staff may have no family record.
- **Family** — the join between adults and kids: combined surname(s), family photo with a caption identifying everyone in it, an optional family-name pronunciation recording, member list split into adults and kids, address, phone. Lists show the city; the full address powers map actions. Families choose how much address to share (full postal address or just the city).
- **Classroom** — name and mascot artwork, the grade band it serves, and its students, staff, and parents. Classrooms nest teams/subteams that student rows reference.
- **Grade** — K through 8, grouped into bands (K, 1st/2nd, 3rd/4th, ...) for browsing.

## Navigation

On wide screens a persistent sidebar carries the app identity, the section list, and the signed-in user. On phones — the primary way the community uses the app — the sidebar gives way to a dark-teal top bar and a bottom tab bar holding the six everyday sections (People, Classrooms, My Family, Staff, Map, Email List), with the rest behind a hamburger menu. The top bar shows the page title while browsing and becomes a back arrow plus the record's name on detail pages. Tab strips collapse to the first tabs plus "More ▾", people grids drop to two columns and gain a per-card overflow menu, detail pages stack their blocks full-width, and family-band member rows pick up photo thumbnails. Same structure throughout — only the chrome changes.

Sections:

### People (home)

Four tabs, each with search and filter:

- **Everyone** — grid of circular photos. Each card: role label with pronouns (e.g. "PARENT (SHE/HER)"), name, and a context line — kids' names for parents, grade/team chain for students, job title for staff.
- **Students** — larger cards, first name prominent over last name, grade/team chain, pronouns badge.
- **Families** — family-photo cards with grade badges, surname combination, and kids' first names.
- **Staff** — grouped into sections (admin and office staff, teaching staff, ...), title over name.

### Person detail

Breadcrumb back to the list, favorite (heart) toggle, photo, role label with pronouns, name with nickname/pronunciation line, grade/team chain for students, email and address rows with quick actions (message, mail, map). Students add the "about me" paragraph. Below, a contrasting family band: the person's family name, a narrative caption of who's who, kid rows (grade/team, email), adult rows, and a link to the family page.

### Family detail

Family photo with click-to-expand and its identifying caption, grade badges, family name, member first-names, city with message/map actions, and a members section split into adults and kids, each row linking to the person.

### Classrooms

Three tabs: browse classrooms by grade band (mascot art, student count, link to detail), the same grouped by classroom, and room parents (parent rows annotated with each of their kids' classroom and grade). Classroom detail shows the mascot, name, and tabbed member lists — students (grouped by team, with parents' names above each student and the about-me blurb inline), staff, and parents — with per-tab counts.

### My Family

Goes straight to the signed-in user's own family page.

### Staff

The staff list as a top-level section — same content as the People staff tab.

### Map

A Google map of family locations: one brand-teal pin per geocoded family address, a popup card (family photo, name, address, family-page link) on pin click, and search and filters narrowing the pins. Below the map, an "update my address" self-service action.

### Email List

A copyable contact table for party planning and outreach: full name, email, role, grade, classroom. Tabs narrow to parents, students, both, or the user's bookmarked people. Filters select grades or classrooms.

### Data View

A raw tabular view over the underlying records, for power users.

### Share & About

Share the app by SMS or link, an explanation of why photos and facts are collected, a bug-report pointer, and an opt-out form for removing a person's information.

## Behaviors

- Everything is cross-linked: parents ↔ kids ↔ families ↔ classrooms; any person reference navigates to that person.
- Search is per-list and immediate; filters cover role, class, grade, city, pronouns, and new-to-Helios.
- Favorites/bookmarks mark people and feed the email list's bookmark tab.
- Photos lazy-load; full-size view on click where the photo is the subject (family pages).
- All data is community-only, behind sign-in; opt-out removes a person on request.
- Self-service media: viewing your own record, your kids', or your family page shows inline edit icons — a camera on the photo for uploads, and microphone/file icons under the pronunciation player to record in the browser or upload audio. Replaced files move to an `archive` folder in the media drive with a timestamp, the sheet's media cell is updated, and every change appends to the sheet's `Change Log` tab (timestamp, actor, target, kind, file, archived file).
