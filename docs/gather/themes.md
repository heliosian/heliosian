# Gather themes

A theme is everything Helios Gather draws an invitation in: the page behind it, the sidebar beside it, the envelope and the liner inside its flap, the seal, the card, and every line of words on the card. A theme is data and nothing else: no theme has CSS, markup or code of its own. `web/public/gather/envelope.css` draws every envelope and card from CSS custom properties, and `themed` in `web/public/gather/themes.js` sets those properties from a theme.

The format, every setting and the rules a theme keeps are written for the people and AIs who make themes, in the theme kit's `README.md` (`web/gather/themekit/README.md`).

## Where themes live

A theme is a `THEME` row in the CONFIG sheet: its `key`, `name`, `blurb`, the `design` as JSON holding only where it differs from the standard, and `added_by`. A blank `added_by` makes it the site's own, which every host picks from; a host's upload is theirs, and only they can choose it for an invitation. Anyone who can see an invitation can read the theme it's drawn in. The pictures a design names are `THEME_PICTURE` rows, one per file name, uploaded as `original` and drawn from `picture`, the JPEG the pictures service makes. The rules are in `internal/db/policy.go` under THEME; `GROUP.theme`, `envelope`, `seal` and `liner` reference `THEME` rows, and `usable_theme` keeps an invitation to the site's themes and its host's own.

The standard is `internal/db/themebase.json`: every setting with its value when a theme leaves it out. `themeChecked` (`internal/db/themes.go`) holds every `THEME` row to it as it's written: the key's form, no setting the standard lacks, none blank, the seal shapes and card alignments the CSS draws, a `#rrggbb` color in every part the mail envelope and letter are drawn from, text colors the mail can draw, and pictures named only by a bare file name or a `data:` address. Deleting a theme an invitation uses leaves that invitation a copy of it (`themeInUse`).

`plain`, the site's theme with that key, is what Create Your Own starts from.

## Uploading and managing

A host uploads a theme from the editor: Theme, Upload a theme, a drop area that takes the theme's folder, a zip of it, or loose files, dragged in or chosen (`themeUpload` in `web/gather/editor.js`). Several themes go in one drop: each zip and each folder is a theme of its own and the loose files together are one more (`themeGroups`), each sent in turn, and those that fail stay in the drop area with their reasons while the rest upload. Every file of a theme goes to `POST /api/do/theme` (`registerThemeUpload`), which unpacks zips, takes the one `theme.json` (or, with none of that name, the one `.json` file) as the theme, keeps the pictures the design names, and leaves out every other file. A picture the design names and the upload lacks is refused. A JPEG, PNG, GIF or WebP written into the design as a `data:` URL is taken out into a picture of its own, `inline-1.png` and on, the same bytes once however often they appear, and the design names it instead (`filePictures`); inline SVG stays where it is. A design still longer than a sheet cell takes is refused, since `THEME.design` is one cell. Uploading the same key again replaces the host's theme and its pictures; a Gather admin's upload of a key one of the site's own themes has, and none of theirs does, replaces that theme and keeps it the site's (`gatherAdmin`), where anyone else's makes a private theme of their own beside it.

Gather's admins manage every theme, the site's own and every host's private ones, from Manage themes in Gather's sidebar (`web/gather/thememanager.js`), which only they see; the pages read admin status the way When's calendar does, from an `APP` row only an admin can read. It lists each theme with its envelope, its name and blurb, and whose it is, and offers Rename (a popup for the name and blurb), Make public (blank `added_by`, the site's own), Make private (`added_by` the admin, so only they can pick it) and Delete. The editor's Theme picker also shows an admin a trash button on a site theme's card (`removableTheme` in `web/gather/editor.js`). Deleting a theme deletes its pictures. Where any invitation's theme, envelope, seal or liner is that theme, `themeInUse` first gives whoever made each such invitation (its `GROUP.added_by`) a private copy - the same name, blurb, design and pictures over the same `CONTENT`, keyed as the theme was or, where the maker already has that key, with `-copy` after it, one copy per maker - and points those columns at it, so the invitations look as they did; the delete trigger's writes skip the policies, so the copy is the maker's whoever deletes. The site's `plain`, which Create Your Own starts from, can be neither deleted (`themeInUse`) nor made private (`themeChecked`); a refusal is shown as it comes. A theme made private while other hosts' invitations use it keeps drawing them, since anyone who sees an invitation reads its theme, but those hosts can't pick it again.

## How pages get themes

`GET /open/themes?ids=…` returns each theme laid over the standard, with every picture name turned into `/open/theme-picture/{id}?c={original}`, the `THEME_PICTURE` row and the upload it holds. That route serves the row's made picture to anyone, the same as share cards; when the pictures service hasn't made it yet, as just after an upload, the request waits for it (`waitMade` in `internal/db/pictures.go`). The address changes with every new upload, so the picture is cached for good under it. Uploading a theme fetches it afresh (`refreshThemes` in `web/public/gather/themes.js`), so its new pictures show at once. The signed-in pages list the themes the viewer can pick with an `/api/q` query and load them with the themes their invitations use (`loadModel` and `fetchInvite` in `web/gather/state.js`). The outside guest's page loads the themes its view names.

## The theme kit

`GET /open/theme-kit.zip` (`themeKit` in `internal/db/themekit.go`) builds the kit a theme is made from, which the upload popup links to: the README, the standard, a starter in `my-theme/`, every site theme but `plain` in `examples/` with its pictures, and `preview.html`. The preview works opened from disk: the kit inlines `envelope.css`, the standard, the check's lists, and `elements.js`, `themes.js` and `envelope.js` bundled into one script (`bundledModules`, which takes only `import {…} from '…';` lines), so it draws the envelope and card with Gather's own code. Its check is the upload's, written again in JavaScript in `web/gather/themekit/preview.html`; the Go check decides.

## Sample themes

The sample data's site themes are the packages in `sampledata/themes/<key>/`, each a `theme.json` and its pictures. `TestTheSampleThemesAreTheirPackages` holds the `THEME` and `THEME_PICTURE` rows in `sampledata/dataconfig/` to them; the pictures are sample uploads in `sampledata/bucket/content/` with `CONTENT` rows in `sampledata/datadocuments/CONTENT.csv`.

## Seeing one

A theme is checked by eye in the kit's preview, or in the app the way `docs/screenshots.md` takes a capture. A sample invitation, Theo Turns Eight, is hosted by Maya Lindqvist, so signed in as her, the editor switches its design and the capture shows the card in it, the `n` in `nth-child` being the theme's place in the picker after Create Your Own and Upload a theme:

    go run ./tools/startserver --email maya.lindqvist@example.org --capture https://gather.heliosiandev.com:8080/g/theo-turns-eight --out local/screenshots/theme.png --wait ".env-seal" --click ".env-seal|.invite-edit|.editor-pages-link:nth-child(3)|.theme-head .button|.theme-tile:nth-child(n)|.modal-actions button[type=submit]" --settle 3s

The sealed envelope is the same command without `--click`.
