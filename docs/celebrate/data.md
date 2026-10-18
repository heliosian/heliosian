# Data

Celebrate keeps nothing of its own: every celebration, category, party, ticket, waitlist place, payment and setting is a row in the data model (`docs/datamodel.md`), read and written under the policies. This file says which rows and cells those are, and what the pages work out from them (`loadModel` in `web/celebrate/state.js`).

## Where each thing lives

- **Celebration** - a `GROUP` of kind `celebration` under the `celebrate` category. Its code is its `slug` (`sc-2026`, the address `/celebrations/sc-2026`, matched ignoring case); title `name`; theme `subtitle`; `start`, `end`, `all_day`, `location`; street address `address_override`; `description`, whose markdown links are the banner's buttons; picture its first `PHOTO`. Whether it takes parties is its `adding`: `approval` lets anyone host one there, pending; blank takes none but an admin's.
- **Category** - a `GROUP` of kind `category` whose `parent` is a celebration, in `order`. Each celebration keeps its own; a party filed in one has it as its `parent`.
- **Party** - a `GROUP` of kind `party` whose `parent` is its celebration or one of its categories. `name`, `subtitle`, `description` (markdown; its first paragraph past the chips and callouts is the summary, its `> [!chip] …` lines the chips, its `> [!note] …` quotes the callouts), `slug` (the friendly address `/g/{slug}`), `start`, `end`, `all_day`, `location` (the place in words, shown to everyone), `address_override` (the street, shown with a map link), `flyer`, picture its first `PHOTO`, `price` and `unit`, `capacity`, `minimum`, `drop_off_allowed`, `parent_ticket_required`. Tickets are on sale while `join` is `direct` and closed with `none`. Who may hold one is `eligible`: the `adults` group, the `students` group, or blank for both. Hosts are its `managed_by` group's members (`<name> Managers`, managing itself). Pending is `status` `pending`; hidden is `visible_to` blank, on the party and its waitlist alike.
- **Ticket** - a `MEMBER` of the party, `member=yes`; `guest_of` is who bought or brought them when that is not the holder, `note` the buyer's note, `added` when it was taken.
- **Waitlist** - the `GROUP` the party's `waitlist` names (`<name> Waitlist`, under the party, `join` `direct` with the party's `eligible`); a party without one takes no waitlist. Each place is a `MEMBER`, in the order of `added`; a family asking for several places is the asker's row plus an unnamed guest `PERSON` per extra place, each `guest_of` the asker. Giving the places deletes those rows and writes the tickets.
- **Payment** - a `PAYMENT` per place billed: `person` who pays, `for` the holder, `group` the party, `amount` the party's price when it was bought, `added`, `stripe_invoice` once invoiced, `status` (blank while owed, `requested`, `paid`, `cancelled`). An item sold outside the app names the celebration as its `group`, the item in `name`, and no `for`. A free ticket has none, and a payment outlives a ticket removed or passed on. What a party raised is the sum of its payments not cancelled.
- **Guest** - a `PERSON` with `source` `guest`, and a `PERSON_EMAIL` when they have an address, which belongs to one guest only.
- **Settings** - `SETTING` rows of app `celebrate`: `Current` (the celebration whose parties lead the page), `Banner` (the one the band advertises), `Parties Intro`, `Ticket Note`.
- **Admins** - the group `APP` `celebrate` names in `admins`.

## What the pages work out

A party is **past** once its `end`, or its `start` when it has none, is behind; **closed** while it is not open, is hidden or does not sell (`join` not `direct`); **available** while `capacity` is blank or above its count of tickets; then **waitlist** when it has a waitlist and **sold out** when it has none. A card says how many places are left once a party is getting full (under 40% of its capacity, or ten or fewer). A person is admitted when the party's `eligible` is blank or holds them in effect (`EFFECTIVE_MEMBER`). The invite card reads the party's `invitation` `MESSAGE`s: none yet is Create Invite, any is RSVPs on Helios When; the RSVP marks are the party's `rsvp_yes` and `rsvp_no` groups.

## Sample data

The sample community (`sampledata/`) holds the Celebrate category, one celebration under it, Taco Night (`/g/taco-night`) hosted by the sample staff member with the sample parent holding a ticket, Celebrate's `APP` row and its admins group, and the `Current` setting, so the pages render locally.
