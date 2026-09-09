# Home app

HCA Home is the community's front door: one page of links, grouped into categories, to everything the Helios Community Association and the school run elsewhere. It replaces the Glide app of the same name and serves at heliosian.com, www.heliosian.com, and home.heliosian.com.

## Entities

- **Category** — a title and an optional image. Categories display in the order their rows sit in the sheet, which is also how they are reordered.
- **Link** — title, optional description, URL, optional image, the category it belongs to, a Visible flag, and who added it when. Links display in sheet row order within their category. A link with no image of its own shows its category's image; with neither, a tile of its initial.

Hidden links (Visible = No) are kept but shown only to admins, greyed and badged, so a seasonal link can be parked rather than deleted.

## The page

A single page. The olive top bar carries the sunflower mark, the app name, and the signed-in user's avatar, whose menu holds the email, Admin Tools for admins, and Sign Out. Below, "HCA Quick Links" heads the categories, each a white panel of cards.

Each card: the image, the link's title as a small-caps label, the URL shorn of its scheme as the bold line, the description in muted text, and the actions — Go! opens the link in a new tab, Copy Link copies its URL, and the "…" menu repeats both. On phones the two buttons fold into the menu.

## Editing

Editing behaves as the directory's does: in place, for admins, with no separate editing pages. Admins see Add Link and Add Category above the list, a pencil beside every category title, and Edit in every card's menu. Each opens a modal with the entity's fields, an image picker, and Delete. An image is uploaded the moment it is chosen and named by its content; the save that follows records the name. A category cannot be deleted while links remain in it. Renaming a category carries its links along.

Every change is applied to the in-memory model first, then written to the sheet cell by cell through the shared write queue, and appended to the Change Log tab. A change the sheet rules reject (a duplicate title, a bad URL, an unknown category) is refused before anything is written.

Who may edit is the Admins tab of the Apps sheet plus the platform super admins (`docs/config.md`). Admins manage the tab from `/admin`, a page holding only that list.

## Sign-in

Everything behind Google sign-in restricted to the school domain, like every app here; there is no public view. The Locked-to-Helios-login flag the Glide app needed does not exist, because every viewer is signed in. The login page is HCA Home's own splash. A session is shared with the directory on the same tier, since the cookie is scoped one label up from the app's hostname.

## Installable

Ships as an installable web app like the directory (`docs/who/pwa.md` describes the mechanics): its own manifest, icons, and iOS splash battery under `web/public/home/`, all taken from the Glide app so a home-screen icon looks the same as before.
