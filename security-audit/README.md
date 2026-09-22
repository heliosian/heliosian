# Security audit

What an audit of this codebase looks for, what it leaves alone, and how its findings are kept.

## The system

One Go binary serves every app, picked by hostname, behind Google sign-in restricted to the school's Workspace domain, with the server's own HMAC-signed session cookie on the tier-wide domain. The data is the community's: families, children, addresses, photos, mail, invitations. It lives in Google Sheets and Cloud Storage buckets read as one service account, is held in memory, and is served to frameworkless JavaScript. A few paths answer without a session: `web/public/`, the sign-in exchange, `/hooks/` (callbacks checked by the caller's secret), `/open/` and `/ext/` (addresses whose token is the whole of the check). `docs/dev.md` and `docs/deploy.md` describe the whole shape.

The people to think about:

- anyone on the internet, holding nothing
- someone holding one `/open/` or `/ext/` link - a calendar app, a chat app's link preview, a guest invited from outside, whoever the link was forwarded to
- someone who can send mail to an address the service receives at, or post to a hook
- a signed-in member who is not an admin - a parent, a staff member, a student - including one who has left the school since they signed in
- an admin of one app, who is nobody in the others
- a person or a service whose text ends up in front of Claude: a member's question, a sender's mail, a page of the school's website

The lists below say where to look, not what is wrong. None of their lines is a claim about the code.

## In scope

### The browser environment

- Script injection through anything a person typed or a sheet, a mail, Claude or another service supplied, and each place it reaches the page: `innerHTML`, attributes, `href` and `src` (a `javascript:` or `data:` address in a link an admin or a sheet supplied), rendered markdown, Claude's streamed answers, mail shown as HTML, names and titles in the toolbar every app shares.
- Cross-site request forgery on anything that changes state, the sign-in exchange and sign-out included, and what stands against it: the cookie's `SameSite`, an origin check, a token. `GET` routes that change something.
- Cookie attributes and scope: `Secure`, `HttpOnly`, `SameSite`, lifetime, and the domain. The session and the spoof cookie are domain-wide, so every app's origin carries them; check what production's cookies are sent to - development runs under `heliosiandev.com`, a domain of its own resolving to the reader's own machine, and nothing under `heliosian.com` may point anywhere but production.
- What one app's origin can do to another's: a script injection in the least careful app is one in all of them if the cookie and the APIs are shared. Whether an app's routes answer under another app's hostname.
- What is served back under the service's own origin that someone uploaded or imported: content type, sniffing, SVG and HTML as images, `Content-Disposition`, whether media could come from an origin that carries no session.
- Framing and clickjacking; a content security policy that the frameworkless pages could carry as they are; `target="_blank"` and what the opened page can reach.
- Open redirects: the login page's return address, the portal's `Redirects` tab, anything that takes a `next` or a `url`.
- What leaves in a `Referer`, to the map tiles or any other third party, from a page whose address is itself a secret (`/ext/{token}`, `/open/…`).
- What the browser keeps: `localStorage` and what the installed app caches, on a shared family computer, after sign-out, and across Spoof Mode.
- What loads from a third party at all, and what that script could then read.
- Cache headers on signed-in responses and on `/open/` responses.

### The auth integration

- How the Google credential is verified: signature and key rotation, audience, issuer, expiry, the hosted domain claim rather than the address's suffix, `email_verified`, the sign-in exchange's own double-submit token.
- The session cookie: what it signs, how the signature is compared, its lifetime, and whether there is any way to end one - someone who leaves the directory, a lost phone, a rotated key as the only lever.
- The member gate: who counts as a member, when that is re-checked, and what someone the directory no longer lists can still reach.
- Identity resolution: `Email Aliases`, `Name to Email` and the actor resolution the directory and the portal do - whether an alias or a name match can make one person another, and who can write those tabs.
- Parents answering for students, hosts for parties, managers for groups: each delegated right, checked on the server and not by the page.
- `auth.Public`: every path it lets through, and whether each does its own check. A new route under `/hooks/`, `/open/` or `/ext/` is public by its prefix alone.
- Hook secrets: how each is compared, whether a signed request can be replayed (Mailgun's timestamp and token), what the calendar watch's channel token proves, and what a hook does before it has checked.
- `/open/` and `/ext/` tokens: how long, how random, from which source, what each one opens, whether it can be revoked or re-issued, whether it outlives what it was for, whether it is logged, and whether one kind of token is accepted where another is expected.
- Admin and super admin: how each is decided, that every handler that needs one asks on the server, that an app's admin is not an admin elsewhere, and that a regular admin cannot make themselves more (`docs/config.md`).
- Spoof Mode (`internal/auth/spoof.go`, `docs/toolbar.md`): who can start one, that a spoofed request can never reach more than the person viewed as, that writes made while spoofing are recorded against the admin, that mail and invitations sent while spoofing say who really sent them, and that the spoof cookie cannot be minted or kept by someone who is no longer a super admin.
- The sample server's sign-in (`auth.Fixed`, `cmd/startserver`, `cmd/cookie`): that nothing of it is reachable in the production binary, by a flag, an environment variable or a hostname.
- Host routing: the `Host` header as the app selector, and anything built from it - links in mail, redirect targets, the login URI.

### Server data handling

- Ids and paths from the request used to reach a row, an object or a file: the file-serving order in `docs/dev.md` (`web/public/<app>/<path>` and after) against traversal and against serving a signed-in file before sign-in; bucket object names built from request input; row keys that reach another family's row.
- Sizes and counts with no bound: request bodies, uploads, image dimensions before decoding, mail and attachments taken in through hooks, the number of recipients one action mails, the length of what goes to Claude.
- Anything a member can do that spends money or sends mail without a bound the owner chose: Claude, Places Autocomplete, geocoding, Vertex AI embeddings, Mailgun. The limits Helios Ask names in its own constants are the model; look for the paths that have none.
- Server-side requests to addresses a person supplied: the image import (that the three stock libraries and Wikimedia Commons are the whole of it, redirects included), link previews, anything that fetches what a sheet names.
- Mail taken in: who a message is really from before Loop decides whether they may post (SPF, DKIM and DMARC results, not the `From` line), header injection on the way back out, loops and amplification through groups that contain each other or an address that answers automatically, what a calendar reply can change and for whom, attachments and HTML kept and later shown.
- Mail sent: CRLF and header injection through names, titles and addresses; escaping in the HTML templates; text an outsider chose going out under the service's name; the `.ics` an invite carries, and what an unescaped title or place can add to it.
- Values written to a sheet that a spreadsheet would run as a formula when an admin opens it, and values read from a sheet trusted as if the server had written them.
- Text handed to Claude that a member or an outside sender wrote, and what the tools it can call would then do. The tools are read-only; the question is whose eyes they read with, and what an injected instruction can make an answer say, link to or draw.
- What the triage queue files on GitHub: a member's text in a public repository's issue, mentions and markdown included, and what of the reporter goes with it.
- Errors and logs that carry what they should not: tokens in logged URLs, credentials, message bodies, a stack trace in a response.
- Concurrency only where it is a security matter: a check and the write it guards on different sides of the write queue.

### Storage and integrations

- How each secret reaches the process and where else it ends up: pages, logs, the image, the build's uploaded source, the repository, `local/`, the log file and the minted cookie `cmd/startserver --detach` leaves behind, the capture browser's profile holding a real session, Chrome's debugging port while it runs.
- Keys rendered into pages (the Maps browser key) and how each is restricted; server keys that would work from anywhere if they leaked.
- One key doing every job: the Mailgun key across all domains, the GitHub App's installed permissions against what filing an issue needs, the Anthropic key shared by the service and the job.
- Buckets and sheets: that nothing is readable by address alone, what `directory@` can do beyond what the server needs, who can act as it (`serviceAccountTokenCreator`), and who is in the shared drive - since whoever can edit an `Admins` or `Super Admins` tab by hand is an admin.
- The deploy path as an integration: a push to `main` is production, so who can push, what the build's service account can do, and what the GitHub connection grants are part of the system.
- The real-data dev server: real community data on a laptop, the blob cache under `local/cache/blobs/`, and the rule that nothing else is written to disk.
- What each outside service is sent, and whether it needs all of it: addresses to Places and the geocoder, questions and directory data to Anthropic, documents to Vertex AI, recipients and bodies to Mailgun, reports to GitHub, search words to the stock libraries.
- What is kept for ever that need not be: received mail whole in the mail bucket, conversations, change logs with the old values in them, bounced addresses.

### Defense in depth that is missing and cheap

A second check that costs a few lines where one check stands alone today. The test is whether it fits the code as it is without a new moving part:

- a response header set once in `internal/app` for every app - `nosniff`, a frame rule, a referrer policy, a content security policy the pages already satisfy
- a cookie attribute, a narrower cookie domain or path, a `__Host-` or `__Secure-` prefix
- a constant-time compare, a bound on a size, a timeout on an outbound request, an allowlist where there is a denylist
- a visibility check in the owning package rather than in each caller, so the next caller cannot forget it (`docs/dev.md`, The rules live with the thing)
- a token that names what it is for, so it cannot be used as another kind
- a default that denies: a route that is private unless listed, a field that is left out unless named
- a startup refusal for a configuration that would be unsafe, in the manner the server already refuses a missing input

### What a non-admin can read

- The model and every API response as a member receives them, not as the page draws them: fields sent and then hidden by script, whole models sent where a page needs a slice.
- Children's data and family details beyond what the consent form allowed (`Preferences`), in every place they surface: the directory, the map, photos, lists, invitations, Helios Ask.
- Photos and media by name: whether an object's address is enough for any member once known, consent or not, and after the sheet has stopped naming it.
- Guest lists, replies, tickets, waitlists, sign-ups and who declined, for events a person is not part of.
- Mail a person is not a reader of (`loop.Group.MailReadableBy`), group membership, and the rules that decide it.
- Helios Ask as a side door: its tools and its document search must read with the asker's eyes - the same `VisibleTo` the pages call, and `MailReadableBy` for a Loop group's mail, which has a membership to hold it to. Its other documents are open to every asker on purpose: the forwarding hook files mail only when it went to everyone or to a whole class (`docs/ask/artifacts.md`, What is in it and what is not), what went to one class is not secret from another, and nothing is kept from students. The question for those is what gets filed, not who reads it.
- What differs by kind of member: whether a student, a parent and a staff member are meant to see the same things, and whether the server knows the difference.
- Admin-only state reaching everyone: `/api/config` and its super admin list, feedback reports, change logs, bounce lists, other people's feed addresses.
- Anything reachable by changing an id in a request.
- What a signed-out fetcher gets: share cards and link previews at `/open/share/`, the outside guest's page at `/ext/`, a feed's contents - each asked with no viewer, so each should show only what everyone may see.

`docs/dev.md`, The rules live with the thing, names the visibility function each package owns; a handler that answers without asking it is a finding.

## Out of scope

- **Checklists and compliance** - no OWASP line items for their own sake, no scanner output, no header or TLS grades, no dependency-age reports, no policy, paperwork or legal reading (FERPA, COPPA, GDPR). A missing header is a finding only with the attack it would have stopped. A finding names something a person can do here that they should not be able to.
- **Phishing and human exploit chains** - anything that starts with talking a member or an admin into something. The exception is the egregious: the service itself making the lie easy, such as mail it sends in a member's name with text an outsider chose, or a link on its own domain that lands somewhere else.
- **Improvements that cost more than they buy** - anything needing a new service, a new store, a framework, a key management scheme, a WAF, per-user encryption or a rewrite to close a gap that small. If the fix would be the most complicated thing in the package, it is not a finding.
- **An attacker who has already won** - someone holding the service account, the session key, a super admin's Google account, project owner, push access to `main`, or a member's unlocked phone. What such a person can do is not a finding; how someone lesser becomes one is.
- **Trusted people doing their job** - a super admin reading everything, Spoof Mode existing, an app's admin editing that app's data, a content manager of the shared drive editing a sheet by hand. What Spoof Mode records and what a hand edit can make the server do remain in scope.
- **The directory being a directory** - a member looking up another family is the product. Scraping by a member is in scope only where the server hands over more than the pages need.
- **Volume** - flooding a single-instance service with requests. In scope instead: one cheap request that costs a great deal of memory, money or mail.
- **The platforms underneath** - bugs in Google sign-in, Cloud Run, Sheets, Mailgun, Anthropic, the browser, Go or its standard library. How this code uses them is in scope; they are not.
- **Dev tooling for its own sake** - `cmd/` tools and the sample server run by one developer on their own machine, except where one leaves real data or a credential behind, or where sample-mode behaviour could be reached in production.
- **Theory without a path** - a primitive that is unfashionable but unbroken as used here, a timing difference nobody could measure through Cloud Run's front end, version strings, anything that ends in "could potentially".
- **Bugs that are only bugs** - wrong behaviour with no security consequence goes to the ordinary issue tracker.

## Findings

One file per finding under `security-audit/findings/`, copied from `security-audit/TEMPLATE.md` and named for what it is about in a few lowercase words joined by hyphens (`open-feed-token-never-expires.md`). A file is:

    Description: one sentence
    Status: open
    Severity: medium
    ---
    brief details

- **Description** - one sentence saying what is wrong, where, and who (anyone, a link holder, a member, an admin) can do what because of it.
- **Status** - `open` when written; `revisit` when it is true but waits on something before it can be worked on, with what it waits on in the details; `fixed` once a change closes it; `wontfix` when it is understood and left, with the reason in the details; `invalid` when it turns out not to be one, with the reason in the details.
- **Severity** - how bad it is while it stands, judged by who can do it and what they get, and kept as the finding's status changes. `critical`: anyone on the internet, or a link holder, reads or changes the community's data at large, or takes a credential. `high`: a signed-in account or a mail sender reads what consent or the rules keep from them across the community, acts as someone else, or takes the service down. `medium`: a leak or a write kept to one app, one group or one kind of record, money or mail spent without a bound, or something that needs a further condition to bite. `low`: a nuisance, a narrow leak, a second check that is missing where the first still stands.
- **Details**, after the `---` - brief: the files and lines, how to see it, and the fix if one is plain. A few short paragraphs at most.

When a finding was written and when it last changed come from git, so a file carries no dates. A finding that stops being true is edited, not deleted, so the record of what was looked at stays.

`go run ./cmd/findings` lists them: `--status open,wontfix` keeps those statuses, `--text token` keeps findings with those words anywhere in the file, `--since 2026-01-31` keeps what changed on or after that day. Filters combine. `--first` keeps one of what is left, the next to work on: the open finding first by file name among those at the highest severity any open one has, so the answer holds still until that finding's status changes; a `revisit` finding is passed over unless `--revisit` is given, which counts it as open. `--stat` prints, in place of the list, a coloured grid of how many of the kept findings stand at each severity and status, with totals.
