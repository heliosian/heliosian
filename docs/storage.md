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

`internal/store` is the store. An app declares its spreadsheet in a `store.Spec`: each tab's name, the columns it must have, the columns that key a row (for the Change Log), and its cascade hook if it has one; the function that builds the model; and what to log when a model loads. `store.New` reads the spreadsheet, checks every tab's columns and the Change Log's, and builds the model, or refuses to start.

A handler states a change as operations on rows - `store.Insert`, `store.Set` (every matching row, or a new one when none matches), `store.Update` (every matching row, and nothing when none does), `store.Delete` - and hands them to `Commit(ctx, actor, ops...)`. A commit has two halves that never wait on each other. Under the store's own lock, it:

1. matches each operation against the tables as they stand, which gives the rows' previous values, and drops any that change nothing;
2. runs the cascade hook of every tab a change touched, which adds operations of its own, until none adds more;
3. builds the model from the changed tables - a model that refuses rejects the whole change, and nothing is written;
4. swaps the tables and model in, and queues the IO writes and the Change Log rows on the one write queue every store shares (`who.Queue`);
5. releases the lock and returns.

The memory half takes milliseconds and waits only on another commit's memory half; the IO half runs on the queue in commit order, and no request waits on it. An IO write that fails is logged `[ERROR]` and the store refreshes from the sheet. A mail hook is the one caller that waits for its IO: it answers the mail provider only once what it took is in the sheet, so a failure is retried by the provider rather than lost.

A store reads its sheet at startup, every five minutes after, and once more thirty seconds after a start (`deployOverlap` in `internal/app`): during a deploy the old revision keeps serving and writing while the new one reads, and that second read picks up what it wrote. The five-minute refresh is for what changes in the sheets by hand. A refresh reads on the write queue and swaps its result in only while no commit's writes are still queued: memory already holds those, and the sheet it just read does not. Work accepted but not yet committed - a mail the documents' hook took, a group's post being filed - holds the queue (`Hold` and `Release` in `internal/who/queue.go`), so the shutdown drain waits for it.

## Order

A row's place in a list the app lets people arrange is its `Order` cell, never its place in the tab: people sort and rearrange the sheet freely. An order is a sort key - lowercase letters and digits, compared as text, never ending in 0 - so there is always a key between any two, one character longer when they sit side by side. Moving a row writes that row's key alone, an ordinary `set` in the Change Log with the key it had. `store.Order` gives keys for rows in the order wanted, keeping every key it can (the longest run already in order) and placing the rest between their neighbours. A blank key sorts after every key, blank rows in the tab's own order, so a row added by hand lands last; it gets a key the first time a move needs it. Anything else in the cell refuses the load.

## Hooks

- **Cascade hooks** belong to a tab (`store.Tab.Cascade`) and turn a changed row into more operations, inside the same commit: renaming a Staff Birthdays charity renames it on every donation and in the default-charity setting, and moving a newsletter date moves every birthday pinned to it. Each sees the row before and after and returns operations; it writes nothing itself. The same hooks carry what the other apps do by hand today: deleting a Loop group's managers, rules and messages with it, renaming a person's address across Who?'s tags and photos, deleting a calendar invitation's invites, replies and groups.
- **After-commit work** runs once the commit is in memory, off the store: mail a change sends, filing a sent Loop post into Helios Ask's documents, geocoding a new address. One that changes stored data does it through a commit of its own, on the store that owns that data - another app's included.

## Change Log

Every spreadsheet has one `Change Log` tab, written by the store and by nothing else: Timestamp, Actor, Real Actor, Action, Tab, Key, Column, Previous. One row per cell a commit changes, cascades included:

- **Action** is `insert`, `set` or `delete`.
- **Key** names the row: each of the tab's key columns and its value, `Email=…; Year=…`.
- **Previous** is what the cell held before the change - empty for a row that did not exist.
- **Actor** is who the change was made as, and **Real Actor** who was signed in, the two differing under Spoof Mode (`docs/toolbar.md`). Work nobody signed in does - the calendar import, the birthday reminders, Loop's mailer, the invite sweep - names itself as the actor, and Real Actor is empty.

What a row holds now is the tab itself; the Change Log is how to get back to what it held before. Nothing reads it back.

## Moving there

The IO layer and the store stand, and Staff Birthdays, Heliosian, HCA-Team and Helios Celebrate are on the store, their Change Logs in this shape. Staff Birthdays' one write past the store is the weekly copy into the association's own spreadsheet (`docs/birthday/data.md`), an outbound export of rows the app never reads back; a birthday team joiner's place on the app's Heliosian list is a commit on Heliosian's store (`home.Grant`). The other apps still write through `internal/data` from their own commit helpers and log their own Change Log rows in the old shape; they move one at a time, each shipped and tested before the next, and each app's own commit, write and log helpers are deleted as it moves:

1. **The other apps**: Helios Loop, Helios When with the calendar import, and Who?.
2. **Feedback and Helios Ask's documents**, and the last per-app queue interfaces.

Each app's `Change Log` tab in the old shape is renamed `Change Log (old)` with `tools/renametab` when its app moves, before `tools/createtabs` makes a new one in this shape and before the build that reads it deploys.
