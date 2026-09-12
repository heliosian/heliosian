# Helios Celebrate

Helios Celebrate is the site for the community's fun(d)raiser parties: members of the community host a party - a fondue night, a pickleball mixer, a wine tasting - and families take tickets, which the school invoices later. It replaces the parties part of the Glide site at celebrate.heliosschool.org (whose fund-a-need, raffle and photo pages are not carried over) and serves at celebrate.heliosian.com. The sheet is `Celebrate`, the package is `internal/celebrate`, and the API is `/api/celebrate/`.

## Entities

- **Celebration** - one year's Spring Celebration, the gala the parties raise money around: a code (`SC-2026`), title, theme, when and where, a blurb, a picture, and a button (text and link - Learn More, to the celebration's own site). Parties are filed under one. The one marked Current is the banner across the top of the parties page - its picture as the background under a teal wash, the title and theme, the date and place, and the button - and the others are reachable from the year picker beside the heading. Admins change the banner from Admin Tools › Banner, which previews it and opens the current celebration's editor.
- **Party** - what it is (title, subtitle, a one-line summary for the card, the description, a need-to-know line), who hosts it, a category and an audience line ("Parents & Kids"), when and where, the price and what one ticket covers ("$65 per person"), how many tickets there are (or no limit) and any minimum to hold it, a picture, and its switches (below).
- **Host** - someone who runs a party: they edit it, see who is coming and their contact details, add and remove anyone, and offer waitlist places. A party may have several.
- **Ticket** - one person on a party: someone in the directory by address, or a guest by name - a visiting cousin, a non-Helios sibling, a friend. Each carries who is billed, the price when it was taken, an optional note, and where the invoice stands. A ticket is either sold or a place on the waitlist.

Names, faces, grades and households come from the directory model at request time, like the volunteer portal; the sheet stores only emails. A ticket holder the directory does not know is shown as a guest.

## The switches

Every party has, on its page for whoever runs it and in its editor:

- **Tickets on sale** - off, the party is listed but sells nothing (the old site's Allow Tickets).
- **Waitlist when full** - a full party takes names in order; off, it shows Sold Out.
- **Parents / Students / Staff** - who may hold a ticket. Someone who is both a parent and staff fits if either does; a guest fits wherever anyone does, since the host reads the name. At least one must be on.
- **Drop-off** - a child may come without a parent.
- **Parent ticket required** - a parent who stays needs a ticket of their own.

## Availability

What a party's ticket button says follows from the row and the clock: **past** once its end (or start, with no end) is behind; **closed** while it is not Open or sales are off; **available** while tickets remain; **waitlist** when full and taking names; **sold out** when full without one. A party with no cap is never full.

## Who may do what

- **Anyone signed in** browses every open party and sees who is coming (names and faces, as the old site did after sign-in). They take tickets for themselves and their household - the partner and children the directory lists with them - and guests by name, billed to an adult in the family (a student's tickets go to a parent). While there is room each is sold; then each goes to the waitlist, or the whole purchase is refused when the party has none. Nobody holds two tickets to one party. A sold ticket stays sold - it is a fundraiser, and the old site's FAQ said the same: no refunds, resell it to another family - so a family cannot give one back; they can leave a waitlist, which cost nothing. Whoever runs the party removes anyone.
- **Anyone signed in** posts a party with Host a Party. It lands Pending, with them as its host, until an admin opens it; they can edit it meanwhile and it shows on their Hosting page.
- **Hosts** edit their party (everything but its status and year), add and remove hosts (never themselves), flip the switches on the party page, open any attendee's ticket, add anyone from the directory billed to anyone, and move a name off the waitlist onto a ticket when a place opens up - the waitlist is offered by hand, never promoted on its own, since a place taken up automatically is one the family might no longer want. A host is never refused for room.
- **Admins**, the Admins tab plus the platform super admins (`docs/config.md`), approve, hide and delete parties (an empty one only), edit any, record where each ticket's invoice stands, manage celebrations, categories, settings and the admin list from `/admin`, and download the invoice list.

## Pages

One shell, `web/celebrate/index.html`, routes every page on the client: a left rail (Parties with its three tabs and their counts under it, My Family's Parties with the household under it - Me, then the partner and children, each with how many parties they hold a ticket to - Hosting, Approval Needed for admins, and Host a Party) beside the shared top bar (`docs/toolbar.md`), whose search filters the parties list and carries the words there from any other page.

- **Parties** (`/`, or `/celebrations/{code}` for another year) - the current celebration in a band across the top, then the parties in three tabs as the old site had them: Available, Waitlist, and All Parties (upcoming, then past), with a category filter. A card is the picture with its audience chip, the date and time, the title and summary, and the availability with what is left.
- **Party** (`/parties/{id}`, or `/p/{pretty}` when it has a friendly address) - laid out as HCA-Team lays out an event: the wide banner with the date stamp floating in its corner and, opposite, the round tools (the pencil for whoever runs it, copy link, view full size); then the category and audience chips, the title, the description, and the need-to-know line in the tinted callout behind a megaphone, beside a rail of cards - Date & Time with Add to Calendar; Where, the place in words with the street address (signed-in members only) under it as a map link; the price and what is left, the hosts by face; the flyer, shown whole and opened full size on a click (hosts upload or replace it right there); and Questions?, a mail to the hosts. Under the words, the ticket band (Get Tickets or Join the Waitlist, what is left, and the family's own tickets - a waitlist place with a way to leave it), then Who's Coming as a grid of faces each placed by a line - a student's grade, "Parent to Sam Whitfield (Grade 3)", a staff member's job, "Guest of Jordan Whitfield" - then the waitlist in order. A host's face, and each face on the grid, opens the person's card as HCA-Team opens a volunteer's: the big face, the name with pronouns and what places them, buttons to email, text, call or copy them, the contact rows and the household as chips, and the way through to their Helios Who? page (a guest by name gets the card with just that). Whoever runs the party gets, from an attendee's face, the same card with a second tab, Ticket - who is billed, the price, the note, the waitlist offer, and for an admin where the invoice stands - and, above the grid, the attendee contact list: every ticket with how to reach the person (a child through the parents' addresses, a guest through whoever bought the ticket), who is billed, the status and the note, with one click to copy the addresses and one to copy the table for a spreadsheet. Below sits the band of switches with Edit Party and, for an admin, Approve / Hide.
- **My Family's Parties** (`/my`) - one section per member of the household with the parties they hold a ticket to or wait for, and one for the guests the family has brought, upcoming first; `/my/{name}` - the address's local part, `/my/ella.whitfield` - is one member's alone, from the rail.
- **Hosting** (`/hosting`) - the parties the viewer runs, with counts and money raised; for an admin, also Approval Needed and All Parties.
- **Admin Tools** (`/admin`) - in the admin chrome every app shares: the banner (a preview and Edit Banner), celebrations, categories (renamable, reorderable), settings (the intro under the heading and the note on the ticket form), invoicing (every ticket by purchaser with totals, each openable to mark its invoice sent or paid, and a CSV of the same), and the admin list.

The ticket form asks who to bill (an adult of the household, by face), who is coming (the household by face, with anyone who already holds a ticket or whom the party's audience rules keep out shown greyed and saying why), guests by name, and a note, and tallies the price as it goes, saying how many will land on the waitlist. Hosts and admins get a directory picker in the same form for anyone else.

On phones the rail becomes a drawer behind the hamburger, a slim title strip sits under the shared bar, and a bottom tab bar holds Parties, My Family's Parties and Hosting.

## Editing

As in the other apps: in place, through modals and one-click switches, with every change applied to the in-memory model first and then written to the sheet through the shared write queue and appended to the Change Log. A change the rules reject is refused before anything is written. Party images are uploaded, found in the image libraries (`internal/imagesearch`), or cropped, like the volunteer portal's.

## Friendly addresses

A party may carry a `Pretty ID`: `fondue` puts it at `celebrate.heliosian.com/p/fondue`, and every link to it and the copy button use that address. Lower-case letters, digits and hyphens, at most 40; one address for one party across every celebration - saving refuses one another party holds. Changing or removing one writes a row to `Redirects`, so the old address keeps working and the browser's address bar is corrected to the live one; the bare `/parties/{id}` always resolves too. Hosts set it in the editor's Basics tab.

## Sign-in and install

A link to a party previews in chat apps and social feeds even though the site is behind sign-in: the sign-in page served at a party's address carries the party's Open Graph tags (title; the day and hours, the place in words and the summary as the description; the canonical address), and `/share/{id}.png` is a public 1200x630 card drawn on the server by `internal/sharecard`, laid out like HCA-Team's: on the pale wash, the mark and "Helios Celebrate", the title in deep teal with the yellow swoosh, then the day behind a calendar glyph, the hours behind a clock and the place behind a pin; on the right the party's flyer shown whole, or without one its banner scaled to cover the panel. Only an open party previews; a pending or hidden one, or any other page, gets the plain sign-in page and no card. The tags say what a poster on the wall would say - never the street address, never who is coming.

Everything sits behind Google sign-in restricted to the school domain, sharing a session with the other apps on the same tier. It installs to a home screen with the sun-over-the-hills mark under `web/public/celebrate/brand/` (cut from the exports in `~/Dropbox/Kids/Heliosian/images/celebrate/`: the app icon square for the home-screen icons, the bare mark for the favicons, the app switch and the maskables, the vertical lockups for the rail and the login page); there is no splash battery yet.

## Not carried over

The old site's fund-a-need items, raffles, evening agenda, FAQ and photo galleries are out of scope for now, as is email: nobody is mailed when a ticket is taken or a place offered.
