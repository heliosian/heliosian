Description: A family is keyed by its alphabetically first adult's email even after that adult has been withheld - the silent partner of a staff member listed by default, or anyone removed by the per-person Opted Out cell - and the key is served to every member as the `families` map key and `key` field of `/api/directory/model` and as the path of `/families/<key>`, naming the person the directory left out beside their household's name, children and street address.
Status: fixed
Severity: medium
---
Fixed: the model and the family page's address know a family by `familyID` (`internal/who/load.go`), an HMAC of the key email under a key derived from `SESSION_KEY` (`internal/app/app.go`, the `FamilyIDKey` in `Config`); the email itself sits in the unexported `email` field of `Family`, which only the Families tab upsert and its change log row read. `TestWithheldFirstAdultIsNowhereInTheModel` builds both paths below and asserts the withheld adult's email is absent from the encoded model.

`buildFamilies` (`internal/who/load.go`) sets `Key` to `slices.Min(hh.adults)`. `removePeople` takes withheld and opted-out people out of `AdultEmails` and `KidEmails` but keeps the family under the same key, deleting it only when nobody is left. `Family.Key` is serialised as `key` (`internal/who/model.go`) and is the `Families` map's key in the model every member fetches (`internal/who/who.go`); `FamilyPath` (`model.go`) and `familyLink` (`web/who/families.js`) put it in the URL, so it also lands in browser history, referer headers and access logs.

The consent form cannot produce this: its answer is per family (`applyPreferences`), so a form opt-out withholds every adult and child of the household together and the empty family is deleted. Two paths do produce it:

- The staff exemption (`docs/who/data.md`): in a family nobody answered for, a staff parent stays listed and the non-staff partner is withheld. When the partner's email sorts first it is the key. A staff family that never filled the form in is the usual case, not the edge.
- The per-person Opted Out cell: the self-service opt-out (`internal/who/upload.go`) and the admin's hide (`internal/who/admin.go`) each set it for one person, and `removeOptedOut` removes only them. A parent who opts out of their own record leaves the family standing under their email.

Fix: serve an opaque id and keep the email on the server.

1. Derive the id as an HMAC of the key email under a key of its own, derived from `SESSION_KEY` the way `calendarWatcher` (`internal/app/app.go`) derives its watch key, truncated to hex. Not a plain hash: a member who suspects someone's household can hash the guess and look it up in the map, which is the same oracle in a thinner disguise. `NewCache` (`app.go`) takes the derived key and hands it to the loader.
2. Key `Model.Families` by the id and set `Family.Key` to it. Add an unexported `email` field on `Family` for the two places that need the address: `applyFamilyWrite`'s upsert of the Families tab by `Email` and its change log row (`upload.go`). `familyKeys`, `consentFamilies`, `applyFamilies` and `familyKeysByEmail` carry ids; `applyFamilies` checks the row's email against the family's `email` instead of its key.
3. Handlers that read `{key}` from the path and look it up in `Families` (`upload.go`, `admin.go`, `who.go`) work unchanged. The client is unchanged apart from URLs turning opaque; bookmarks to `/families/<email>` break, which is the point.
4. Test: build a model where the first adult is withheld under each path, encode the member view to JSON, and assert the email appears nowhere in the bytes. That covers every serialisation at once rather than the three named here.
