# Brand assets

Every app's marks, icons and lockups are drawn by hand in one family, and their sources live in `asset-sources/`, which never enters an image (`.dockerignore`, `.gcloudignore`): a folder per app, named for the app as the designer knows it (`heliosian`, `who`, `hcateam` for HCA-Team, `when`, `celebrate`, `birthday`, `loop`, `ask`, `wiki`, `mcp`), each holding the Photoshop files and an `exports/` folder of flat PNGs cut from them - `app icon.png`, the mark on its opaque teal square; `favicon.png`, the bare mark on a transparent ground; `symbol_white.png`, the mark in white; and the lockups, `horizontal - teal.png`, `horizontal - white.png`, `vertical - teal.png` and `vertical - white.png`. `decorative/` beside them holds Heliosian's hero and rail pictures, each in a light and a dark version.

## From an export to the app

Each file under `web/public/<app>/brand/` is cut from its app's exports at the width the file already has, so rebuilding changes the art and nothing about the layout:

- `favicon-*.png`, `favicon.png` and `logo-mark.png` - the bare mark, transparent.
- `icon-*.png` and `apple-touch-icon.png` - the app icon square, opaque, since a transparent home-screen icon composites onto black on iOS.
- `maskable-icon-*.png` - the bare mark at 62% of the square on the app icon's corner colour, which keeps it inside the maskable safe circle.
- `logo-tile.png` - the app icon under a rounded mask with a 22% radius, applied last with `-compose CopyOpacity`.
- `symbol-white.png` and `symbol-watermark.png` - from `symbol_white.png`, the symbol centred on a square canvas and the watermark trimmed, at about 6% opacity (alpha at most 16/255). Who keeps these two, and `logo-wordmark.png` (the white horizontal export), in `web/who/brand/` rather than under `web/public/`.
- `bot-face-1.png` and `bot-face-2.png` - Ask's bot in the conversation, from its `symbol_nochat_face1.png` and `symbol_nochat_face2.png`, the bare face without the chat bubble, resting and open-mouthed.
- `logo-lockup*.png` - whichever of the horizontal and vertical exports, trimmed, is nearer the file's own shape, since an app's two lockups differ in proportion (Who's vertical one is taller than wide, the wiki's nearly square); the white export for a name with `light` in it, else the teal.

- `web/public/common/brand/apps/<app>.png` - the app's mark wherever another app shows it (the app switch, Heliosian's apps), the bare mark from `favicon.png`; and `<app>-outline.png`, where an app has one, the white symbol from `symbol_white.png`, which the rails and lists draw through as a mask.

- `splash/splash-<size>.png` - the iOS launch screens the login page lists (`web/templates/login.html`), one for every device size it names: the white vertical export, trimmed, 55% of the screen's shorter side wide, centred on the brand teal. Every app has the same set of sizes.

A lockup is always one of the exports, trimmed and scaled, and never assembled from the mark and the words: a lockup put together here does not match the designer's. When two apps' marks come out at different sizes, the fix is in the source files, not in the cut. The apps share one look - the same rail, the same toolbar, the same page ground - and there is no per-app theme to set.

New art comes from the designer. Nothing here draws or extends a picture beyond cropping, resizing and compositing what an export already holds.

## Size and colour

The exports are 16-bit PNGs, so every cut passes `-depth 8`, or the files come out several times larger than they need to be.

Anything with a transparent ground - the bare marks, the lockups, the tile's corners, the white symbols, the bot's faces, the app marks - is kept in full colour with its full alpha:

    magick <export> -resize <width> -depth 8 -strip PNG32:<out>

Never reduce one of these to a palette. ImageMagick's `PNG8` keeps one bit of alpha, so every soft edge the designer drew becomes a hard step and the mark looks jagged on any ground but white, and a one-colour export such as `symbol_white.png` collapses to a bilevel image with no alpha at all, which a mask then draws as nothing. The exports' edges carry all 256 alpha levels; the cut should too, at the cost of a few kilobytes a file.

Only art on an opaque ground - the app icons, the maskable icons and the splash screens - is reduced to an 8-bit palette, which loses nothing visible there:

    magick <export> -resize <width> +dither -colors 255 -depth 8 -strip PNG8:<out>

Painted pictures - the hero and rail landscapes - are never palette-reduced either: a 255-colour palette turns their gradients into hard bands. They are resized and stripped only, kept in full colour, and allowed to be larger.
