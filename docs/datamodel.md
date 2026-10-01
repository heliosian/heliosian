# The data model

`internal/db` declares the data model's tables, one sheet tab each, in `db.Tables`: every table's name, the spreadsheet it lives in, the columns that key a row, and each column's kind. A tab and its columns are named exactly as the table and its columns are (`PERSON`, `vc_name`), so the sheet, the code and a query share one name for everything. `tools/createtabs` lays out the five spreadsheets from it - `Data: People`, `Data: Groups`, `Data: Documents`, `Data: Mail` and `Data: Config`, each with its Change Log - and their ids are the `DATA_*_SHEET` variables (`docs/deploy.md`, Configuration values). A generated table (`EFFECTIVE_MEMBER`, `INBOX`) and a generated column (`PERSON.name_show`) are declared beside the stored ones and have no tab or column in a sheet: `Table.Stored` is what a tab holds.

## IDs

An ID is a three-letter prefix naming what it identifies, then 11 symbols of upper and lower case letters and digits, about 65 bits; the prefixes are the `*Prefix` constants in `internal/db/id.go`. IDs are case-sensitive and are never trimmed: `db.ParseID` accepts only the exact form and answers the prefix, and `db.TableOf` the table keyed by that prefix, so an ID found anywhere - a URL, a log line, a cell - says which table holds it. `db.Mint` draws a new one, again while `taken` refuses it. A `purchase_id` is minted the same way but keys no table.

## Cells

`Column.Check` and `Table.Check` read a cell as its kind requires: an enum one of its values in any case, a reference an ID whose prefix is its target table's (any table's for `MESSAGE.about` and `ALIAS.id`), a list of references comma-separated, a yes/no, a date or moment, an order key, an address or a link as `internal/cells` and `internal/store` read them everywhere else, and an amount as digits with up to two decimal places. A blank cell is accepted unless the column is required; a column a sheet holds that the table does not declare is ignored.
