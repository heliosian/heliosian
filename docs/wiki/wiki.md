# Helios Wiki

Helios Wiki, at `wiki.heliosian.com`, is parent-to-parent info: a tree of pages anyone in the community writes and anyone signed in reads. It is an app like any other in `model.Apps`, so its place on Heliosian and the app switch follows its row on the Apps sheet's `Visibility` tab, and a new deployment finds no row and adds one listing nobody (`docs/home/data.md`).

It is built on the data model alone (`docs/datamodel.md`): it reads through `QUERY /api/q` and writes through `/api/do/wiki` and `POST /api/q`, as the signed-in person, so the policies decide everything it shows and allows.

## Pages

A page is a `DOCUMENT` of kind `wiki`: its `name` is the title, `author` the person who started it, `published` when they did, and `content` a `CONTENT` row of type `text/markdown` holding the page's Markdown. The extractor, search and inbox leave wiki pages alone.

Pages form a tree. A page's `parent` is the wiki page it sits under, blank for a page at the top, and `order` its place among its siblings. A sub-page keeps its kind, the one exception to the data model's rule that a document under another has a relation and no kind; the load refuses a wiki page under any document that is no wiki page, and a page whose parents lead back round to itself (`checkDocuments` in `internal/db/model.go`).

A page may have side cards, shown down its right: each a `DOCUMENT` under the page with the relation `side`, its `name` the card's title, its `content` the card's Markdown, and its `order` its place among the page's cards. The load refuses a side card under anything but a wiki page. Cards are the page's alone: its sub-pages don't show them.

A page's or a card's Markdown may show pictures, `![words](/api/wiki/picture/<sha256>.<ext>)`: a picture goes up, and comes in from the picture libraries, through the same image routes HCA-Team, Celebrate and When use (`imagesearch.Search.Register`, under `/api/wiki`, the folder `wiki-images` from `imageFolders` in `internal/blob/images.go`), into `wiki-images/` in the media bucket, named by its SHA-256, and `GET /api/wiki/picture/{name}` (`registerWiki` in `internal/db/wiki.go`) answers it from the bucket to anyone the directory signs in, for a name the `wiki-images/` pattern allows and nothing else, under a `Content-Security-Policy: sandbox` of its own. The shared `markdown.js` draws an image only from that path, so Markdown anywhere, a wiki page or an Ask answer, never loads a picture from elsewhere; any other image is drawn as its words.

`POST /api/do/wiki` takes `{"document", "parent", "name", "body", "sides"}` and saves a page: with no `document` it starts one under `parent`, last among its siblings; otherwise it renames and rewrites that one and, when `parent` differs from the page's own, moves it there, last among its new siblings, so every save names the parent the page should have. A parent that is no wiki page, or that is the page itself or one of its own sub-pages, is refused. `sides`, when sent, is every card the page should have, in order, each `{"id", "name", "body"}`: a card with no `id` is added, one with an `id` of the page's is retitled, rewritten and placed as it says, and a card of the page's that the list leaves out is deleted; a save without `sides` leaves the cards as they are. The call stores every body as below, the body in the media bucket under `content/` and its `CONTENT` row, unless a row with the same bytes already exists, and in the same transaction inserts or sets the `DOCUMENT` as the caller, answering `{"result", "hash"}`. The `CONTENT` row is the server's to make: no policy lets a person insert one, so a page's bytes are always what was sent. Each save points the page at its new content; the previous content, held by nothing, goes with the sweeper, so a page keeps no history of its own. Reordering is a set of the page's `order` through `POST /api/q`.

## Who may do what

The clauses are in `policySource` (`internal/db/policy.go`), under Everyone, Wiki admins and System: import:

- Anyone signed in reads every page, as for any document sent to no group.
- Anyone but a guest starts a page, as its author, and renames, rewrites, moves or reorders any page, and adds, changes, reorders or removes its side cards. Its content must be Markdown that no document but a wiki page holds.
- The import key (`docs/datamodel.md`, The query API) starts pages with no author, and their side cards, for loading a document into the wiki; since it is no guest, the clauses above let it rename, rewrite and move pages too.
- A page with no sub-pages is deleted by its author, and by a wiki admin, its side cards deleted with it in the same batch. A page with sub-pages can't be deleted until they are moved or deleted. A wiki admin is an effective member of the `admins` group the `wiki` `APP` row names, or a super admin.

## The app

`web/wiki/` is the client, served for `/`, `/new`, `/p/{id}` and `/p/{id}/edit` (`internal/app`). No page says who started it or when.

- The rail is laid out as HCA-Team's is: main items with an icon, and the rows under each behind a rule. On a page, the first main item is the top-level page that page sits in, with everything under it, each branch folded but for the open page's and what the viewer opened, each fold remembered in the browser. Then All Pages, a link to `/`, which lists the top-level pages; the rail lists none under it. New Page, a small button, closes the rail. A star marks each page the viewer started. Dragging a page in the open section above or below a sibling reorders it.
- `/` lists the top-level pages with how many pages each holds; a search in the top bar lists every page whose title holds the words, with its path.
- A page is laid out as Celebrate lays out a party. Across the top, the path to the page on one row with Add Sub-Page and Edit Page. Then its title, its Markdown drawn by the shared `markdown.js` and the pages under it; beside them, On this page lists the page's headings, each scrolling to its heading, and the page's side cards follow it. The page knows the viewer is a wiki admin by asking for the viewer's own `PERSON` row under `(admin_of "wiki")`.
- `/new` (with `?parent=` for a sub-page) and `/p/{id}/edit` are the editor: the title, the page it sits under, the Markdown with Insert Image under it, which uploads a picture and puts its Markdown at the cursor (each side card has the same as an icon), then Save and Cancel, and beside them, as on the page, the side cards, each a title and Markdown that moves up or down or goes, with Add Card; a card left with neither a title nor any Markdown is dropped when the page is saved; and on an existing page, for its author or a wiki admin, Delete.

The sample server's wiki is in `sampledata/datadocuments/` (`DOCUMENT` and `CONTENT`), with the pages' Markdown in `sampledata/bucket/content/` (`docs/dev.md`): a fictional tree with sections, sub-pages and a side card on Camping Trips.
