# The community's documents

Helios Ask reads more than the apps. It reads the community's documents - every weekly newsletter, every announcement to all the families, everything each classroom's parent list has carried since 2018, the pages of the school's website, and the parent portal's school-wide and classroom pages with the documents they link - through two tools that search them and read one whole (`data.md`, The tools). It is the community's memory: how camping has been run each year, what the nut policy says, when the school moved to Veracross, how the school describes its program, who organised a thing last time. To the model they are all documents: the tools say nothing of where one came from, and give a page's address so it can be linked. The package is `internal/artifacts`; this file carries what reading it cannot tell you.

## What is in it and what is not

Only mail that went to everyone or to a whole class is in it: the school's newsletter under each mailer it has used, the announcement lists (`parentsandstaff`, `community`, `parents`, `parentsonly`, `parentsandstudents`), the chat everyone is on, each classroom's parents and students, each grade band, and the reading and maths groups a teacher writes to. The channel a message came through is kept with it and can be searched on its own.

Nothing a committee or a role kept to itself is in it. The board and each of its committees, the HCA's teams, the room parents, the staff, admissions and the registrar are withheld by name, and a list whose name begins like one of theirs is withheld with them. Helios Ask answers whoever asks it, so anything sent to a few people on behalf of the rest would be the whole community reading a committee's post. Mail addressed *to* `hca@heliosschool.org` is out for the same reason: it is what people write to the association, not what the association sent out.

`Channel` in `internal/artifacts` is that judgement, and the only place it is written down: given a message's list header, its sender and its recipients it answers with the channel and the kind, or that the message is no part of the corpus. Every message goes through it, so a message arriving later is held to the same rule as the ones already on file, and `Build` refuses one that is not the community's.

Every page of the website that its sitemap names is in it, less three kinds. The staff directory and the calendar are left out because Helios Who? and the calendar carry them in their own structure. Pages that are not the school's words are left out: Finalsite's template instructions and widgets, and an ad and a test copy the site still publishes. So are pages of no use to a reader: login, search, the site maps, the 404 page and the website's privacy and accessibility statements. `excluded` in `internal/artifacts/page.go` is that list; the archive does not fetch those pages and `BuildPage` refuses them.

From the Veracross parent portal, only what went to every family or to a whole class is in it: the pages under its Information for Parents, School Programs and Classroom Pages menus, the HELP site (the Helios Everything Library Page, a Google Site of the curriculum by grade band) and the Google Docs, Slides and Drive files those pages link. Nothing under a student's record is in it, nor messages, billing, the family profile, the directory, the calendars or the health portal. Linked spreadsheets are left out, since they are sign-up sheets with families' names, and so is any link whose words name a class Lookbook or say "Meet Your Classmates", the decks of students' photos.

## Where it lives

A document is one message, one page or one portal resource, kept as markdown and cut into chunks, each chunk with the vector an embedding model gives it. The bucket holds the documents, the `Artifacts` spreadsheet is their index, and the server holds every chunk and vector in memory, so a search is one embedding call and a scan.

The `Artifacts` spreadsheet lives in the community shared drive beside the others and `ARTIFACTS_SHEET` names it. One tab, `Documents`, one row per document: Key, Title, Date, Author, Kind, Channel, Source, Chunks, Object. The key is the SHA-256 of the message id, which is the one name a message carries wherever it was delivered, so the same message reaching two of the accounts is one document; a page's or a portal resource's is the SHA-256 of its address. Source is `mail:` and the message id for a message, and the address for a page or a portal resource. A page's kind is `page` and its channel `website`; a portal resource's kind and channel are both `portal`. Object is the bucket name, `artifacts/<key>-<fingerprint>.json`, where the fingerprint is a hash of the document as rendered - its markdown, its chunks and the embedding model's name. A change to the converter, the chunker or the model therefore gives every message a new object name, which is how the import knows what is stale and the server knows what it need not read again.

The object is one JSON file: the row's fields, the whole markdown, and the chunks, each with its section path and its vector. A vector is written as base64 little-endian float32 rather than as decimal text, which is the difference between thirty megabytes of corpus and half a gigabyte of it. The width asked for is a few hundred of the model's three thousand dimensions, which the model nests so that a prefix is itself a good vector; at this corpus's size it retrieves as well and costs a quarter of the bytes to keep, fetch and hold. A document embedded by a different model or at a different width refuses the load and says to import it again, since vectors of two widths cannot be compared.

The objects sit in the media bucket under `artifacts/`, and no route serves them: they reach a browser only as the words Claude writes.

## From a message to a document

A message on its way in is a `Message`: its id, subject, date, sender, recipients, list header and its text, as HTML or as plain text. The corpus that is on file was read out of the mailbox once over JMAP, which is done and gone; what comes next comes in by hook, the way Helios Loop takes the groups' mail (`docs/loop/data.md`, Mail), and lands as the same `Message` for the same `Build` to render. A message saved before a hook exists sits as one JSON file under `imports/mail/`, which is gitignored: the headers and the text, not the whole message, since attachments are most of a mailbox's bytes and none of its words.

The id is what a document is keyed by, so a message that arrives twice is one document however it got here. An issue of the newsletter is a separate message to each address, so it is known by its title and day as well.

`cmd/importartifacts` turns saved messages into documents:

    eval "$(go run ./cmd/findsheet)"
    go run ./cmd/importartifacts --dry-run imports/mail/*.json
    go run ./cmd/importartifacts --i-have-user-permission-to-spend-money imports/mail/*.json

Every message is read and rendered before anything is embedded, so what a run would spend is known before it spends it; `--dry-run` reports that and writes nothing, and printing one message's markdown is what naming a single file does. Embedding costs real money, so a run that writes refuses to start without `--i-have-user-permission-to-spend-money`, as the periodic sync does.

For each message the HTML is rendered to markdown by the package's own walker, or the plain text is taken as markdown when that is all the message has, its hard wrapping joined back into paragraphs. Headings become `#` headings, and because one of the mailers sets its section banners as bold capitals in a paragraph of their own rather than as headings, a block that is nothing but bold capitals becomes a heading too. Paragraphs and layout cells become paragraphs, list items become `- ` items, links keep their address, and images, scripts and styles are dropped. Inline markup that turns out to wrap whole blocks - a mail template puts links around tables - keeps its words and gives up the wrapping. What a mailing list adds around a message is cut off: the subscription footer with its unsubscribe address, the link to the thread on the web, and the confidentiality notice a mail client puts under a signature. A message that reads as nothing but that footer has no words to index, and one already on file as its footer is taken off the corpus.

Every link a newsletter carries is one of the mailers' redirects and carries the address of the person it was sent to, so none may be handed to a reader. Each is turned back into the address it points at: the current mailer writes that address into the link itself, where it can be read with no request at all, and the former one keeps it, so its links are followed - all of a page's at once, and only the one kind of its redirects that goes anywhere, never the unsubscribe or view-in-browser ones. A link that cannot be turned back keeps its words and loses its address. Marks the mailer added to count clicks are left off the address.

## From a page to a document

`cmd/archivesite` reads the sitemap Finalsite publishes at `/fs/pages/sitemap` and saves each page it names under `imports/site/` as a `Page`: the address the request ended at and the whole HTML. A page that redirects off the site (the parent portal, the inquiry form, social media) is not saved. Several sitemap entries can redirect to the same page, which is saved once under its own address. The archiver waits five seconds between requests, as the site's robots.txt asks, and sends an Accept header, since without one the site answers 406.

    go run ./cmd/archivesite
    eval "$(go run ./cmd/findsheet)"
    go run ./cmd/importartifacts --dry-run imports/site/*.json

`cmd/importartifacts` takes saved pages and saved messages alike; a file with a `url` is a page. A page's document is its `<main>` content without what the site repeats around it. The page title is dropped from the text because it is the document's title (the `<title>` less the site's name). The section menus and forms are dropped. So is the row of tabs over a set of panels, since each panel's title appears again as its heading. Relative links become absolute against the page's address. The document is dated by the day the site last published the page (`page-published`), its author is Helios School, and its source is its address, which the tools give the model as the document's url.

## From the portal to a document

The portal and the HELP site sit behind the school's sign-in and draw most of their pages in the browser, so `cmd/archiveportal` reads them through the capture browser (`docs/screenshots.md`) with a parent signed in, in a tab of its own. It follows the portal's menus and HELP's, waits for each page to stop changing, and saves the content as a `Resource` under `imports/portal/`: the address, the title the menu gives it, the page that linked it, when it was fetched, and the body as HTML or as text. A Google Doc is exported as HTML from a page on the document's own origin, since Google answers the signed-in export only to the browser holding the session. A deck is exported as text and a Drive PDF as its text through `pdftotext`, both downloaded by the browser into a temporary folder it is pointed at for the run and pointed away from after. A HELP page embeds its decks by their real addresses, which are what it exports; a deck linked only by its published address carries its words as pictures and is skipped. The site notices a reader who comes through too often, so the archive is run rarely and the import works from what it saved.

    go run ./cmd/capturebrowser
    go run ./cmd/archiveportal
    eval "$(go run ./cmd/findsheet)"
    go run ./cmd/importartifacts --dry-run imports/portal/*.json

`cmd/importartifacts` knows a resource by its `format`. HTML is rendered by the same walker as mail, with relative links made whole against the resource's address and Google's `google.com/url?q=` wrappers turned back into the address they carry. Text keeps its lines, a page break becomes a paragraph break, and a line that is only a number, a deck's slide number, is dropped. A resource is dated by the day it was fetched, since the portal says nothing of when a page last changed, its author is Helios School, and its source is its address, which the tools give the model as the document's url.

## Chunks

The markdown is cut at its headings: each heading opens a chunk named by the path of headings above it (`ANNOUNCEMENTS & REMINDERS › Picture Day`), a heading in mixed case sits inside the capitals heading before it, the words before the first heading are a chunk of their own, and a section longer than six thousand characters is cut again at paragraph breaks, since the embedding model reads at most about two thousand tokens at a time. What is embedded is not the chunk alone but the document's title, its date in words and its channel over the chunk's section, so a passage saying only "sign up here" is still found by what it is about.

The chunks go to Gemini's embedding model on Vertex AI, twenty-five to a request, as retrieval documents; a question is embedded the same way as a retrieval query. Documents are then written to the bucket and their rows to the sheet two hundred at a time, several at once, so a run that fails part way has recorded all but its last batch. A message already on file under the same object name is skipped; one on file under an older rendering is imported again, its row updated in place and the object it superseded deleted, so the bucket keeps nothing nobody names.

## Searching

`search_documents` embeds the question and scores every chunk: the dot product of the normalised vectors, plus a small share for the words of the question the chunk itself holds, since names, dates and numbers are where an embedding alone is weakest. The newer document wins a tie. A reply quotes the message it answers, so the same words are in the corpus many times over; a passage already covered by a better-scoring one - inside it, or holding it - is left out, so the answer's room goes on different things rather than the same thing said again. The search narrows to a range of dates, and says what dates are on file rather than answering from everything when it cannot narrow as asked. Each passage carries its document's title, date, age against today in the same words the calendar tools use, its section, and for a page its address. Nothing else about where a document came from reaches the model. The kind, the channel and the author stay on file for the import and the probe, so the model calls everything a document and links the ones it can. `read_document` is one document whole, by the key a passage carries, with its address when it has one.

`cmd/artifactsprobe` runs the same load and the same search from a laptop, printing the top passages with their scores, so a question's retrieval can be read before Claude sees it.

## Loading it

The server reads the sheet every five minutes like every other model. An object's name carries a fingerprint of the document inside it, so a name the last load already read is the same document: only what is new or has changed is fetched, and an ordinary refresh costs one read of the sheet. What it does fetch it fetches many at a time, since thousands of objects read one after another would take longer than the service is given to start. The objects go through the media store's disk cache like every other fetch, so a dev server or a tool run over the corpus pays for the download once; the bytes are parsed into the model and not held a second time.

## What it needs

- The `Artifacts` spreadsheet in the shared drive with its `Documents` tab (`cmd/createsheet --title Artifacts`, then `cmd/createtabs`), named by `ARTIFACTS_SHEET` wherever the server runs (`docs/deploy.md`, Configuration values).
- The media bucket, which the server reads and the importer writes and deletes in as `directory@`.
- Vertex AI: the Vertex AI API on in the project, and `roles/aiplatform.user` on it for `directory@` (`docs/deploy.md`, IAM). Local runs act as `directory@` through the impersonated application-default credentials every tool here uses, so nothing more is set up on a laptop.

## Sample and tests

Sample mode has no bucket and no sheet: `sampledata/artifacts/` holds a few fictional messages and one fictional page in the same forms the fetch and the archive save, and the sample server renders and embeds them at startup with a fake embedder, a hashed bag of words, so the search tool runs and finds the right passage without asking anything of anyone. The tests in `internal/artifacts` pin which channel each kind of message belongs to and which are withheld, the markdown a newsletter renders to, a plain-text message's paragraphs, the chunks each cuts into, the cut of a long section, bold capitals becoming headings, inline markup around blocks keeping its words, the footers and notices coming off, which links count as the mailers' and what becomes of each kind, a page's markdown without the site's menus and tabs and with its links made whole, the excluded pages being refused, a portal resource's links made whole and unwrapped, a text resource keeping its lines without its slide numbers, a saved file read as a message, a page or a resource by what it carries, vectors surviving base64, the object name following the rendering, the search's ranking and its narrowing by date, and a quoted passage being returned once. The ones in `internal/ask` pin the tools' answers for the sample parent, a page's passage carrying its address, and no passage carrying where it came from.

## What is left to do

Mail arriving from here on needs the hook that takes it: a route that reads a message, puts it through `Channel` and `Build`, embeds it and writes it, which is the serving side of what `cmd/importartifacts` does by hand.

The website's pages are as fresh as the last archive: a page the school edits changes in the corpus only when `cmd/archivesite` and the import are run again, and a page the school takes down stays until its row is removed. The portal's documents are the same, and are dated by when they were fetched rather than when they changed.

HELP links fourteen school-wide decks - Camping, Imagine Lab, Student Council, Wilderness Village, Art, PE, Passion Projects, Library and more - only by their published addresses, whose words are pictures. They are out of the corpus until their source decks are found or their slides are read as images.

A reply keeps the message it quotes, so the corpus holds that text more than once. The search hides the repetition rather than the corpus avoiding it; cutting the quoted chain off at import would make the corpus smaller and the embeddings sharper.
