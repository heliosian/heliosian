Description: Filing a mail into Helios Ask's documents makes the server GET every link in it whose query has an `email` parameter, on any host and through four redirects (`artifacts.tracking`), so whoever writes a Loop post or a mail the hook takes can aim blind requests from the server and stall the one filing worker.
Status: open
Severity: low
---
`tracking` (`internal/artifacts/links.go:46-58`) is true for any http or https address with `email` in its query, before it looks at the host; only the path rule below it is kept to Veracross's and Benchmark's hosts. `Build` warms every link in the message, and `Resolve` calls `follow` (`:152-175`), which requests the address and up to `maxHops` redirects, twelve at a time with a twenty-second timeout. Both the mail hook (`internal/artifacts/inbox.go:101`) and every Loop post (`Filer.Post`, `:113`) come this way.

Only a `Location` header is kept, so nothing is read back; the use is reaching an address inside the project's network with a GET, and a post with thousands of slow links holding the single filing worker - and the write queue's hold behind it - for as long as they take.

Fix: apply the `email` rule only on the mailers' hosts, as the path rule already is, and cap the links followed for one message.
