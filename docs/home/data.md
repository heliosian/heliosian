# Data model

The tabs, columns, and validation rules are in `internal/home`; this file carries only what reading that code cannot tell you.

Home's data lives in one Google Sheet, `Apps`, in the community shared drive, reached through drive membership like the directory's sheets. `APPS_SHEET` names it, and `cmd/createtabs` reads that variable to lay it out from an empty spreadsheet titled `Apps`, alongside the directory and config sheets it lays out in the same run.

## Tabs

- `Categories` — Title, Emoji, Style, Max. Row order is display order.
- `Links` — Title, Description, URL, Image, Category, Visible, Added By, Added. Row order is display order within a category.
- `Admins` — Email. Who may edit, beyond the platform super admins (`docs/config.md`).
- `Visibility` — App, Visibility, Emails. At most one row per community app (`who`, `team`, `celebrate`): whether everyone sees it or only the people in its Emails cell, in the app switch of every app's toolbar and among the front page's links. Usually empty.
- `Change Log` — Timestamp, Actor, Action, Kind, and the link or category fields. Appended on every change; never read back.

The sheet is the schema from scratch, not an import of the Glide app's tables. The Glide app's links were carried over once, transcribed from captures of its pages, with their images copied into the bucket.

## Rules

**Title is the key.** Links and categories are edited by title, so a title must be unique within its tab. An edit that renames carries the new title in the same write, so the row keeps its place; a category rename rewrites the Category cell of every link in it.

**Visible is Yes or No**, spelled exactly so. Any other value refuses the load.

**Style is `cards`, `tiles` or `events`**, spelled exactly so, and every category needs one. `cards` renders the category as large feature cards, each with its image, description, and its own button; `tiles` renders it as a row of compact tiles with a picture, the title and a line. A blank or misspelled Style refuses the load rather than guessing a presentation, the same stance Visible takes — the front page's shape is a property of the sheet, not of what the renderer happens to fall back to.

**`events` is the one section that holds no links**: HCA-Team's upcoming events, which the portal supplies. The Style value is what marks the row as the app's own - no other column says so, and the row has no id: the sheet is keyed by Title throughout, and at most one row may carry this style, so it is found by that. `cmd/createtabs` seeds the row ("Upcoming Events" 📅) when it makes the tab, and the sample data carries it, so normally the sheet holds it like any other category and it is renamed, re-marked and moved like one. A sheet without the row (one laid out before the section existed) still gets the section, synthesized at the top under that standing name; the app writes the row the first time an admin renames it, changes its emoji or moves it (`saveCategory` and `reorderCategories` in `internal/home/home.go` append it, then order it where the page had it). No link may name it as its category, and it cannot be deleted - only renamed and moved.

**Max caps what a section shows.** Blank shows everything; a whole number of one or more shows that many links (or, for the events section, events) and puts the rest behind a See More button, which opens the section for the rest of the visit. A search shows every match regardless. Anything else in the cell refuses the load. The category editor's "Show at most" field sets it.

**Visibility is `everyone` or `list`**, spelled so, and App is `who`, `team` or `celebrate` (`internal/home.Apps`, the keys `web/common/toolbar.js` lists the switch by; Heliosian itself is not among them, being the front page and the way home); a second row for one app, or any other value in either column, refuses the load - a misspelled app or mode would narrow nothing, silently. An app with no row is everyone's. Emails is the list, separated by commas, semicolons, spaces or line breaks, forgiven case and duplicates, read only while Visibility is `list` but kept either way, so the list drawn up for an app is still there after it has been everyone's for a while. `list` with an empty cell shows the app to nobody. This is visibility, never access: a direct link, a bookmark or a shared event page still opens the app to anyone signed in. The admin page's **App Visibility** panel edits the tab a card per app - a switch between the two modes, and under the list mode the people, picked from the directory - each change upserting the app's row (`/api/admin/visibility`); a long list can be pasted into the cell by hand, live within the five-minute reload.

**A category must exist before a link names it**, and cannot be deleted while a link still does.

**A category goes by an emoji, not a picture.** Its Emoji cell is blank or one emoji (a short run of symbol runes, joiners and variation selectors - a flag or a skin-toned face passes, a word refuses the load). The editor offers a grid of them and takes any other one pasted in. The emoji marks the rail and stands in for a link with no image of its own; without one, an outline read off the title does (a school building, a calendar, a chat bubble, else a grid).

**Link images are named, and the name must resolve.** An Image cell is either an object under `link-images/` in the media bucket, content-addressed like photos and written by the image picker (an upload, or a picture found through the search - `internal/imagesearch`, shared with HCA-Team - which fetches and stores it the same way), or a path to a bundled file under `web/home/` or `web/public/home/` (the way the sample data ships). A name that resolves to neither refuses the load, because a recorded image with nothing behind it is a bug, not a missing picture.

**A load either succeeds whole or refuses**, the same stance as the directory: the server does not start, or does not refresh, on a sheet that breaks a rule, and never serves a page quietly missing a link.

## Ordering

Categories and links have no order column. The sheet's row order is the display order, and moving a row in the sheet is how something is reordered. Adding through the app appends, so a new link lands last in its category until someone moves its row.
