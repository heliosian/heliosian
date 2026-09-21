Description: Helios Ask keeps up to fifty whole chats - questions and the answers with the directory data in them - in `localStorage` under `ask-chats:<address>`, and Sign Out clears none of it, so the next person at a shared family computer reads them from the browser's tools without signing in.
Status: open
Severity: low
---
`web/ask/app.js:13-42` reads and writes the chats under a key named for the signed-in address. Sign Out is a bare form post to `/auth/logout` (`web/ask/index.html:51`), which clears the cookies and leaves the origin's storage as it was. The chats of a person viewed as in Spoof Mode are kept the same way, under that person's address, on the admin's machine. Every other app keeps only interface state there (`navOpen`, filters, `superEdit` switches).

Fix: answer `/auth/logout` with `Clear-Site-Data: "storage"` on the ask host, or have the page clear its `ask-chats:` and current-chat keys as it submits the form; and keep no chats while `GET /auth/spoof` says a spoof is on.
