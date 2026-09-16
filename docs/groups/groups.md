# Helios Groups

Helios Groups is where the community's email groups are made: each is an address at `groups.heliosian.com` whose members follow from rules over the directory, and each is manifested as a real Google Group, kept in step as the directory changes. It serves at groups.heliosian.com. The sheet is `Groups`, the package is `internal/groups`, and the API is `/api/groups/`.

## Entities

- **Group** — a name, which is the local part of its address and never changes once made; a title and a description, which the Google group wears as its display name and description; one or more managers; and its rules. Names are lowercase letters, digits and hyphens, unique across the sheet, and a handful mail systems reserve (postmaster, abuse, and the like) are refused.
- **Rule** — one line of the definition, a filter as Helios Who? has them: a role (Students, Parents, Staff), words to find in a name or address, classrooms, grades, tags and Magic Tags, and Add family, which widens the rule's matches by the parents, children or siblings of the people it matched. Within a rule every choice must hold: Parents and Grade 3 together are the parents of a Grade 3 student, since a parent matches a grade or classroom through their children, as Who?'s filters read them. A rule with no choice at all is refused, so nobody makes an all-community group by accident.
- **Include and exclude** — a group has one or more include rules, whose matches are unioned, and zero or more exclude rules, whose matches are taken away from that. The result is the members' directory addresses; someone whose address is a placeholder nothing can reach is left out.
- **Manager** — who may change the group: its rules, its words, its managers, or delete it. Whoever makes a group is its first manager. The app's admins (the `Admins` tab and the platform super admins) see and may change every group. Managers are the app's own notion: the Google group's membership is exactly the rules' result, and a manager who is not matched by them gets no mail from it.

## Tags are read as their owner

A tag is private to the person who made it and whoever they share it with, and a Magic Tag is a person's own (the party they host, the activity they chair, the band they are room parent for), so a rule naming a tag carries its owner: whoever wrote the rule. The editor offers the editing manager their own tags, the tags shared with them (each named with whose it is), and their Magic Tags. A shared tag in a rule is read as its owner's, and only while the rule's owner is still a manager of it: once the sharing ends, the rule matches nobody through it, and the group's page says the tag is no longer shared. A rule another manager wrote is shown on the group's page and in the editor in words, and can be removed but not changed, since changing it would mean reading their tags. The server enforces the same: a rule read as anyone but the viewer must already be on the group word for word. A Magic Tag never lists its owner (Who? never puts the viewer on their own list), so a host who wants to be on the party's group adds a rule with their own name in it.

## Pages

- **My Groups** (`/`) — the groups the viewer manages, each a card with its title, its address to copy, its description, its member count, its managers and where it stands with Google. An admin's page is every group. The toolbar's search narrows the cards. New group leads to the editor.
- **A group** (`/groups/{name}`) — the address to mail or copy, the description, the sync standing, the rules read out in words, the managers, and the members as the rules pick them out now, each with the word that places them. Edit, for a manager or an admin, opens the editor on the same page.
- **The editor** (`/new`, or a group's page with `?edit=1`) — the title, the address (typed once, suggested from the title, fixed after), the description, the managers picked from the directory, the include and exclude rules each a row of controls (role chips, a search box, Classroom, Grade, Tags and Add family dropdowns), and a preview of the members the draft picks out, asked of the server as the rules change so the client never reimplements matching. Every member list, the preview's included, runs its full length and the page scrolls; Save, Cancel and Delete sit in a bar stuck to the foot of the window while the editor is open, so they are in reach wherever the page is. Saving a new group makes its Google group within a few seconds, and the page it lands on watches for that (`/api/groups/status`, asked every couple of seconds for up to a minute) and redraws the standing in place, as it does for a group whose last sync failed, so a retry shows without a reload; deleting one, after a word of confirmation, deletes the Google group.
- **Admin Tools** (`/admin`) — the Admins list, and Appearance for the platform's super admins, in the chrome every app shares.

## Keeping Google in step

Every change that can move a member in or out of a group tells the syncer: the directory reloading or being edited, a tag set or dropped, HCA-Team or Helios Celebrate reloading or being edited (their Magic Tags), and the groups sheet itself. The syncer waits a couple of seconds for the rest of a burst, recomputes every group, and sends Google only the difference from what it last sent - a bulk tag edit becomes one round of calls. Its first pass after a start reads every group's members back from Google, having nothing to diff against. A change that fails to land is logged, shown on the group's page, and retried on the next change, of which the directory's five-minute reload is one. The periodic job (`data.md`) reconciles everything from scratch on its own cadence, so a change that fell through is never lost.

## Sample data

`sampledata/groups/` holds three groups: one over a tag with Add family, one with an include and an exclude rule over grades, and one managed by someone other than the sample parent, who sees it only because the sample Config sheet makes them a super admin. In sample mode nothing reaches Google: the changes are logged and remembered in memory, so every page and every sync path can be tried locally.

## Brand

The app wears Heliosian's marks as stand-ins under `web/public/groups/brand/` and as its mark in the app switch (`web/public/common/brand/apps/groups.png`) until it has art of its own.
