# Gather theme kit

A Helios Gather invitation arrives as an envelope. Tapping its seal opens it to a card. A **theme** is the design for all of it: the envelope, its liner and seal, the card, its fonts and the page behind it.

When Helios Celebrate draws a celebration from an invitation, the invitation's theme dresses the whole Celebrate site too: `page.backdrop` behind every page, `page.ink` and `page.muted` for the words on it, the `sidebar` colors on Celebrate's sidebar and top bar, `card.accent` and `card.accentInk` on its buttons and highlights, and the title font and rule on its page headings. Party cards and panels stay white, so the backdrop needs a wash dark or light enough for `page.ink` to read on it. A Celebrate admin can also upload a theme in Edit Celebration and pick it as the celebration's site theme, dressing Celebrate in it while the invitation keeps its own.

This kit holds everything needed to make one:

| In the kit | What it is |
| --- | --- |
| `README.md` | This file: the format, the rules, and a prompt for an AI. |
| `standard.json` | Every setting a theme has, with the value it gets when the theme leaves it out. |
| `my-theme/theme.json` | A starting point. Rename the folder and edit the file. |
| `examples/` | Gather's own themes, each in its own folder with its pictures. |
| `preview.html` | Open it in a browser and choose a theme's folder. It draws the envelope and card the way Gather does and lists anything Gather would refuse. |

## Making a theme

1. Copy `my-theme` and edit its `theme.json`, or hand this whole kit to an AI with the prompt at the end of this file.
2. Open `preview.html` in Chrome, Edge, Firefox or Safari, click **Choose a theme folder**, and pick the theme's folder. Fix whatever it lists, then choose the folder again to check.
3. In Gather, open an invitation's editor, go to **Theme**, pick **Upload a theme**, and drop in the theme's folder, a zip of it, or its `theme.json` with its pictures. Gather finds the theme file and the pictures it names, and leaves out everything else.

Only your own invitations can use a theme you upload. Uploading again with the same `key` replaces it, and every invitation in it changes with it.

## The theme file

`theme.json` is one JSON object. It has three fields of its own:

- `key`: a short name in lower-case letters, digits and dashes, like `ice-rink`. Upload it again with the same key to replace it.
- `name`: what the Theme picker calls it, like `Ice Skating`.
- `blurb`: one line under the name in the picker.

Everything else is the design, in the sections below. **Write only what differs from `standard.json`.** A theme that leaves out a setting gets the standard's value. Every value is text, except `fonts.load`, which is a list. Gather refuses a setting `standard.json` doesn't have, so check the spelling against it.

Most values are CSS: colors, lengths, `background` layers, borders and shadows, written as they would be in a stylesheet.

### `fonts`

- `display`: the font stack for the title, like `"Marcellus, Georgia, serif"`.
- `body`: the font stack for everything else.
- `load`: the Google Fonts families to load, as Google's `css2` address writes them, like `["Marcellus", "Nunito:wght@400;600;700"]`. A font that isn't loaded falls back to the next one in the stack.

Text settings name a font as `display`, `body`, or a stack of their own.

### `page`

The page around the envelope and card, and the invitation's pages.

- `backdrop`: a CSS `background`: a color, gradients, or pictures. A picture here is the best thing a theme can have; see "Give it a background picture".
- `ink`, `muted`: text colors on the backdrop.

### `sidebar`

The invitation's sidebar and top bar in Gather, and Celebrate's when a celebration is drawn from the invitation.

- `background`: a CSS `background`.
- `ink`: its text color.
- `active`: the highlight behind the page you're on, and badges. Usually a translucent color.

### `envelope`

- `body`, `side`, `front`, `flap`: the envelope's paper, from the back to the closing flap.
- `liner`: a CSS `background` for the inside of the flap. Patterns and pictures look good here.
- `radius`: how rounded its corners are.

### `seal`

The sticker that closes the envelope.

- `color`: its color.
- `ink`: the color of what's written on it.
- `shape`: `circle`, `square` or `stamp`. A stamp has a dashed edge.
- `text`: `initial` writes the first letter of the invitation's title. Anything else is written as it is: a symbol or an emoji, like `"✦"`.
- `picture`: `none`, or a picture for the seal's face, like `url("seal.png") center / cover`. It's drawn over `color` and cut to the seal's shape, and `text` is left off. Write picture layers only, with no color of their own. A PNG with a see-through background lets `color` show around it. The invitation email still draws the plain colored seal with its letter, so pick a `color` that suits the picture.

### `card`

- `background`, `ink`: the card's paper (a CSS `background`) and its main text color.
- `accent`: the color for highlights: the line under the title, the hosts and the guests' buttons.
- `accentInk`: text on the accent color.
- `radius`, `padding`, `width`, `minHeight`: its shape and size. `padding` keeps the words clear of the art. Raise its top or bottom when pictures sit there.
- `align`: `center` or `left`.
- `details`: how the date and the place sit under the line beneath the title. `stacked` puts the day, the hours and the place one under another. `side-by-side` puts them in two columns with a thin line between: the day and the hours under a calendar icon, and the place's name and address (or the street and the town) under a map pin, the icons in the kicker's color. On a phone the two columns stack. The link preview picture lays them out the same way. The text settings `day`, `hours` and `place` style them either way.
- `border`, `shadow`: CSS `border` and `box-shadow`.
- `art`: a CSS `background` drawn over the paper and under the words: patterns, drawings and pictures.
- `artMask`: a CSS `mask` over the art, to fade it where the words go, or `none`.
- `artOpacity`: how strongly the art shows, from `0` to `1`.
- `frame`: a border drawn inside the card. `inset` is how far in, and `border` and `shadow` are CSS. Use `none` to have no frame.

### `text`

One setting each for the card's lines:

- `kicker`: the small line above the title. Its `words` say what it says, like `You’re invited to`.
- `title`: the invitation's name.
- `day`, `hours`, `place`: when and where. `day` also has a `format`: how the date is written, with `{weekday}` (Saturday), `{wkd}` (Sat), `{month}` (March), `{mon}` (Mar), `{day}` (6) and `{year}` (2027) standing for its parts and anything else written as it is. The standard is `{weekday}, {month} {day}, {year}`; `{wkd} · {mon} {day}` writes "Sat · Mar 6", and `{weekday}, {month} {day}` leaves out the year. The invitation's sidebar, its email and its link preview write the date this way too.
- `message`: the hosts' words to their guests.
- `hosts`: the "Hosted by" line.

Each has `font` (`display`, `body` or a stack), `size`, `weight`, `style` (`normal` or `italic`), `case` (`none` or `uppercase`), `spacing` (letter-spacing), `lineHeight` and `color`.

`rule` is the line under the title: `width`, `height`, `style` (a CSS border style such as `solid`, `dashed`, `dotted` or `double`) and `color`.

A text `color` is `ink`, `accent`, or a `#rrggbb` color.

### `words`

What the invitation's pages call things, in the theme's own voice. They show on Gather, on Helios Celebrate and on the page a guest from outside opens; the invitation email keeps the standard words.

- `coming`: the guest list's title for the guests, standard `Who’s coming` (the hosts' own stays Guest list). A space party might say `Who’s beaming down`.
- `yes`, `maybe`, `no`: the answers, on the RSVP buttons, the guest list's sections and the answers in the hosts' table, like `I’m in`, `Maybe`, `Can’t make it`.
- `none`: a guest who hasn't answered yet, standard `No response`, like `Still orbiting`.

Keep each short; they sit on buttons.

## The rules

Gather checks a theme when it's uploaded and refuses it with the reason if:

- It has a setting `standard.json` doesn't have, or a value that isn't text (`fonts.load` is a list of text).
- A setting is blank. Write `none` for a part a theme does without.
- `seal.shape` isn't `circle`, `square` or `stamp`, or `card.align` isn't `center` or `left`.
- `seal.picture` isn't `none` or picture layers with no color of their own.
- Any of these has no `#rrggbb` color in it: `page.backdrop`, `page.ink`, `page.muted`, `envelope.body`, `envelope.side`, `envelope.front`, `envelope.flap`, `envelope.liner`, `seal.color`, `seal.ink`, `card.background`, `card.ink`, `card.accent` and `card.accentInk`. Gather draws the invitation email and the link preview from these, and those can't show gradients or pictures. A gradient or picture works when it ends with a color layer, like `url("liner.jpg") center / cover, #1d4e89`.
- A `text` color isn't `ink`, `accent` or `#rrggbb`.
- `text.day.format` names a part in braces that isn't one of `{weekday}`, `{wkd}`, `{month}`, `{mon}`, `{day}` or `{year}`.
- A `url(...)` points anywhere but a picture in the theme's folder or a `data:` address. Write a picture by its file name alone, like `url("card-top.jpg")`. No web addresses.
- A picture the theme names is missing, or two pictures share its name.
- `theme.json` is longer than 50,000 characters once its JPEG, PNG, GIF and WebP pictures are moved into files (Pictures, below). Inline SVG drawings stay in it, so keep them small.
- There's no theme file, or more than one. Drop one theme at a time.

## Pictures

Pictures are JPEG, PNG, GIF or WebP files in the theme's folder, named in the design by file name. Gather turns each into a JPEG no more than 2048 pixels on its longest side, except an animated GIF, which stays a moving GIF. Keep pictures under a few megabytes. Pictures are drawn as CSS backgrounds, so `center / cover`, `100% auto` and `no-repeat` work as they do anywhere.

Save every photo or painted picture as its own file beside `theme.json` and name it there; don't write it into `theme.json` as a base64 `data:` address. If one is written in anyway, Gather moves it into a file of its own when the theme is uploaded.

Small drawings work well as SVG in a `data:` address, like `Ice Skating` in `examples/`. Keep each to a few thousand characters. Write `#` as `%23`, `<` as `%3C` and `>` as `%3E`, and use single quotes inside the SVG.

### Pictures that move

A picture can move if it's an animated GIF, named in the design like any other. Gather keeps it moving, redrawn at most 1280 pixels on its longest side; an animated WebP, or any other format, is drawn still. Link previews show the GIF's first frame, so make that frame stand on its own. Keep a moving picture short and small, a loop of a few seconds and a few megabytes, since a backdrop fills the screen of every guest who opens the invitation.

## Give it a background picture

A picture behind the page does more for a theme than anything else. It fills the whole screen around the envelope and the card, and behind every page of the invitation, so guests are somewhere from the moment they open it: a frozen pond at dusk, a torchlit great hall, a desert under a starry sky. Area 51 in `examples/` shows how much a backdrop carries. Every theme should have one unless it's meant to be plain.

- **What makes a good one:** a scene or texture with no words and no main subject in the middle, since the envelope and card cover the center. Busy edges and a quieter middle work best. Landscape, at least 1920 pixels wide.
- **How to write it:** name it in `page.backdrop`, with a dark or light wash over it so the card stands out, and a color at the end for the email:

  ```json
  "page": {"backdrop": "linear-gradient(rgba(8, 18, 22, 0.7), rgba(8, 18, 22, 0.8)), url(\"backdrop.jpg\") center / cover fixed, #0a1a20"}
  ```

  `fixed` keeps it still while the page scrolls. Pick `page.ink` and `page.muted` to read on the washed picture.
- **The same picture, elsewhere:** a crop of it often makes a good `envelope.liner` or band of `card.art`, so the envelope, the card and the page feel like one place.

## Making it look right

## Making it look right

- **Envelope shades.** The envelope's four parts are one color in four shades: `front` the lightest, `side` a step darker, `body` between them, `flap` the darkest, so the folds read. The seal stands out from the flap.
- **Contrast.** `card.ink` on `card.background`, `card.accentInk` on `card.accent`, `page.ink` on the backdrop and `sidebar.ink` on the sidebar each need to read easily, at about 4.5:1 or more. The sidebar is usually the theme's darkest color as a gradient.
- **Art stays out of the words.** The card's words fill the middle, from about 12% to 88% of its height. Keep art in the corners or along the top and bottom edges, by where it's placed, with `card.padding`, or with `card.artMask`. A long title wraps to two or three lines, so leave room for it.
- **Pictures behind the page** take a dark or light `linear-gradient` wash over them so the card stands out (see "Give it a background picture").
- **Type.** A script or decorative face is for the title alone. The body face should read easily at 18px. Pick `text.title.size` so a title of three or four words fits in two lines.

## A prompt for an AI

Give an AI this whole kit, or at least `README.md`, `standard.json` and one example, along with this:

> Make a Helios Gather invitation theme for **[the occasion, the mood, the colors you like]**. Follow README.md exactly. Write one `theme.json` with a `key`, `name` and `blurb`, holding only the settings that differ from standard.json. Give it a background picture: name `backdrop.jpg` in `page.backdrop` with a wash over it, as README.md's "Give it a background picture" shows, and also make the picture, or if you can't, describe it in a few sentences I can give an image generator. Draw the card art and the liner pattern with CSS gradients or small inline SVG in `data:` addresses, or name other picture files and make or describe them the same way. Never write a photo or painted picture into theme.json as base64; give it as a file. A picture that should move must be an animated GIF, as README.md's "Pictures that move" says. Keep the words easy to read over the art: use `card.padding` and `card.artMask` to keep the middle of the card clear. Every color the rules list must contain a `#rrggbb` color. Check every rule in README.md's "The rules" before you answer, and give me the finished `theme.json` and the pictures or their descriptions.

Then put the file in a folder of its own with its pictures, named as the theme names them, and check it in `preview.html`.
