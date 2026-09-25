Description: Filing a mail into Helios Ask's documents makes the server GET every link in it whose query has an `email` parameter, on any host and through four redirects (`artifacts.tracking`), so whoever writes a Loop post or a mail the hook takes can aim blind requests from the server and stall the one filing worker.
Status: fixed
Severity: low
---
`tracking` (`internal/artifacts/links.go`) was true for any http or https address with `email` in its query, whatever the host, and `Resolve` followed such links with requests of their own, up to four redirects each, twelve at a time with a twenty-second timeout. Both the mail hook (`internal/artifacts/inbox.go`) and every Loop post (`Filer.Post`) came this way.

The redirects being followed were Benchmark's, a mailer the school stopped using in 2024. The fix drops following altogether: `Resolver` makes no requests. A link is tracking only on a `.veracross.com` host under `/c/`, and is decoded locally from its path; one that does not decode is dropped and keeps its words. Every other link is kept as written. `Warm`, `follow` and the Benchmark host rules are gone.
