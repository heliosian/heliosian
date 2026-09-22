Description: Helios Ask bounds the new message but not the earlier turns a request may restore (`conversation.restore`), so any signed-in account can send about 2 MB of text to Claude Opus with every question, thirty times an hour.
Status: fixed
Severity: medium
---
`chat` (`internal/ask/ask.go`) reads a 2 MB body, holds `message` to `maxMessageLength`, and for a conversation the server no longer has calls `conv.restore(body.Turns)`, which took each user and assistant turn at any length into `conv.messages`. Those are sent again on each of up to eight tool rounds. Restored exchanges counted toward `maxTurns`, so the number of them was bounded at thirty-nine, but not their size; with `claude-opus-5`'s 1M context a 1.9 MB restore went through at hundreds of thousands of input tokens a question.

Fixed by `maxConversationLength`: a conversation whose messages, marshalled as sent to the model, run past 200,000 runes is refused the next question with the same "start a new one" a chat past `maxTurns` gets. It is one limit on the conversation whatever built it - a live chat's questions, answers, tool calls and results, or a restored transcript - checked where `maxTurns` is, so restore is not a special path.

Signing the transcript the browser keeps, so only the server's own answers restore, was considered and left out: `restore` takes only user and assistant text, never tool results; the model's tools run as the signed-in viewer with that person's own permissions and the answer goes only to them, so a forged answer steers the model for nobody but its forger; and restoring someone else's transcript reaches none of their data, since the system blocks and tools bind to the signed-in viewer.
