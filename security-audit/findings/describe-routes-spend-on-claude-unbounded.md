Description: Loop's and Staff Birthdays' "Generate" routes send caller-written text to Claude Opus with no role asked, no length cap and no rate limit, so any signed-in account can spend the Anthropic key's money in a loop.
Status: open
Severity: medium
---
`POST /api/loop/describe` (`internal/loop/handlers.go:485-533`) needs no group and no manager: `Title` and `RuleWords` come from the body as written, bounded only by the 256 KB body limit, and go into `describe.GroupFacts` (`:499`). `POST /api/birthday/charity/describe` (`internal/birthday/handlers.go:785-803`) discards the admin bit and passes `name` and `donationLink` - 64 KB at most - to `Describer.Charity`, which runs Opus with web search and web fetch (`internal/describe/describe.go`). Neither counts calls. Helios Ask's own limits (`messagesPerHour`, `maxMessageLength`, `internal/ask/ask.go:31-39`) are the model the README names; these two paths have none.

A loop of `fetch('/api/birthday/charity/describe', {method: 'POST', body: '{"name":"x"}'})` from any session is the demonstration.

Fix: cap the title at the group's own title length and build the rule words on the server from the rules; cap the charity's name and link; put both behind the per-user hourly window Ask uses, and the birthday one behind the team or an admin.
