# Data model

The tabs, columns, and validation rules are in `internal/celebrate`; this file carries only what reading that code cannot tell you.

The site's data lives in one Google Sheet, `Celebrate`, in the community shared drive, reached through drive membership like the other sheets. `CELEBRATE_SHEET` names it, and `cmd/createtabs` reads that variable to lay it out from an empty spreadsheet titled `Celebrate`.

## Tabs

- `Celebrations` - Code, Title, Subtitle, Start, End, Location, Address, Description, Image, Button Text, Button URL, Current, Banner. One row per year's gala. Two Yes/No flags, each on at most one row: `Current` says whose parties the parties page lists first; `Banner` says which one the band across the top advertises - next year's gala over this season's parties, say - and blank everywhere means the current one. The banner is its picture as the background, its theme small over its title large, the date and place, and a button (both cells or neither): Button URL is a web address, or the word `calendar` for a Save the Date that adds the gala - title and theme, date, place - to the reader's Google Calendar (it needs a Start). Code is short and unique (`SC-2026`, the old site's event codes); parties name it. Current is Yes on one row at most; with none, the latest by Start leads.
- `Categories` - Title. Row order is the filter's order. A party names one by title; a rename carries every party along.
- `Parties` - Party ID, Celebration, Title, Subtitle, Summary, Description, Need To Know, Note Emoji, Note Title, Hosts, Category, Audience, Ticket Unit, Price, Capacity, Minimum, Start, End, Location, Address, Image, Flyer Image, Pretty ID, Status, Tickets, Waitlist, Adults, Students, Drop-Off, Parent Ticket Required, Added By, Added. Hosts is the names as shown ("McDowell and Park/Gulliver Families"); who actually runs it is the Hosts tab. Audience is the words on the card; the two Yes/No columns are the rule (Adults means parents and staff alike). Location is the place in words for everyone ("The Parks' House in Los Altos"); Address is the street address, shown only to signed-in members - the whole site sits behind sign-in today, but the split keeps the street out of anything that might one day be shown before it, such as a share preview. `Image` is the wide banner across the page and the card; `Flyer Image` is the party's poster, shown whole in the page's rail and opened full size on a click - uploaded whole, never cropped or found in a library. Note Emoji and Note Title dress the need-to-know callout on the page (blank means the megaphone and "Good to know"). Ticket Unit is the word after "per" ("person", "adult") or a whole phrase from the old site ("1 ticket per child (parents free)"), which the page shows as typed.
- `Hosts` - Party ID, Email. One row per host per party.
- `Tickets` - Ticket ID, Party ID, Email, Name, Purchaser, Status, Quantity, Price, Note, Added By, Added. A `Ticket` row is one person: Email names someone in the directory (through its aliases); a guest has a Name, and an Email only when the buyer gave one - an address the directory does not know stays a guest. A `Waitlist` row is a family's request: Email and Purchaser are whoever asked, Quantity how many tickets they want (blank means one; a sold ticket is always one), Price the party's price when they asked. Purchaser is who is invoiced. Price on a sold ticket is the party's price when it was taken, so a later change does not reprice it; a host's free ticket is 0, and what a party has raised is the sum of these, never the count times the price. Added is a date and time, which orders the waitlist.
- `Settings` - Key, Value: `Parties Intro`, `Ticket Note`, and `Hosting Open` (Yes or No; blank means Yes - whether anyone may post a party), all optional; saving from Admin Tools appends a key the tab lacks.
- `Admins` - Email.
- `Redirects` - Type, Old, New, Date. Written whenever a party's address changes - a friendly name set, changed or removed - so a link someone kept still works. Old and New are site paths (`/p/fondue`, `/parties/{id}`); a bare word means `/p/{word}`; Type is `Party`. Read at load; a live address always wins over a redirect of the same name, and a chain of renames is followed to its end (`Model.Resolve`).
- `Change Log` - Timestamp, Actor, Action, Kind, Celebration, Party, Title, Email, Details; appended on every change, never read back.
- `INVOICING` - Date, Party Title, Event Code, Purchaser Email, Guest Name, Action, Quantity, Cost, Invoice, Invoice To. The accounting ledger, in the bookkeeper's own layout: the app appends one row per sold ticket the moment it is minted (a purchase, or a waitlist offer) - the date, the party and its celebration's code, who is billed, who the ticket is for, `ADD`, `1`, the cost - and reads it back for Admin Tools › Invoicing, never editing a row; a free ticket bills nobody and is left out. Invoice and Invoice To are theirs to fill in, and are what the app shows as where each ticket stands. Nothing is written when a ticket is reassigned (billing does not move) and there is no REMOVE row, since a sold ticket is never given back.

**Column order does not matter, column names do**, as in the Events sheet: every write places each cell under the column of that name (`Writer.AppendCells`, `Writer.Set`), so columns may be rearranged by hand; a column the app does not read is left alone.

## Keys are ids

Every party has a `Party ID` and every ticket a `Ticket ID`, unique across the sheet, minted by the app (eight characters from a 31-symbol alphabet); rows added by hand need one too, and the loader refuses two rows sharing one. A row with a **blank id is a deleted thing** left in place: it is not loaded, and a host or ticket row naming a party no live row has is skipped the same way. Each load logs how many rows it skipped and why (`Model.Skipped`), so a mistyped id shows up as a count rather than a silently missing row. `Pretty ID` is an optional friendly address, one party per address across the sheet; the loader refuses two rows sharing one. Celebrations and categories are keyed by their code and title; renaming either rewrites every party naming it in one change.

## Status, tickets, and room

A party is `Pending`, `Open`, or `Hidden`, spelled exactly so; Tickets is `Open`, `Closed`, or blank for Open. Capacity is a positive count or blank for no limit (a hand-typed `70.0` reads as 70); whether a party is full is the count of sold tickets against it, never a flag, and the waitlist is the rows whose Status says so, each counting its Quantity. Price is a dollar amount as typed: `65`, `65.0`, `$12.50`.

## Yes and No

Flag cells are `Yes`, `No`, or blank. A blank takes the column's default, chosen so a hand-added row behaves the way most parties do: `Waitlist` and `Adults` default to Yes; `Students`, `Drop-Off` and `Parent Ticket Required` to No. A party allowing nobody refuses the load. The app always writes Yes or No explicitly.

## Dates

Start and End are wall-clock, `2026-09-19 17:00` or `2026-09-19` for a whole day, with no time zone: a party at five is at five at the school. A party is past once its end (or its start with no end; the next day for an all-day one) is behind the school's clock. Added on a party is a date; on a ticket a date and time, `2026-08-22 19:04`.

## Images

An Image or Flyer Image cell is an object under `party-images/` in the media bucket, content-addressed and written by the image picker, or a path to a bundled file under `web/celebrate/` or `web/public/celebrate/`. A name that resolves to neither refuses the load.

## A load either succeeds whole or refuses

The same stance as the other apps: a sheet that breaks a rule does not load, or does not refresh. As with the Events sheet, a failed load does not stop the server: until the first load succeeds every route answers `503` with the loader's reason, the other apps serve normally, and the five-minute refresh brings the site up on its own once the sheet is fixed. A refresh that fails after a good load keeps serving the last good model and logs the reason.

## Why the Glide tables were not kept

The old site kept parties in one wide tab with four contact-email columns and a copied directory, attendees in a tab of names with an `Attending` flag and a separate waitlist tab with an `Approve for Purchase` flag, and invoicing in a third tab written by formulas. The Hosts tab replaces the email columns, one Tickets tab with a Status replaces the attendee and waitlist tabs, the Invoice column replaces the invoicing tab, and the directory lookup replaces the copy.

## Importing the old site

`cmd/celebrateimport` carries the old sheet's two seasons across, once, into the empty `Celebrate` spreadsheet. It reads the old tabs as CSVs dumped into `imports/celebrate/` (which git ignores) - `Parties`, `Attendees`, `Parties Waitlist`, `INVOICING` and `Categories`, each by `cmd/dumptab` from the old spreadsheet, shared with `directory@` - converts them, proves the result loads with the same `BuildModel` the server runs, and only then writes. A dry run (the default) reports the counts and what it skipped and why, and leaves the converted tabs as CSVs under `imports/celebrate/converted/` to look over; `-write` uploads each party's picture from the old site's storage into the bucket under `party-images/` (content-addressed, as the picker names them) and fills the five tabs, refusing unless every one is still empty below its header - so a second run cannot double the rows.

What the conversion decides, where the old sheet did not say:

- Every party and ticket gets a fresh id. A celebration row per event code the parties name (`SC-2025`, `SC-2026`), the latest current, with only its code and title - the rest is filled in by hand. The categories are the Event Types the parties actually use, in the old sheet's order.
- `Show on Website` is Status (`Open` or `Hidden`); `Allow Tickets` is Tickets (`Open` or, blank or false, `Closed`). The per-kind ticket flags arrived with the 2026 season, so a row with none set - all of 2025 - is read the way the site read it then: grown-ups (Parents and Staff both Yes) and children when the audience words name them ("kids", "families", "grades", "ages" and so on). Staff follows `Allow Adult Tickets`, which is the nearest thing the old sheet had.
- The four contact columns are the hosts; the first is Added By. The form timestamp is Added. Glide's long dates ("Saturday, September 19, 2026 at 5:00 PM") keep their time; a bare day stays a day.
- An attendee row is a ticket if it is still `Attending`. Its purchaser is the purchaser column or, where the old lookup left `#N/A`, the attendee's own address; a row with neither is skipped. A named child under the purchaser's own address is a guest by name, since that address was only the way to reach the parent; a child with an address of their own keeps it. The attendee `Date` has no year, which the party's event code supplies.
- The old INVOICING tab comes across as it stands, row for row in the same columns (dates normalized, the Event Code the party's celebration), less the auction items that were never a party; nothing is matched to tickets, since the ledger is now the record of where each invoice stands.
- A waitlist row becomes a `Waitlist` ticket at its own timestamp, unless the same purchaser went on to hold a ticket for the party. `Approve for Purchase` has no counterpart and is dropped.
- Auction items in the INVOICING tab, the party row with no name, and the attendee rows naming no party are skipped and counted.

The Admins, Settings and Redirects tabs are left for hand entry; add at least one admin before the site is useful.

## Sample data

`sampledata/celebrate/` holds one CSV per tab: two celebrations (the current one and last year's), a party the sample parent hosts, an adults-only one with two tickets left, a kids party with drop-off, full parties with and without a waitlist (the sample parent waits on one, their partner on another), a party with sales closed, a past one with invoices sent, a pending suggestion, a hidden party, guests by name, and last year's parties paid up, so every page and rule renders locally. The party pictures under `web/celebrate/sample/` are drawn placeholders.
