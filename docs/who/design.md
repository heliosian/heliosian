# Design language

Visual reference for the apps.

## Palette

Every app, Admin Tools page and sign-in page takes its colours from one stylesheet, `web/public/common/palette.css`, linked first in each page's head: the brand teal, the accent teal, the page and card grounds, the lines, the ink, the muted text, the input ground, the one alert red, and the logo swatch's greens and golds. It sits in the public tree so the sign-in pages, served before sign-in, reach it too. An app's own `:root` holds only what is its alone, such as Helios When's day-type colours. Dark mode (`web/common/dark.css`) and Quan mode (`web/common/quan.css`) redefine the same names. Each colour has one name, the role's where there is one: `--brand` for the teal as words and marks, `--brand-deep` for the deepest teal (the ink on a lime button, a tooltip's ground), `--brand-dark` for the darker shade a brand fill takes on hover, `--amber` for the pending and today gold, `--green-2` and `--green-3` for the buttons and their hover. Dark mode pales `--brand` so it reads as words on the dark page, so a teal ground that carries white words uses `--brand-fill` (defined in `dark.css`), which stays deep in dark mode and turns pink with the rest in Quan. Stylesheets write colours as these variables rather than as literals; the categorical `--cat-*` colours are used by name where a category takes one. A `theme-color` meta tag and a manifest's colours cannot read a stylesheet, so they carry the brand teal as a literal.

Grade-band colors (mascot tile grounds, one per band):

| Band | Hex | Classrooms |
|---|---|---|
| Kindergarten | `#d20210` red | Hummingbirds |
| 1st / 2nd | `#f55e03` orange | Hawks, Falcons |
| 3rd / 4th | `#fec502` yellow | Jays, Ravens |
| 5th / 6th | `#488925` green | Condors, Ospreys |
| 7th / 8th | `#0047af` blue | Herons, Egrets |

The app is light-on-white; dark teal is reserved for the sidebar, the family band on detail pages, and emphatic buttons. Small-caps labels reuse the deep teal as text ink, which ties the light pages to the brand without more color.

## Logo and identity

- The mark: a teal spiral-bound address book with a person on its cover, a green and a red book fanned behind it, and a yellow sun rising over them. The lockup sets "Helios" in white and "Who" in the brush lime with a yellow question mark, "A VISUAL DIRECTORY" letterspaced beneath; a dark-text variant exists for light grounds but the app only uses the white one, on the dark-teal sidebar and the admin header (`web/who/brand/logo-wordmark.png`).
- The app icons (`web/public/who/brand/icon-*.png`, `apple-touch-icon.png`) are the mark on `#0e4d54`; the maskable icon keeps the mark at 58% for the safe circle; the favicons are the bare mark on a transparent ground; the splash screens are the vertical white lockup centred on `#0e4d54`.
- The wordmark's geometric face appears only in the logo artwork; it is not the UI font.

## Classroom mascots

The nine classrooms are birds — Hummingbirds (K); Hawks and Falcons (1st/2nd); Jays and Ravens (3rd/4th); Condors and Ospreys (5th/6th); Herons and Egrets (7th/8th). Each classroom tile is a square: bird silhouette art (black, or black and white for the water birds) over a yellow sun disc on the band's ground color, named in white brush script. Grade tiles reuse the system, combining both of the band's birds in one composition ("Grade 1" shows hawk and falcon together); Kindergarten's grade tile is a mirrored hummingbird variant. People without photos use a soft illustrated bird avatar in the same spirit.

## Typography

- The UI font is **Roboto**, self-hosted as woff2 files in `web/public/common/fonts/` (weights 400, 500, 700, 900; no Google Fonts network dependency) with the fallback stack `-apple-system, BlinkMacSystemFont, system-ui, sans-serif`. Body text is weight 400; the weights in use are 400/500/700/900 — Roboto has no native 600 or 800, so anything that would have called for those collapses to the nearest real weight (600 → 500, 800 → 900) rather than letting the browser synthesize a fake bold, which renders visibly blurry.
- Hierarchy: black (900) for the rare oversized splash heading; bold page headings and names; medium-weight secondary emphasis; letterspaced ALL-CAPS micro-labels in brand teal for roles, titles, and section badges; regular body; muted gray secondary text; italics for pronunciation lines.
- The brush script appears only in brand and mascot artwork, never as UI text.

## Shape language

- Circular avatars for people; rounded-rectangle (~12px radius) cards, photos, and mascot tiles.
- Pill-shaped search inputs and filter buttons; circular icon buttons for quick actions (message, mail, map, tag).
- List rows with right chevrons; hairline dividers; generous whitespace.
- Detail pages break the white page with a full-width deep-teal band for family content.
- Inline separators: "▶" chains grade to classroom to crew; "·" dots separate contact fragments.

## Responsive chrome

- Phones are the primary target; the desktop layout is the adaptation, not the other way around.
- Narrow screens replace the sidebar with the shared toolbar pinned on top (hamburger, search, badges, avatar, app switch - `docs/toolbar.md`), a slim brand-teal strip under it for the page title (or back arrow + record name), and a bottom tab bar of icon-and-label items for the everyday sections.
- Content keeps one structure across widths: grids go from multi-column to two columns, detail blocks stack full-width, and the teal family band spans edge to edge.
- The status-bar area is part of the brand chrome (`viewport-fit=cover`, translucent status bar over teal — see `docs/who/pwa.md`).
