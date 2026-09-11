# Volunteer portal

HCA-Team, the HCA Volunteer Portal, is where the Helios Community Association asks for help and where families answer: every event, committee, and idea the association runs each school year, the roles under each, and who signed up. It replaces the Glide app of the same name and serves at hca.heliosian.com. The sheet, package, and API are named for what they hold, `Events`; only the hostname and the branding carry the association's name.

## Entities

- **Category** — a heading on the Sign Up page ("Headline Events", "HCA Committees", "Just an Idea"), with a blurb. Categories display in the order their rows sit in the sheet.
- **Activity** — one thing to help with in one school year: title, description, image, when it happens (a start and end, or a free-text timing like "All Year"), where, how many spots, and its status. Activities form a tree: a committee, task, booth, or shift is an activity whose parent is the event it belongs to, and it can hold activities of its own — a booth with its own performance slot. The roots, the ones with no parent, carry one of the page's categories and are what the opportunities page lists; a child lists under its parent, grouped by whichever of the root event's own categories it names. Any activity either takes sign-ups directly or only through the things under it (Direct Sign-Up).
- **Volunteer** — a person on an activity at any depth, as a volunteer, a volunteer open to co-chairing, or a co-chair, with an optional note. Who has offered to co-chair is public: they follow the chairs in the Volunteers list tagged CHAIR OPT and appear under Co-Chair Options in the rail, where whoever runs the event can promote them; an Offer to Co-Chair button under "A co-chair is needed" makes the offer in one click.
- **Link** — a sign-up sheet, chat group, or document attached to an activity, shown in its Resources card.

Activities repeat year to year by being copied: "Spring Celebration" in one year and the next are two activities, each carrying its own tree and sign-ups. Running an activity means running everything under it: a co-chair of the event edits its shifts too.

## Status

An activity or role is `Pending`, `Open`, `Done`, or `Hidden`. Pending is a suggestion waiting for an admin; open takes sign-ups; done is over; hidden is parked. Whether something is full is not a status: a spots count sets the capacity, and the sign-up count against it says the rest.

## Who may do what

- **Anyone signed in** browses everything open, signs themselves or someone else up, removes themselves, suggests an activity or a role (which lands pending), and sees the co-chairs of a private list plus their own entry on it.
- **Editors of an activity**, its admins and co-chairs, change it and its roles and links, set statuses short of approving, remove anyone, and open the roster. Over their event they count as admins: making someone a co-chair - or unmaking one - is their appointment, a Co-Chair choice in the sign-up editor's As list that only they see; nobody picks it for themselves, and a sitting co-chair keeps the position while editing their own note. Clicking a volunteer's face opens them a two-tab window - the contact card, and that sign-up - where everyone else gets the card alone. The server sends them a secret volunteer list in full, but the page shows it to them as everyone else sees it - nobody - until they edit or turn on Show Hidden Things, so what an organizer looks at is what people get; the Join and sign-up buttons stay. With direct sign-up off, the section keeps its heading and says to pick something below, and the buttons go.
- **Admins**, the Admins tab plus the platform super admins (`docs/config.md`), approve suggestions, hide and delete, copy an activity into the next year, and manage categories, settings, and the admin list from `/admin`. Their hat is off by default: the page treats a system admin as any other parent (plus whatever they co-chair) until they turn on **Super Edit Mode** in the account menu, which brings every admin control back - the pencils, Approval Needed, Show Hidden Things - and is remembered per browser; Admin Tools stays in the menu either way. The server keeps enforcing by the admin list either way; the switch is about what the page shows and offers.

## Pages

One shell, `web/hca/index.html`, routes every page on the client: a left toolbar (Opportunities with each category and its count, My Sign Ups, Calendar, Approval Needed for admins, and Suggest an Idea) beside a top search bar that filters whatever page is open.

- **Opportunities** (`/`) — cards for this school year's root activities, grouped under their categories in sheet order, with a category chip row that filters the grid and an academic-year dropdown. Undated activities follow dated ones. Hidden and pending activities stay off the grid unless an admin turns on Show Hidden Things in the account menu; Show Completed Events brings back what is done or past. Another year is `/years/{year}`.
- **My Sign Ups** (`/my`) — the viewer's own sign-ups at any depth, by year and by the root's category.
- **Calendar** (`/calendar`) — a month grid of every dated activity, root or child.
- **Approval Needed** (`/approvals`) — admins: every pending activity at any depth, shown as Root ▶ Child.
- **Activity** (`/activities/{id}`, or `/v/{pretty}` when it has a friendly address; things under an event sit under its path, `/v/inight/poland`) — one page for every node. A hero image with the date stamp, then the description beside a rail of when, who runs it, the flyer when there is one (click for full size), resources, and who to ask. A child's chip is a link up to its parent; a root's is its category. Opportunities & Needs lists what sits under it grouped by the event's own categories — each with an Add button for whoever runs the event, and a Suggest button for everyone else where the category allows adding; hidden and pending rows join the list, muted, only for an editor who is editing or has Show Hidden Things on. While an activity's page is open the toolbar carries its event too: the event itself, then its committees grouped by the event's categories, every group and every committee with things under it closed until opened (the counts say what is inside), nested to any depth, each with its sign-up count or "2 of 5" against a set number of spots. Anyone can open the hero image at full size; editors click the hero's pencil to reveal inline editing of the title, description, category, image (upload, or find one on Wikimedia Commons - Google Images when a key is set up - through a server-side search that fetches and stores the picked picture like an upload; crop with the freeform tool ported from Helios Who?; remove), and dates, an Edit Categories button that manages the event's own categories, drag-and-drop of rows between categories, and the status under the pencil.
- **Admin Tools** (`/admin`) — the page's category headings (with images, reorderable), settings, and admin list.

On phones the toolbar becomes a drawer, the search bar an overlay, and a bottom tab bar holds Opportunities, My Sign Ups, and Calendar.

## The school year

School years turn over on July 1 and are written as both calendar years, `2026 - 2027`. The current, last, and next years are computed from the date; nothing records them.

## Names and photos

The portal stores only email addresses. Names and photos come from the directory model at request time, so a volunteer looks the same here as in Helios Who?. Someone the directory does not list, such as a parent who has not opted in, is shown under a name read out of their address. Sign-in resolves through the directory's email aliases too, so a person is one address across every app.

## Editing

As in the other apps: in place, through modals, with every change applied to the in-memory model first and then written to the sheet cell by cell through the shared write queue and appended to the Change Log. A change the rules reject is refused before anything is written. Deleting an activity or role refuses while anyone is signed up for it, and deleting a category refuses while an activity names it.

## Sign-in and install

A link to an event previews in chat apps and social feeds even though the site is behind sign-in: the sign-in page served at an event's address carries the event's Open Graph tags (title, the date line and a sentence of the description, the canonical address), and `/share/{id}.png` is a public 1200x630 card drawn on the server - the mark and wordmark, the title with the yellow swoosh, when it is (large), the address, and the event's flyer shown whole on a tinted panel - or, without one, its banner filling the right side, or the rail's meadow when it has neither. A thing under an event is labelled with what it sits under - "INTERNATIONAL NIGHT" above "India", and "India · International Night" in the tags - and takes the nearest date, timing and image above it when it has none of its own. Only what is open or done previews; a hidden or pending thing, or any other page, gets the plain sign-in page and no card. The tags say what a poster on the wall would say and never who signed up; anyone holding the link sees them, so an event's title, description and image are public in that sense. Montserrat is bundled as TTF in `web/hca/fonts/` for the card (`internal/events/share.go`).

Everything sits behind Google sign-in restricted to the school domain, sharing a session with the other apps on the same tier. It installs to a home screen like the directory (`docs/who/pwa.md`), with the icons and splash screens the Glide app used under `web/public/hca/`.
