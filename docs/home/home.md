# Home app

Heliosian is the community's front door: one page of links, grouped into categories, to everything the Helios Community Association and the school run elsewhere. It replaces the HCA Home Glide app and serves at heliosian.com, www.heliosian.com, and home.heliosian.com.

## Entities

- **Category** — a title, an optional emoji, and a style. Categories display in the order their rows sit in the sheet, which is also how they are reordered.
- **Link** — title, optional description, URL, optional image, the category it belongs to, a Visible flag, and who added it when. Links display in sheet row order within their category. A link with no image of its own shows its category's emoji; with neither, its initial.

Hidden links (Visible = No) are kept but reach only admins, who see them greyed and badged while **Super Admin Mode** in the account menu is on (off by default, remembered per browser; the same switch brings out the add cards), so a seasonal link can be parked rather than deleted and an admin still sees the page as everyone else does.

## The page

A single page. A teal sidebar, dark at the top shading to medium at the foot, carries the white lockup, a link to Home and to each category (its emoji, or without one a school building, a calendar or a chat bubble by the title's wording, else a grid), and the campsite illustration at its foot. The shared toolbar (`docs/toolbar.md`) runs across the top of the content with the search box - it filters the links and the upcoming events as you type, and `/` jumps to it - and the signed-in user's avatar, whose menu holds the email, Edit Categories, Super Admin Mode and Admin Tools for admins, and Sign Out.

The hero is a welcome - "Tools and resources for the Helios Community", with a kicker and a line under it - over the left of a landscape that runs along the hero's bottom edge and ends in a pale wave. Then the categories, one of which is the events section - HCA-Team's next six open, dated events (`Model.Upcoming` in `internal/events/upcoming.go`, handed to the page in its model), which sits among the others and is renamed, re-marked and moved like them (`docs/home/data.md`), though it takes no links and cannot be deleted. Its cards each carry the event's picture or a calendar glyph, a month-and-day stamp, the title, the day and time beside a calendar, and one row of buttons - a calendar-with-plus icon that adds it to Google Calendar (an entry built from the sheet's start, end, location and description: all-day for a bare day, two hours long for a timed start with no end, in the school's time zone), and **Volunteer**, the event's page, where the sign-up is; "See all in HCA-Team" beside its heading leads to the portal itself. With nothing ahead the section stays off the page (it says so in Super Admin Mode). A section with nothing to show stays off the page - a category whose links are all hidden, or the events section with nothing ahead - except in Super Admin Mode, where it appears empty so it can be filled or edited. A section whose Max is set shows that many and a "See more (n)" button for the rest. The link categories, each headed by its title in the Montserrat-over-swoosh treatment Who? and HCA-Team use, come in two styles:

- **cards** — tinted feature cards three to a row: the link's picture (or the category's emoji) in a disc, the title, the description, an Open App button in the card's accent, the picture again large and faint behind the right edge when there is one, and a round chevron in the corner that opens the link too.
- **tiles** — white cards three to a row: the picture (or, without one, the category's emoji or glyph on a pale ground) beside the title and the description on one line.

A card opens its link; in Super Admin Mode it wears a pencil in its corner, the way into its editor. The foot of the page is the leaves along the bottom edge. On phones the sidebar becomes a header strip (the mark on its tile, the section links), the illustrations at its foot drop, and the grids go to one column.

The art lives in `web/home/brand/`: `hero-art.png` (the landscape strip, transparent above and to the left), `footer-leaves.png` (the wave and leaves), and `sidebar-camp.png` (the campsite, transparent).

## Editing

Editing behaves as the directory's does: in place, for admins, with no separate editing pages. With Super Admin Mode on, admins see a pencil beside every category title and on every card, and a dashed add card at the end of every category ("Add Event" under Events, "Add Chat" under Chats, "Add Link" where the title does not read that way) that opens the link editor with the category chosen. The link editor takes an image by upload or by Find an image, the same picture search HCA-Team has (`docs/events/events.md`); the category editor takes an emoji from a grid of school-flavoured picks in groups, from the whole library behind "More emoji…" (every single-character emoji with its Unicode name to search by, generated into `web/home/emoji.json`), or any pasted in. Each opens a modal with the entity's fields, an image picker, and Delete. An image is uploaded the moment it is chosen and named by its content; the save that follows records the name. A category cannot be deleted while links remain in it. Renaming a category carries its links along.

Every change is applied to the in-memory model first, then written to the sheet cell by cell through the shared write queue, and appended to the Change Log tab. A change the sheet rules reject (a duplicate title, a bad URL, an unknown category) is refused before anything is written.

Who may edit is the Admins tab of the Apps sheet plus the platform super admins (`docs/config.md`). Admins manage the tab from `/admin` - Admin Tools in the shared admin chrome (`docs/toolbar.md`), with a Categories overview and the admin list.

## Sign-in

Everything behind Google sign-in restricted to the school domain, like every app here; there is no public view. The Locked-to-Helios-login flag the Glide app needed does not exist, because every viewer is signed in. The login page is Heliosian's own splash. A session is shared with the directory on the same tier, since the cookie is scoped one label up from the app's hostname.

## Installable

Ships as an installable web app like the directory (`docs/who/pwa.md` describes the mechanics): its own manifest, icons, and iOS splash battery under `web/public/home/`, all taken from the Glide app so a home-screen icon looks the same as before.
