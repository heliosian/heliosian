Description: Helios Ask bounds the new message but not the earlier turns a request may restore (`conversation.restore`), so any signed-in account can send about 2 MB of text to Claude Opus with every question, thirty times an hour.
Status: open
Severity: medium
---
`chat` (`internal/ask/ask.go:114-142`) reads a 2 MB body, holds `message` to `maxMessageLength`, and for a conversation the server no longer has calls `conv.restore(body.Turns)` (`:247-254`), which takes each user and assistant turn at any length into `conv.messages`. Those are sent again on each of up to eight tool rounds. The only limiter is thirty questions an hour a person; there is no daily or service-wide cap.

`POST /api/ask/chat` with a short `message`, no `conversation`, and `turns` holding 1.9 MB is hundreds of thousands of input tokens a question.

Fix: in `restore`, drop or clip a user turn over `maxMessageLength` and an assistant turn over a fixed cap, stop at `maxTurns` exchanges, and cap the total.
