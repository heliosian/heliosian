Description: Helios Ask keeps up to fifty whole chats - questions and the answers with the directory data in them - in `localStorage` under `ask-chats:<address>`, and Sign Out clears none of it, so the next person at a shared family computer reads them from the browser's tools without signing in.
Status: fixed
Severity: low
---
Sign Out can happen on any app, and only the Ask origin can clear Ask's storage, so `Clear-Site-Data` on sign-out would miss every sign-out made elsewhere.

Fixed by encrypting the chats instead: `GET /api/ask/key` (`internal/ask/ask.go`) answers, `no-store` and only to a signed-in person, an HMAC of their address under `ChatKey`, derived from `SESSION_KEY` in `internal/app`. `web/ask/app.js` imports it as a non-extractable AES-GCM key and keeps the chats as one sealed value under `chats-<sha256 of the address>`. After sign-out, from any app, what is left is ciphertext with no key in the browser. On load the page removes every entry not in that form, which clears the old plaintext `ask-chats:` and `ask-current:` entries, and an entry that will not decrypt is removed. `docs/ask/data.md`, The conversation, describes it.
