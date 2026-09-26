# Brand assets

Every app's marks, icons and lockups are drawn by hand in one family, and their sources live in `asset-sources/`, which never enters an image (`.dockerignore`, `.gcloudignore`): a folder per app, named for the app as the designer knows it (`heliosian`, `who`, `hcateam` for HCA-Team, `when`, `celebrate`, `birthday`, `loop`, `ask`), each holding the Photoshop files and an `exports/` folder of flat PNGs cut from them - `app icon.png`, the mark on its opaque teal square; `favicon.png`, the bare mark on a transparent ground; `symbol_white.png`, the mark in white; and the lockups, `horizontal - teal.png`, `horizontal - white.png`, `vertical - teal.png` and `vertical - white.png`. `decorative/` beside them holds Heliosian's hero and rail pictures, each in a light and a dark version.

## From an export to the app

Each file under `web/public/<app>/brand/` is cut from its app's exports at the width the file already has, so rebuilding changes the art and nothing about the layout:

- `favicon-*.png`, `favicon.png` and `logo-mark.png` - the bare mark, transparent.
- `icon-*.png` and `apple-touch-icon.png` - the app icon square, opaque, since a transparent home-screen icon composites onto black on iOS.
- `maskable-icon-*.png` - the bare mark at 62% of the square on the app icon's corner colour, which keeps it inside the maskable safe circle.
- `logo-tile.png` - the app icon under a rounded mask with a 22% radius, applied last with `-compose CopyOpacity`.
- `symbol-white.png` and `symbol-watermark.png` - from `symbol_white.png`.
- `logo-lockup*.png` - the horizontal export for a file wider than twice its height, else the vertical one; the white export for a name with `light` in it, else the teal.

A lockup is always one of the exports, trimmed and scaled, and never assembled from the mark and the words: a lockup put together here does not match the designer's. When two apps' marks come out at different sizes, the fix is in the source files, not in the cut. The apps share one look - the same rail, the same toolbar, the same page ground - and there is no per-app theme to set.

New art comes from the designer. Nothing here draws or extends a picture beyond cropping, resizing and compositing what an export already holds.

## Size and colour

The exports are 16-bit PNGs, so every cut passes `-depth 8`, or the files come out several times larger than they need to be. A flat mark or lockup is then reduced to an 8-bit palette, which loses nothing visible on flat art:

    magick <export> -resize <width> +dither -quantize transparent -colors 255 -depth 8 -strip PNG8:<out>

Painted and soft-edged pictures - the hero and rail landscapes, anything with an alpha fade - are never palette-reduced: a 255-colour palette with one alpha level per entry turns their gradients and fades into hard bands. They are resized and stripped only, kept in full colour, and allowed to be larger.
