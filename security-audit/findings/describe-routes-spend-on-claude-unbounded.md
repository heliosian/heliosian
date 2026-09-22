Description: Loop's and Staff Birthdays' "Generate" routes send caller-written text to Claude Opus with no role asked, no length cap and no rate limit, so any signed-in account can spend the Anthropic key's money in a loop.
Status: fixed
Severity: medium
---
Every call to Claude now passes one limiter, `claude.Limiter` in `internal/claude/limit.go`: `perHour` calls per person per hour, one window shared by Helios Ask's chat and by both Generate buttons. `internal/app/app.go` holds the single instance and hands it to `ask.Register` and to `describe.New`, and the Describer checks it inside `Group` and `Charity` (`internal/describe/describe.go`), refusing with `claude.ErrTooMany`, which `POST /api/loop/describe` (`internal/loop/handlers.go`) and `POST /api/birthday/charity/describe` (`internal/birthday/handlers.go`) answer as 429. The birthday route also sits behind `requireTeam`, though anyone may join the team.

The Describer also refuses a prompt it has built past `maxPrompt` (4 KB) with `describe.ErrTooLong`, answered as 400, so a call's input is bounded as well as its count. The rule words are still the caller's text; they shape only the description handed back to them.
