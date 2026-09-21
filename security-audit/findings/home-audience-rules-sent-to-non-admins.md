Description: Heliosian's front page model carries each section's and link's audience `Rules` to everyone in that audience, not only to admins, so a member reads the owning admin's address, the names of their private or shared tags, and the search words that decide who sees what.
Status: open
Severity: low
---
`internal/home/home.go:379` copies `category.Rules` into the section every viewer gets, and a link goes out whole, `Rules` included (`:384-394`; the fields are `internal/home/load.go:196`, `:230`). Only the admin's editor reads them; the page uses `ForMe`.

`GET` the front page's model as any member in a restricted audience and read `categories[].rules` and `categories[].links[].rules`.

Fix: blank `Rules` on sections and links unless the viewer is an admin.
