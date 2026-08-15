# Design language

Visual reference for the apps, extracted from the existing directory app. Colors are sampled from live captures; treat them as the working palette until original brand assets arrive.

## Palette

Brand:

| Color | Hex | Use |
|---|---|---|
| Brand teal | `#014E54` | Canonical brand color (the app manifest's theme color): login backdrop, detail-page family bands, role/title small-caps labels, primary buttons. Renders near `#1f4d53` in the UI |
| Sidebar teal | `#173c41` | Sidebar background (darkest surface) |
| Teal highlight | `#2f5054` | Selected sidebar item |
| Band row teal | `#29555b` | Row highlight inside dark bands |
| Sun yellow | `#fee000` | Large sun disc in the logo lockup; celebratory accent |
| Sun orange | `#f68d1e` | Small disc standing in for the wordmark's "o", overlapping the sun |
| Overlap red | `#ef563f` | Where the two discs overlap |
| Brush lime → green | `#69c01f` → `#0b963e` | Gradient of the "Who?" brush script and its underline |

Neutrals:

| Color | Hex | Use |
|---|---|---|
| White | `#ffffff` | Content background, cards |
| Row gray | `#f5f5f5` | List rows, zebra sections |
| Input gray | `#efefef` | Search fields, filter buttons |
| Ink | `#0d0d0d` | Headings, names, values |
| Muted | `#707070` | Secondary lines, field labels |

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

- The lockup: "Helios" in a light-weight white geometric sans, its "o" replaced by a small orange disc overlapping a much larger yellow sun disc (red where they overlap), with "Who?" sweeping underneath in a lime-to-green gradient brush script, finished with a brush underline. It sits on the brand teal.
- The app icon and splash art are the same lockup.
- The wordmark's geometric face appears only in the logo artwork; it is not the UI font.
- Actual raster assets pulled from the existing app live locally (uncommitted) in `screenshots/brand/`: favicons (16, 32), app icons (192, 512, plus maskable variants), the full-lockup splash (2732×2048), and the PWA manifest. Vector originals still need to come from the school.

## Classroom mascots

The nine classrooms are birds — Hummingbirds (K); Hawks and Falcons (1st/2nd); Jays and Ravens (3rd/4th); Condors and Ospreys (5th/6th); Herons and Egrets (7th/8th). Each classroom tile is a square: bird silhouette art (black, or black and white for the water birds) over a yellow sun disc on the band's ground color, named in white brush script. Grade tiles reuse the system, combining both of the band's birds in one composition ("Grade 1" shows hawk and falcon together); Kindergarten's grade tile is a mirrored hummingbird variant. People without photos use a soft illustrated bird avatar in the same spirit.

Original tile art (1254×1254 JPEGs for all nine classrooms and all nine grade tiles) lives locally (uncommitted) in `screenshots/brand/classrooms/`, with a `contact-sheet.png` overview.

## Typography

- The UI font is **Inter**, loaded from Google Fonts in weights 400, 500, 600, 700, 800 (`fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800`), with the fallback stack `-apple-system, system-ui, "Segoe UI", Roboto, Oxygen, Ubuntu, Cantarell, "Open Sans", "Helvetica Neue", sans-serif`. Body text is weight 400; the weights observed in use are 400/500/600.
- The existing app also loads Roboto (400/500/700/900) and Roboto Mono for its raw data grid; the clone does not need them.
- Hierarchy: bold page headings; semibold names; letterspaced ALL-CAPS micro-labels in brand teal for roles, titles, and section badges; regular body; muted gray secondary text; italics for pronunciation lines.
- The brush script appears only in brand and mascot artwork, never as UI text.

## Shape language

- Circular avatars for people; rounded-rectangle (~12px radius) cards, photos, and mascot tiles.
- Pill-shaped search inputs and filter buttons; circular icon buttons for quick actions (message, mail, map, favorite).
- List rows with right chevrons; hairline dividers; generous whitespace.
- Detail pages break the white page with a full-width deep-teal band for family content.
- Inline separators: "▶" chains grade to team to subteam; "·" dots separate contact fragments.

## Responsive chrome

- Phones are the primary target; the desktop layout is the adaptation, not the other way around.
- Narrow screens replace the sidebar with a brand-teal top bar (hamburger, page title, or back arrow + record name) and a bottom tab bar of icon-and-label items for the everyday sections.
- Content keeps one structure across widths: grids go from multi-column to two columns, detail blocks stack full-width, and the teal family band spans edge to edge.
- The status-bar area is part of the brand chrome (`viewport-fit=cover`, translucent status bar over teal — see `docs/pwa.md`).
