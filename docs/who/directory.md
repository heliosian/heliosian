# Directory app

The directory ("Helios Who?") is the community's who's-who: students, parents, and staff, browsable by person, family, classroom, and grade. It exists to help people connect — put a face to a name after a conversation at pickup, find a classmate's parents to plan a party, get someone's name pronunciation right. Photos and facts are collected from families each year and are updatable at any time.

## Entities

- **Person** — first and last name; role (student, parent, staff); optional pronouns; optional nickname and pronunciation (an audio recording); photo (some people use an illustrated avatar instead); email; role-specific fields:
  - *Students*: grade, classroom, and crew (displayed as a chain, e.g. grade ▶ classroom ▶ crew), optional free-text "about me" written by or about the kid.
  - *Parents*: their kids (shown as context wherever the parent appears), optional room-parent assignments.
  - *Staff*: job title, displayed prominently; staff may have no family record.
- **Family** — the join between adults and kids: combined surname(s) - each member's surname once, the kids' first, joined with "&", less any that is one part of another member's hyphenated surname, so Mager, Ridgeway and Mager-Ridgeway make "Mager-Ridgeway"; the model carries it bare as `shortName` and with " Family" after it as `name`, both from `familyNameFor`, and nothing else builds or trims one - family photo with a caption identifying everyone in it, an optional family-name pronunciation recording, member list split into adults and kids, address, phone. Lists show the city; the full address powers map actions. Families choose how much address to share (full postal address or just the city).
- **Classroom** — name and mascot artwork, the grade band it serves, and its students, staff, and parents. Classrooms nest crews that student rows reference.
- **Grade** — K through 8, grouped into bands (K, 1st/2nd, 3rd/4th, ...) for browsing.

## Navigation

On wide screens a persistent sidebar carries the app identity and the section list, with the shared toolbar (`docs/toolbar.md`) - search, and the signed-in user - across the top of the content. On phones — the primary way the community uses the app — the sidebar gives way to the shared toolbar on top (hamburger, search, badges, avatar, app switch), a slim dark-teal strip under it, and a bottom tab bar holding the six everyday sections (People, Classrooms, My Family, Staff, Map, Email List), with the rest behind the hamburger. The strip shows the page title while browsing and becomes a back arrow plus the record's name on detail pages. Tab strips collapse to the first tabs plus "More ▾", people grids drop to two columns and gain a per-card overflow menu, detail pages stack their blocks full-width, and family-band member rows pick up photo thumbnails. Same structure throughout — only the chrome changes.

The model loads once, with the page; every link inside the app is followed in place by the shared router (`web/common/router.js`), which redraws the page from the loaded model, so moving between pages fetches nothing. Following a link starts the new page fresh - search, filters, open tabs and edit mode back at their defaults, as a reload would leave them - and Back returns to the page before, fresh the same way, at the scroll position it was left at. Breadcrumbs follow where the person came from: each history entry carries the last few pages before it (`trail()` in the router), never the URL, so a person reached from a grade reads Gradebands / Grade 3 / their name, and a link back to a page on that trail picks up that page's own trail again.

Sections:

### People (home)

Four tabs, each with search and filter:

- **Everyone** — grid of circular photos. Each card: role label with pronouns (e.g. "PARENT (SHE/HER)"), name, and a context line — kids' names for parents, grade/classroom chain for students, job title for staff.
- **Students** — larger cards, first name prominent over last name, grade/classroom chain, pronouns badge.
- **Families** — family-photo cards with grade badges, surname combination, and kids' first names.
- **Staff** — grouped into sections (admin and office staff, teaching staff, ...), title over name.

### Person detail

Breadcrumb back to the list, tag control, photo, role label with pronouns, name with nickname/pronunciation line, grade/classroom chain for students, email and address rows with quick actions (message, mail, map). Students add the "about me" paragraph. Below, a contrasting family band: the person's family name, a narrative caption of who's who, kid rows (grade/team, email), adult rows, and a link to the family page. A person in two households gets one band per family, stacked and identical — the page states their families without marking why there are two.

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

A copyable contact table for party planning and outreach: full name, email, role, grade, classroom. Tabs narrow to parents, students, both, or everyone the user has tagged. Filters select grades, classrooms, or one of the user's own tags or lists.

### Lists

The sidebar's Lists section holds the user's own tags and the lists their roles and sign-ups in the other apps give them, in four parts (`listSections` in `web/who/tags.js`):

- Running: their tags (a shared one, which others manage too, with a people icon), then the undated lists they manage and are in - email lists, ongoing activities, room parent lists.
- Coming Up: the dated parties and activities they are in and the events they manage or are going to, soonest first, each with its date on a small line under its name. A thing's day is its own start or, for an undated sub-activity, its nearest dated parent's; once that day (or its end) is past the list drops off. An event counts when it has a Going group: its list is that group, and it shows when they manage the event or are going. A repeating event shows once, at its next date.
- Joined: the undated activities and email lists they are a member of but do not manage.
- Managing: the activities, parties and email lists they manage but are not a member of (not among the list's people), dated or not. Events they manage stay in Coming Up whether they are going or not.

Running, Joined and Managing each sort by name, tags and lists together; one group is one row, whatever mix of tag, activity, email list or event it is. Marks sit in a column at each row's right edge: a mail icon when the group takes email and, in Coming Up alone, a lime star when the viewer runs it - every row of Running and Managing is theirs to run and no row of Joined is, so the star would say nothing there.

Each part folds behind its heading on a click, remembered with the sidebar's other open sections; Managing starts folded, showing only its count.

A list nested under another list of the same part folds into it: the parent's list shows, the child's does not. A managers group - a plain group that manages something besides itself - is never a tag. Each list is a page at `/groups/{slug}`, or `/groups/{group ID}` for a group with no slug, with the same Profiles, Email List and Map views, the Include control, and the CSV as a tag's page, one per group the viewer runs or is in that the rail lists (`sidebar_group`, `docs/datamodel.md`, Policies) - their family, role groups and classrooms among them - including one per party they host or hold a ticket to on Helios Celebrate (its hosts, the viewer among them, its sold tickets' holders and whoever is billed for them, and after them its guests - ticket holders who are `PERSON` rows of `source` guest - each a card with a GUEST label linking to their own person page; guests show on that party's own page alone - never in the Tags filter's combined results, on Invites, on the Map, or in a saved tag - and drop out while a grade, classroom or role filter narrows the list), one per activity they co-chair or volunteer on in HCA-Team (every volunteer and co-chair on it and on everything under it, the viewer among them), one per event on Helios When as above (who is going), one per grade band they are room parent for (the parents of its students), one per email list they manage or are on in Helios Loop (its members as the email list's rules pick them out now, and the people a manager added by hand from outside the directory as its guests, each a GUEST card linking to their person page; `docs/loop/loop.md`), and one per admins group they manage or are in (`docs/datamodel.md`, Groups). The viewer is never on their own list. An email list its manager has archived in Loop keeps its list off the sidebar and the Tags filter for them, though the list's page still opens from the email list's page there. Each app's own rule says who runs a thing, and the lists come and go with those roles: nothing is stored. A list cannot be edited here - Save as tag copies its people into a tag of the user's own to prune and keep - and the sidebar's rows are names alone, while the phone's Lists menu shows each list's app mark beside its name (Celebrate, HCA-Team, When or Loop) and the Tags filter a party, helping-hands, calendar or people icon in the Tags filter, where the lists follow the tags.

## Behaviors

- Everything is cross-linked: parents ↔ kids ↔ families ↔ classrooms; any person reference navigates to that person. A person's page is `/people/{slug}` (`PERSON.slug`, the part of their primary address before the @; `docs/datamodel.md`, Veracross import), or `/people/{id}` for a guest, who has no slug; a family's is `/families/{id}`, a classroom's `/classrooms/{slug}` and a grade's `/grades/{slug}`.
- The old tag and list addresses, `/people?tag={key}` and `/people?list={key}`, are permanently redirected by Who?'s server to `/groups/{key}`, a list key losing everything up to its colon (`party:`, `activity:`, `group:`, `room:`), and land on the lookup below (`people` in `internal/who/who.go`).
- Old links keep working. A page whose name matches nothing is looked up in `ALIAS` once the page has loaded, and the address is replaced with its target's current one (`oldName` and `oldTarget` in `web/who/state.js`): an old person address or renamed slug, an old family or group ID, and an old guest page - `/people/guest:<ticket ID>`, whose alias names the ticket's `MEMBER` row and so its person, or `/people/guest:<old list ID>:<address>`, the person holding that address.
- Search is per-list and immediate; filters cover role, class, grade, city and pronouns. The grade and classroom filters read a student's own, a parent's children's, and none for staff - a teacher is not in the room they teach. The client's `personFacets` (`web/who/filters.js`) and the server's `model.Directory.Facets` (`internal/model/directory.go`, which Loop's rules and Heliosian's audiences read through) must read alike; a change to one is a change to the other.
- Tags file people under named groups — a person can be in several at once (soccer team, class party, carpool) — and feed the email list's My Tags tab and tag filter. A tag is a listed `GROUP` of kind `group` managed by a "<name> Managers" group that manages itself; making one writes both groups, the maker as the managers group's first member and the first person tagged, in one batch (`web/who/tags.js`). The viewer's tags are the groups they manage that are managed by a group of their own, leaving out another tag's managers group. A tag has no owner: every manager may tag and untag, rename, share it (add someone to its managers group) and close it, and a manager leaves by leaving that group. Deleting a tag, or untagging its last person, sets `status` to `closed` on the tag and its managers group. A tag's page is `/groups/{group ID}`, so renaming it moves nothing. A member's tags are private to them and their co-managers, until one names it in a rule, which discloses who it holds to whoever sees that thing (`docs/loop/loop.md`, A rule discloses the tags it names). Lists (above) sit beside them wherever tags are offered, except in a card's tag menu, since they cannot be changed.
- Photos lazy-load; full-size view on click where the photo is the subject (family pages).
- All data is community-only, behind sign-in; opt-out removes a person on request.
- Self-service: viewing your own record, your household's, or your family page shows inline edit affordances — photo upload, crop and order, pronunciation recording or upload, About Me text, preferred name, phone, address. A photo or recording goes up through `/api/do/photo` or `/api/do/pronunciation`, and every other edit is a `POST /api/q` batch on the person's or family's rows (`docs/datamodel.md`, The query API), each change kept in the sheet's `CHANGES` tab. A preferred name writes `name_short`, `name_long` and `name_sort`'s overrides together. A photo's order is its `PHOTO.order` key, a crop is a box on its re-encode, and deleting one deletes its row.
- Who may edit what is the policies' (`internal/db/policy.go`; `docs/datamodel.md`, Permissions): a family manages itself, so every member of a family, students included, may edit the family and everyone in it. A kid of two households is in both families, so either household's members may edit the kid, and neither the other household's page nor its parents. The page shows edit controls to an admin and on the viewer's own household (`canEdit` and `canEditFamily` in `web/who/state.js`), and the to-do list on the viewer's own family page alone.
- Admins: an admin of Who? may edit every person and family, always, with no mode (`docs/toolbar.md`, Admins), each edit written to `CHANGES` under the admin's name, and keeps the settings, colors and classroom and grade images in Admin Tools. Nobody adds a person, their addresses or role groups by hand, or deactivates one, through Who?: people come from `tools/import`. Every check goes by the person the request runs as, so a super admin viewing as a parent in Spoof Mode edits only what that parent could. Admin Tools is in an admin's account menu.
- Refresh reminders: every page carries a band listing content that has aged past its window, the thresholds being the data model's platform `SETTING` rows (`Photo Stale Years`, `Facts Stale Years`). Only students are prompted about a photo or About Me: the signed-in user's kids, and the user themselves when they are a student, for one that is missing, past its window, or whose updated date is unknown (`web/who/stale.js`). Adults and staff are never nudged about their own record, however old it is. A family photo carries no date, so the family is prompted only when it has none, and only to someone who may edit the family. Each row links to that record's edit view, and an Update Family Info action leads to the family page.

## Invite templates

The Invite List Builder's templates are rows of the data model, not code: `INVITE_SERVICE` lists the destination systems (Greenvelope, Evite, ...), and `INVITE_TEMPLATE` each system's export columns, in `order`, each a heading and a template of `{{ parameter }}` tokens the page fills in per family or per person. A heading may repeat within a system, since some systems' own import formats repeat one. Adding a system, or matching a system's own quirk, is a row edit rather than a deploy.

`GREETING` holds the ways the page can address a family or a person: the site's own, with no `added_by`, and the ones people add for themselves. A greeting's `format` is a sample written over the made-up Ender family, and the page swaps each sample phrase it finds for the real names - the whole family, the kids, the adults, a person's full name, a person's first name. Those phrases are fixed in `web/who/pages/invites.js` (`GREETING_WHOLE_FAMILY` and the rest), and the site's own greeting whose format is `GREETING_DEFAULT` is the one the page starts on, so a site greeting's format is written with those phrases in mind.
