# Data model

The tabs, columns, and validation rules are in `internal/home`; this file carries only what reading that code cannot tell you.

Home's data lives in one Google Sheet, `Apps`, in the community shared drive, reached through drive membership like the directory's sheets. `APPS_SHEET` names it. `cmd/createtabs` lays it out from an empty spreadsheet titled `Apps`; the layout is chosen by the spreadsheet's own title, so the same tool serves the directory sheet.

## Tabs

- `Categories` — Title, Image. Row order is display order.
- `Links` — Title, Description, URL, Image, Category, Visible, Added By, Added. Row order is display order within a category.
- `Admins` — Email. Who may edit, beyond the platform super admins.
- `Change Log` — Timestamp, Actor, Action, Kind, and the link or category fields. Appended on every change; never read back.

The sheet is the schema from scratch, not an import of the Glide app's tables. The Glide app's links were carried over once, transcribed from captures of its pages, with their images copied into the bucket.

## Rules

**Title is the key.** Links and categories are edited by title, so a title must be unique within its tab. An edit that renames carries the new title in the same write, so the row keeps its place; a category rename rewrites the Category cell of every link in it.

**Visible is Yes or No**, spelled exactly so. Any other value refuses the load.

**A category must exist before a link names it**, and cannot be deleted while a link still does.

**Images are named, and the name must resolve.** An Image cell is either an object under `link-images/` in the media bucket, content-addressed like photos and written by the image picker, or a path to a bundled file under `web/home/` or `web/public/home/` (the way the sample data and the category marks ship). A name that resolves to neither refuses the load, because a recorded image with nothing behind it is a bug, not a missing picture.

**A load either succeeds whole or refuses**, the same stance as the directory: the server does not start, or does not refresh, on a sheet that breaks a rule, and never serves a page quietly missing a link.

## Ordering

Categories and links have no order column. The sheet's row order is the display order, and moving a row in the sheet is how something is reordered. Adding through the app appends, so a new link lands last in its category until someone moves its row.
