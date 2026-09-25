# Storage

Every app keeps its data in Google Sheets and the media and mail buckets, holds it in memory, and changes it one way. There are four layers, each reached only through the one below it:

1. **IO** (`internal/data`, `internal/blob`) - reads and writes a spreadsheet tab or a bucket object, and nothing else.
2. **Store** - one per spreadsheet: the tables as last read, the model built from them, and `Commit`, the only way anything changes them.
3. **Model** - each app's `BuildModel(tables)`, a pure function from tables to what the pages serve. It refuses tables that break the app's rules.
4. **Hooks** - what else happens when a tab changes, registered with the store: cascades inside a commit, side effects after one.

No handler, background loop, mail hook, import or tool writes a tab or an object any other way.

## IO

A tab is a header row and records keyed by column name; a cell's position in the row is never part of the contract, so columns may be rearranged, and added, by hand. The operations:

- **Read** a spreadsheet's tabs in one batched request (`Tabs`), header-only for the tabs nothing reads rows of. `Raw` reads one tab as the cells stand, for the few read positionally (the invite templates).
- **Insert** rows, each cell under the column of its name, at an explicit address after the last used row - never the Sheets append call, whose table detection starts a row in the wrong column past a blank row.
- **Set** the named cells of every row matching the given columns, inserting one row when none matches; **SetMany** does that for many rows keyed by one column in one write.
- **Delete** every row matching the given columns.
- **Reorder** a tab's rows into the order of their keys.
- **Sync** a tab to a list of rows, cell by cell: the import tabs.

Every request goes through one retry of a quota refusal (`call` in `internal/data/sheet.go`). A column the operation names that the tab lacks is an error, from the sheet and from the CSV fake (`data.Dir`) alike. The schema operations - `Layout`, `AddTab`, `AddColumns`, `DropColumns`, `RenameTab` - live here too, and `internal/data` is the only package that talks to the Sheets API: every tool reaches a sheet through it.

`internal/blob` is the only package that talks to Cloud Storage, and every object it writes, to either bucket, goes up through one upload call (`bucket.put`), and every image through one thumbnail path (`writeWithThumbnail`) before it. What differs is only the name an object is written under: `Store.Put` content-addressed media, `Store.PutNamed` the classroom and grade slots replaced in place, `Store.Write` the image search's own records, `Uploader.Put` the bulk tools' media, `Archive.Put` the mail bucket. An object is written before the commit that names it, outside the store: an object nothing names is harmless, and a row naming no object is the fault the loaders refuse.

## Store

`Commit(ctx, change)` takes a change - inserts, sets and deletes against the store's tabs - and, on the one write queue every store shares (`who.Queue`):

1. matches each operation against the tables as they stand, which gives the rows' previous values;
2. runs the cascade hooks of every tab touched, which add operations of their own, until none adds more;
3. applies the whole change to a copy of the tables and builds the model from it - a model that refuses rejects the change, and nothing is written;
4. swaps the tables and model in, and releases the request, which never waits on Sheets;
5. queues the IO writes and the Change Log rows;
6. runs the after-commit hooks.

An IO write that fails is logged `[ERROR]` and the store refreshes from the sheet, so memory never stays ahead of the sheet for longer than the queue takes.

A store reads its sheet at startup, every five minutes after, and once more thirty seconds after a start (`deployOverlap` in `internal/app`): during a deploy the old revision keeps serving and writing while the new one reads, and that second read picks up what it wrote. The five-minute refresh is for what changes in the sheets by hand. Each store counts its commits, so a refresh that read the sheet before a commit landed does not put the older sheet back. Work accepted but not yet committed - a mail the documents' hook took, a group's post being filed - holds the queue (`Hold` and `Release` in `internal/who/queue.go`), so the shutdown drain waits for it.

## Hooks

- **Cascade hooks** belong to a tab and turn one operation into more, inside the same commit: deleting a Loop group deletes its managers, rules, additions, exclusions, aliases, archived marks, messages and deliveries; renaming a person's address in Who? renames their tags and photos; deleting a calendar invitation deletes its invites, replies and groups. They see the previous rows and return operations; they write nothing themselves.
- **After-commit hooks** run once the commit is in memory: filing a sent Loop post into Helios Ask's documents, mail a change sends, geocoding a new address. One that changes stored data does it through a commit of its own, on the store that owns that data - another app's included.

## Change Log

Every spreadsheet has one `Change Log` tab, written by the store and by nothing else: Timestamp, Actor, Real Actor, Action, Tab, Key, Column, Previous. One row per cell a commit changes, cascades included:

- **Action** is `insert`, `set` or `delete`.
- **Key** names the row: the tab's key column and its value.
- **Previous** is what the cell held before the change - empty for a row that did not exist.
- **Actor** is who the change was made as, and **Real Actor** who was signed in, the two differing under Spoof Mode (`docs/toolbar.md`). Work nobody signed in does - the calendar import, the birthday reminders, Loop's mailer, the invite sweep - names itself as the actor, and Real Actor is empty.

What a row holds now is the tab itself; the Change Log is how to get back to what it held before. Nothing reads it back.

## Moving there

The IO layer above stands. The store, hooks and Change Log are the design the apps reach in steps, each shipped and tested before the next; until an app moves, it writes through `internal/data` from its own commit helpers and logs its own Change Log rows in its old shape:

1. **The store**, with one app on it - Staff Birthdays - and its Change Log in the new shape.
2. **The other apps**, one at a time: Heliosian, HCA-Team, Helios Celebrate, Helios Loop, Helios When with the calendar import, and Who?, each app's own commit, write and log helpers deleted as it moves.
3. **Feedback and Helios Ask's documents**, and the last per-app queue interfaces.

Each app's `Change Log` tab in the old shape is renamed `Change Log (old)` when its app moves, and a new one is made in this shape.
