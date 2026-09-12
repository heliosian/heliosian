# The toolbar

Every app puts the same bar across the top of its content: 48px tall in a pale wash of the brand teal (`#ecf3f2`) under a hairline, the app's own search on the left, and on the right the signed-in person's face and then, at the far edge, the switch to the other apps. It is one stylesheet and one script, `web/common/toolbar.css` and `web/common/toolbar.js`, served to every app behind sign-in (`app.Files` reads `web/common/` after the app's own directory); each app's `index.html` links the stylesheet ahead of its own, and its `style.css` supplies the palette the bar's rules read (`--brand`, `--line`, `--ink`, `--muted`, `--input`).

## Search

The pill holds whatever the app searches - Who? finds people, grades and gradebands as you type; HCA-Team filters the page that is open, or jumps to the opportunities list with the words when the open page has no filter of its own; Heliosian filters its links. A `/` badge at the pill's right edge names the shortcut: a bare `/` anywhere on the page (no modifier, nothing else focused) puts the cursor in the box, the way GitHub and Slack do, and `toolbar.js`'s `onSlash` is that binding. On a phone Who? and HCA-Team fold the box behind a magnifier in their compact bar, and `/` opens that overlay instead.

## The alerts

Before the avatar, the same two badges in every app: a red count of the things the directory wants updated for the new year (a student's photo or facts past the config sheet's thresholds, a family photo missing or old), and a warning triangle when the family's Helios Who? privacy settings disagree with Veracross. Who? reckons them client-side from its own model (`web/who/stale.js`, `pages/privacy.js`) and hangs its to-do dropdown off the count; the server reckons the same numbers in `internal/who/alerts.go` (`Model.Alerts`, with a test pinning it to the client's answer for the sample parent) and hands them to HCA-Team and Heliosian in their models as `alerts`, whose toolbars link the badges across to Who?'s My Family and My Privacy pages (`renderAlerts` in `toolbar.js`).

## The account

The avatar is the hero photo the directory leads with for the viewer - their own, else their family's - or their initial when there is none, filled by `renderAvatars`; each app's model carries `user.photoUrl` and `user.initial` for it. While Super Admin Mode is on - the same switch, under the same name, in every app's account menu - the avatar wears a red dotted ring (`body.is-super`, set by each app through `markSuper`), so the mode is never on unnoticed. There is no name label: the avatar's only job is opening the account menu, whose contents are the app's own (View Profile and My Privacy in Who?, the admin switches in HCA-Team, Edit Categories in Heliosian, Sign Out in all).

## The app switch

The Heliosian mark on its white tile with a chevron, at the bar's far right. It opens a list of every app - Heliosian, Helios Who?, HCA-Team - each with its transparent-background mark, name and tagline, the current one tinted, and under them a line - "Built for the community, by the community" behind the GitHub mark - linking to the repository. `initAppSwitch` in `toolbar.js` fills and wires every `.app-switch` in the page. The links follow the tier of the page's own hostname: from `who.lab.heliosian.com` the portal is `team.lab.heliosian.com`, from `who.local.heliosian.com:8080` it is `team.local.heliosian.com:8080`, and Heliosian is the apex (`heliosian.com`) only in production, `home.<tier>` elsewhere; `hca.<tier>` counts as the portal's. The tile and the marks live in `web/public/common/brand/` (`heliosian-tile.png`, `apps/{home,who,team}.png`), which every app serves under `/brand/`.

## Admin Tools

Every app's Admin Tools wears the same chrome, from `web/common/admin.css` (scoped under `.admin`): a teal header with the app's mark, "Admin", the signed-in address and a close button back to the app; a white rail of tabs in groups (Display, Editing & Control, and in Who? Data Overrides) on the left; cards on the right, one tab's panel at a time. Who? and Heliosian serve it as a page of their own (`admin.html`); HCA-Team draws it inside its shell at `/admin`, whose rail, toolbar and tab bar step aside (`body.is-admin`). On a phone the rail becomes a strip of pills (`.admin-strip`), except in Who?, which keeps its fold-out menu.

## On phones

Who? and HCA-Team already had a 48px brand-coloured bar - hamburger, title, magnifier, avatar - in place of the sidebar; that stays, with the avatar and the app switch (tile only, no chevron) at the shared size. Heliosian has no sidebar to fold away, so its toolbar simply sticks under the header strip that carries its mark and section links.
