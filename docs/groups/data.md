# Data model

The tabs, columns, and validation rules are in `internal/groups`; this file carries only what reading that code cannot tell you.

The groups live in one Google Sheet, `Groups`, in the community shared drive, reached through drive membership like the other sheets. `GROUPS_SHEET` names it, and `cmd/createtabs` reads that variable to lay it out from an empty spreadsheet titled `Groups`.

## Tabs

- `Groups` — Name, Title, Description, Created By, Created. One row per group. Name is the address's local part and the key of every other tab.
- `Managers` — Group, Email. One row per manager per group; every group needs at least one.
- `Rules` — Group, Kind, Roles, Search, Classrooms, Grades, Tags, Family, Owner. One row per rule, in the order the editor shows them. Kind is `include` or `exclude`. Roles, Classrooms, Grades, Tags and Family are comma-separated lists; Tags holds tag names, Magic Tag keys (`party:<id>`, `activity:<id>`, `room:<band>`) and shared tags (`shared:<owner's address>:<name>`, one the owner has let the rule's Owner manage) alike, read as Owner's. A tag with a comma in its name cannot be used in a rule. Every group needs at least one include rule, and every rule at least one of the facets.
- `Additions` — Group, Email, Name. One row per person added by hand from outside the directory (`groups.md`, Addition). Email is lowercased and must be an address; Name may be blank, the address standing in for it. A group holds at most two hundred, once each by address. The load never asks the directory about them, so an address that has since joined the directory is not a refused load; the app refuses one at save alone.
- `Admins` — Email. Who sees and may change every group, beyond the platform super admins (`docs/config.md`).
- `Settings` — Key, Value. The app's colours (`internal/theme`, the same seven keys every app keeps in its own sheet), written by the Appearance panel of Admin Tools.
- `Change Log` — Timestamp, Actor, Action, Group, Detail. Appended on every save and delete; never read back.

**A load either succeeds whole or refuses**, the stance every app takes: a group with no manager or no include rule, a rule naming a group the sheet does not have, an unknown kind, role or relation, or a name that cannot be an address refuses the load, so a hand edit that breaks a rule surfaces as a refused start or a failed refresh rather than a group quietly matching nobody. The app checks the same rules before it writes, so nothing it saves can refuse the next load.

**Saving rewrites a group's rows whole.** A save upserts the group's `Groups` row and replaces its `Managers`, `Rules` and `Additions` rows, so the sheet's rows for a group are always exactly what the editor last held. A delete removes all four.

## The membership

A group's members are never stored. They are computed from the directory model, the owner's tags (`Cache.Tags`, or `who.TagsOf` over the tab for a tool), the tags shared with them (`Cache.SharedTags`, or `who.SharedTagsOf`) and their Magic Tags (`app.SmartLists` plus the directory's room-parent lists) whenever anything asks: the group's page, the editor's preview, the syncer, the periodic job and `cmd/loadcheck` all call the one evaluator in `internal/groups/eval.go`, a port of Who?'s client-side filter with the same reading of a parent's grade and classroom through their children. The rules' result is then unioned with the group's additions, each on once by address whatever the exclude rules say, with `Added` as its reason. The tests pin it to the sample community.

The group's Magic Tag in Who? (`GroupLists` in `internal/app/lists.go`) sorts the same members by whether the directory holds them: the directory's people are the list's People, and the rest are its Guests under the name the manager typed, keyed `<group name>:<address>` - the one `who.Guest` shape a party's ticket holders outside the directory use, keyed by the ticket there.

## Google

Each group is a Google Group under `loop.heliosian.com`, a domain of the heliosian.com Workspace, made and kept through the Cloud Identity Groups API as `directory@`, the runtime identity, which holds the Groups Admin role in the Workspace's Admin console for that - no key, no domain-wide delegation. The Workspace customer id is a constant in `internal/groups/cloudidentity.go`. The group's display name and description are the sheet's title and description, kept in step with them.

Its settings are the Groups Settings API's, which the Cloud Identity API does not reach, and every `Ensure` reads them and patches what differs from the one shape every group has (`wanted` in `cloudidentity.go`): anyone on the internet may post, with no moderation; members from outside the Workspace are allowed, since every member is a heliosschool.org address; every member sees the members and the messages; nobody leaves by hand, the rules deciding who is on it, and nobody joins by asking; and the group is in the Workspace's address list. Both APIs are enabled in the project. Membership can be made visible no wider than the group's own members or the heliosian.com Workspace - the API has no "anyone" for it - so a heliosschool.org person who is not on a group cannot see who is, and Google Calendar expands a group into its members only for an organization that can read it; groups the school's own accounts should expand would have to live in the school's Workspace.

`internal/groups.Google` is the interface the sync speaks: a group made or renamed and settled, its members read, one added or removed, a group deleted, and every address under the domain. `CloudIdentity` is the production implementation and `Fake` the in-memory one sample mode and the tests use.

## Two ways in step

**The syncer** (`internal/groups/sync.go`, `Syncer`) runs in the serving binary and answers every change (`groups.md`), sending Google only the difference from what it last sent. It holds what it last sent in memory: after a start its first pass reads every group back from Google.

**The periodic job** (`cmd/periodicsync`, the groups stage) runs on Cloud Scheduler's cadence and reconciles from scratch: every group made or renamed, every membership read from Google and corrected, so a change the syncer could not land is never lost. A group under the domain the sheet does not list - one deleted while the server was down, or made by hand - is reported in the log and left alone; deleting a group is a person's click in the app, never the job's. A failure for one group is logged and the stage carries on, and the run exits non-zero at the end naming the stage.

`cmd/loadcheck` prints every group with its member count, and `go run ./cmd/periodicsync --dry-run --i-have-user-permission-to-spend-money` prints what each group would hold without touching Google or the sheets (the second flag gates the calendar stage in the same run; `docs/calendar/data.md`).
