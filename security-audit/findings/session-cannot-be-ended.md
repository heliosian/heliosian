Description: A session is a signed address and an expiry thirty days out that nothing re-checks (`auth.sessionEmail`), and sign-out only clears the browser's copy, so a copied cookie, a lost phone or someone who has left the school keeps every app but Who? for up to a month, the one lever being a new `SESSION_KEY` that signs everybody out.
Status: open
Severity: medium
---
`Token` (`internal/auth/auth.go:78-81`) signs `email|expiry`; `sessionEmail` (`:229-258`) checks the signature, the expiry and the address's suffix and nothing else. `logout` (`:216-227`) sets empty cookies and keeps no record, so the old value still verifies. Google is asked once, at sign-in: a Workspace account suspended the next day keeps its session. Who? alone looks the person up on each request (`who.MemberGate`); the other apps do not (`member-gate-only-on-who.md`), and a member who is still listed has nothing that ends a session they have lost hold of.

Fix: the member gate on every app ends it for a leaver. For the rest, put the time of issue in the token and keep a `Signed Out` tab in the config sheet - address and a time - that `sessionEmail` reads from the cache already in memory, refusing a token issued before it; sign-out writes the caller's own row, and a super admin can write anyone's.
