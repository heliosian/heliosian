# The community's documents

This is the old document store: every weekly newsletter, every announcement to all the families, everything each classroom's parent list has carried since 2018, the pages of the school's website, and the parent portal's school-wide and classroom pages with the documents they link, each kept as markdown and embedded. The server loads it with the rest of the store (Loading it, below), and nothing it serves reads it: Helios Ask and Heliosian read the data model's `DOCUMENT` table (`docs/datamodel.md`, Documents and Digest; `ask.md`), and the mail hook files the community's mail there alone. It is written by `tools/importartifacts`, from saved messages, pages and portal resources, and searched from a laptop by `tools/artifactsprobe`. The code is `internal/model/documents*.go`, with the Vertex AI embedder in `internal/artifacts`; this file carries what reading it cannot tell you.

## What is in it and what is not

Only mail that went to everyone or to a whole class is in it: the school's newsletter under each of its mailers, the announcement lists (`parentsandstaff`, `community`, `parents`, `parentsonly`, `parentsandstudents`, `newstudentfamilies`, `new.parents`), the chat everyone is on, each classroom's parents and students, and each grade band. The channel a message came through is kept with it and can be searched on its own.

The rules name what is in, never what is out, so a list nobody thought of stays out on its own. The gate is what is filed, since anything sent to a few people on behalf of the rest would be the whole community reading it. A message is in when one of these holds, in this order:

- Its list header names one of those lists on `heliosschool.org`: kind `list`, channel the list's name.
- Any other list header, and it is out.
- With no list header, its sender's address is at one of the newsletter's mailers' domains or under one (`newsletterMailers` in `internal/model/documents_channel.go`): kind `newsletter`.

A message written to one of those lists by address, with no list header, is out: it did not come through the list. The corpus holds rows of kind `announcement`, mail of that shape; nothing files more of them.

`MailChannel` in `internal/model/documents_channel.go` is that judgement for this store: given a message's list header and its sender it answers with the channel and the kind, or that the message is no part of the corpus. Every saved message goes through it, so one imported later is held to the same rule as the ones already on file, and `Build` refuses one that is not the community's. The mail hook makes the same judgement for the data model with lists of its own (`broadcastLists` and `newsletterMailers` in `internal/db/mailhook.go`, `docs/datamodel.md`, Documents).

Every page of the website that its sitemap names is in it, less three kinds. The staff directory and the calendar are left out because Helios Who? and the calendar carry them in their own structure. Pages that are not the school's words are left out: Finalsite's template instructions and widgets, and an ad and a test copy the site still publishes. So are pages of no use to a reader: login, search, the site maps, the 404 page and the website's privacy and accessibility statements. `excludedPages` in `internal/model/documents_page.go` is that list; the archive does not fetch those pages and `SavedPage.Build` refuses them.

The mail of the Helios Loop email lists is in it too: each post an email list sent is a document of kind `group` whose channel is the email list's name. A post to two email lists is two documents, one under each.

From the Veracross parent portal, only what went to every family or to a whole class is in it: the pages under its Information for Parents, School Programs and Classroom Pages menus, the HELP site (the Helios Everything Library Page, a Google Site of the curriculum by grade band) and the Google Docs, Slides and Drive files those pages link. Nothing under a student's record is in it, nor messages, billing, the family profile, the directory, the calendars or the health portal. Linked spreadsheets are left out, since they are sign-up sheets with families' names, and so is any link whose words name a class Lookbook or say "Meet Your Classmates", the decks of students' photos.

## Where it lives

A document is one message, one page or one portal resource, kept as markdown and cut into chunks, each chunk with the vector an embedding model gives it. The bucket holds the documents, the `Artifacts` spreadsheet is their index, and the server holds every chunk and vector in memory, so a search is one embedding call and a scan.

The `Artifacts` spreadsheet lives in the community shared drive beside the others and `ARTIFACTS_SHEET` names it. Its `Documents` tab has one row per document: Key, Title, Date, Author, Kind, Channel, Source, Chunks, Object. The key is the SHA-256 of the message id, which is the one name a message carries wherever it was delivered, so the same message reaching two of the accounts is one document; a page's or a portal resource's is the SHA-256 of its address. Source is `mail:` and the message id for a message, and the address for a page or a portal resource. A page's kind is `page` and its channel `website`; a portal resource's kind and channel are both `portal`. Object is the bucket name, `artifacts/<key>-<fingerprint>.json`, where the fingerprint is a hash of the document as rendered - its markdown, its chunks and the embedding model's name. A change to the converter, the chunker or the model therefore gives every message a new object name, which is how the import knows what is stale and the server knows what it need not read again.

The object is one JSON file: the row's fields, the whole markdown, and the chunks, each with its section path and its vector. A vector is written as base64 little-endian float32 rather than as decimal text, which is the difference between thirty megabytes of corpus and half a gigabyte of it. The width asked for is a few hundred of the model's three thousand dimensions, which the model nests so that a prefix is itself a good vector; at this corpus's size it retrieves as well and costs a quarter of the bytes to keep, fetch and hold. A document embedded by a different model or at a different width refuses the load and says to import it again, since vectors of two widths cannot be compared.

The objects sit in the media bucket under `artifacts/`, and no route serves them: they reach a browser only as the words Claude writes.

## From a message to a document

A message on its way in is a `DocumentMessage`: its id, subject, date, sender, recipients, list header and its text, as HTML or as plain text. A mailbox read out over JMAP is one JSON file per message under `local/imports/mail/`, which is gitignored: the headers and the text, not the whole message, since attachments are most of a mailbox's bytes and none of its words.

Mail reaches this store only through `tools/importartifacts`, from messages saved as JSON; the mail hook files the community's mail into the data model alone (`docs/datamodel.md`, Documents). A saved message is read into a `DocumentMessage` (`ReadSaved` in `internal/model/documents_message.go`), judged by `MailChannel` unless it already names its channel and kind, and built into a document by `Build`.

The id is what a document is keyed by, so a message saved from two mailboxes is one document. An issue of the newsletter is a separate message to each address, so it is known by its title and day as well.

An email list's posts are not filed here: the data model files each post Loop sends on, sent to its list (`docs/datamodel.md`, Documents; `docs/loop/data.md`, Mail). The rows of kind `group` already in the `Documents` tab are the posts Loop's earlier mailer filed, keyed by the SHA-256 of the email list's name, a slash and the message id, with the email list as the channel.

`tools/importartifacts` turns saved messages into documents:

    eval "$(go run ./tools/devenv)"
    go run ./tools/importartifacts --dry-run local/imports/mail/*.json
    go run ./tools/importartifacts --i-have-user-permission-to-spend-money local/imports/mail/*.json

Every message is read and rendered before anything is embedded, so what a run would spend is known before it spends it; `--dry-run` reports that and writes nothing, and printing one message's markdown is what naming a single file does. Embedding costs real money, so a run that writes refuses to start without `--i-have-user-permission-to-spend-money`.

For each message the HTML is rendered to markdown by the package's own walker, or the plain text is taken as markdown when that is all the message has, its hard wrapping joined back into paragraphs. Headings become `#` headings, and because one of the mailers sets its section banners as bold capitals in a paragraph of their own rather than as headings, a block that is nothing but bold capitals becomes a heading too. Paragraphs and layout cells become paragraphs, list items become `- ` items, links keep their address, and images, scripts and styles are dropped. Inline markup that turns out to wrap whole blocks - a mail template puts links around tables - keeps its words and gives up the wrapping. What a mailing list adds around a message is cut off: the subscription footer with its unsubscribe address, the link to the thread on the web, and the confidentiality notice a mail client puts under a signature. A message that reads as nothing but that footer has no words to index, and one already on file as its footer is taken off the corpus.

Every link a Veracross newsletter carries is a redirect under a `.veracross.com` host's `/c/` that carries the address of the person it was sent to, so none may be handed to a reader. Veracross writes the address it points at into the link itself, so each is turned back into that address with no request at all; one that cannot be turned back keeps its words and loses its address. Marks the mailer added to count clicks are left off the address. Filing a document never fetches anything: every other link, the former mailer's redirects among them, is kept as written.

## From a page to a document

`tools/archivesite` reads the sitemap Finalsite publishes at `/fs/pages/sitemap` and saves each page it names under `local/imports/site/` as a `SavedPage`: the address the request ended at and the whole HTML. A page that redirects off the site (the parent portal, the inquiry form, social media) is not saved. Several sitemap entries can redirect to the same page, which is saved once under its own address. The archiver waits five seconds between requests, as the site's robots.txt asks, and sends an Accept header, since without one the site answers 406.

    go run ./tools/archivesite
    eval "$(go run ./tools/devenv)"
    go run ./tools/importartifacts --dry-run local/imports/site/*.json

`tools/importartifacts` takes saved pages and saved messages alike; a file with a `url` is a page. A page's document is its `<main>` content without what the site repeats around it. The page title is dropped from the text because it is the document's title (the `<title>` less the site's name). The section menus and forms are dropped. So is the row of tabs over a set of panels, since each panel's title appears again as its heading. Relative links become absolute against the page's address. The document is dated by the day the site last published the page (`page-published`), its author is Helios School, and its source is its address, which the tools give the model as the document's url.

## From the portal to a document

The portal and the HELP site sit behind the school's sign-in and draw most of their pages in the browser, so `tools/archivebrowser` reads them through the capture browser (`docs/screenshots.md`) with a parent signed in, in a tab of its own. It knows nothing of either site: it begins at `--start` and follows every link to an address under `--prefix`, waits for each page's `--selector` content to stop changing, and saves that content as a `SavedResource` under `--out`: the address, the words of the first link found to it (or the page's own title when none has any), the page that linked it, when it was fetched, and the body as HTML or as text. The start page is saved only if it is itself under the prefix, so the portal's home page gives the crawl its menus and is not kept. A Google Doc is exported as HTML from a page on the document's own origin, since Google answers the signed-in export only to the browser holding the session. A deck is exported as text and a Drive PDF as its text through `pdftotext`, both downloaded by the browser into a temporary folder it is pointed at for the run and pointed away from after. A HELP page embeds most of its decks by their real addresses, which are what it exports. A deck linked only by its published address draws its slides as pictures, but the page carries each slide's SVG in its scripts, where every text box holds its words as an accessibility label; those labels are the deck's text, slide by slide. A text box that is nothing but a link, such as a slide's way back to HELP or on to another grade band, is left out, and so is any text on more than half of a deck's slides, which is its running title and footer. Words drawn only inside an image are not read. The site notices a reader who comes through too often, so the archive is run rarely and the import works from what it saved.

    go run ./tools/capturebrowser
    go run ./tools/archivebrowser --start https://portals.veracross.com/heliosschool/parent --prefix https://portals.veracross.com/heliosschool/parent/pages/ --selector .app-container --out local/imports/portal
    go run ./tools/archivebrowser --start https://sites.google.com/heliosns.org/help/welcome --prefix https://sites.google.com/heliosns.org/help/ --selector section --out local/imports/help
    eval "$(go run ./tools/devenv)"
    go run ./tools/importartifacts --dry-run local/imports/portal/*.json local/imports/help/*.json

`tools/importartifacts` knows a resource by its `format`. HTML is rendered by the same walker as mail, with relative links made whole against the resource's address and Google's `google.com/url?q=` wrappers turned back into the address they carry. Text keeps its lines, a page break becomes a paragraph break, and a line that is only a number, a deck's slide number, is dropped. A resource is dated by the day it was fetched, since the portal says nothing of when a page last changed, its author is Helios School, and its source is its address, which the tools give the model as the document's url.

## Chunks

The markdown is cut at its headings: each heading opens a chunk named by the path of headings above it (`ANNOUNCEMENTS & REMINDERS › Picture Day`), a heading in mixed case sits inside the capitals heading before it, the words before the first heading are a chunk of their own, and a section longer than six thousand characters is cut again at paragraph breaks, since the embedding model reads at most about two thousand tokens at a time. What is embedded is not the chunk alone but the document's title, its date in words and its channel over the chunk's section, so a passage saying only "sign up here" is still found by what it is about.

The chunks go to Gemini's embedding model on Vertex AI, twenty-five to a request, as retrieval documents; a question is embedded the same way as a retrieval query. Documents are then written to the bucket two hundred at a time, several at once, and each batch's rows committed as `importartifacts` through the store the import builds, every sheet as the server's is (`docs/storage.md`, The sheets), waiting for the write, so a run that fails part way has recorded all but its last batch. That store loads the documents on file the way the server does, through the dev tools' bucket cache (`local/cache/blobs/`, `docs/dev.md`, Real data), which is how the import knows what is already there. A message already on file under the same object name is skipped; one on file under an older rendering is imported again, its row updated in place and the object it superseded deleted once the commit is written, so the bucket keeps nothing nobody names. A document on file that now renders to no words has its row deleted and its object with it.

## Searching

The store's own search (`Documents.Search`, which only `tools/artifactsprobe` calls now) embeds the question and scores every chunk: the dot product of the normalised vectors, plus a small share for the words of the question the chunk itself holds, since names, dates and numbers are where an embedding alone is weakest. The newer document wins a tie. A reply quotes the message it answers, so the same words are in the corpus many times over; a passage already covered by a better-scoring one - inside it, or holding it - is left out, so the answer's room goes on different things rather than the same thing said again.`tools/artifactsprobe` runs the same load and the same search from a laptop, printing the top passages with their scores, so a question's retrieval can be read before Claude sees it.

## Loading it

The `Artifacts` spreadsheet is a part of the one store like every app's sheet (`docs/storage.md`): the server reads the sheet at every refresh, every change is a commit, and its Change Log holds what each changed cell held before. An object's name carries a fingerprint of the document inside it, so a name already read is the same document: the part's build keeps every document it has read by object name and fetches only what is new or has changed, a document filed here is held before its commit so the commit fetches nothing, and an ordinary refresh costs one read of the sheet. What it does fetch it fetches many at a time, since thousands of objects read one after another would take longer than the service is given to start. The objects go through the media store's disk cache like every other fetch, so a dev server or a tool run over the corpus pays for the download once; the bytes are parsed into the model and not held a second time.

## What it needs

- The `Artifacts` spreadsheet in the shared drive with its `Documents` tab (`tools/sheets create --sheet $DIRECTORY_SHEET --title Artifacts`, then `tools/createtabs`), named by `ARTIFACTS_SHEET` wherever the server runs (`docs/deploy.md`, Configuration values).
- The media bucket, which the server reads and the importer writes and deletes in as `directory@`.
- Vertex AI: the Vertex AI API on in the project, and `roles/aiplatform.user` on it for `directory@` (`docs/deploy.md`, IAM). Local runs act as `directory@` through the impersonated application-default credentials every tool here uses, so nothing more is set up on a laptop.

## Sample and tests

Sample mode's `Documents` tab is empty: `sampledata/artifacts/` holds a few fictional messages and one fictional page in the same forms the fetch and the archive save, which the tests read. The tests in `internal/model/documents_test.go` pin the markdown a newsletter renders to, a plain-text message's paragraphs, a page's markdown without the site's menus and tabs and with its links made whole, every kind's links unwrapped, the excluded pages being refused, a portal resource's markdown, a text resource keeping its lines, a saved file read as a message, a page or a resource by what it carries, the chunks following the headings, mixed-case headings nesting under capitals, a long section cut at its paragraphs, bold capitals becoming headings, the object name following the rendering, vectors surviving base64, a quoted passage being returned once, and messages placed by what they carry. What the school's mail reads as Inbox key points and Reminders to-dos is the data model's digest (`docs/datamodel.md`, Digest).

## What is left to do

The website's pages are as fresh as the last archive: a page the school edits changes in the corpus only when `tools/archivesite` and the import are run again, and a page the school takes down stays until its row is removed. The portal's documents are the same, and are dated by when they were fetched rather than when they changed.

A published deck's words that sit only inside its images - a title drawn as a graphic, a photographed poster - are not in the corpus. Reading those slides' images with a vision model would bring them in.

A reply keeps the message it quotes, so the corpus holds that text more than once. The search hides the repetition rather than the corpus avoiding it; cutting the quoted chain off at import would make the corpus smaller and the embeddings sharper.
