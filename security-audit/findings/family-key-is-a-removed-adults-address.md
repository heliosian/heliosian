Description: A family is keyed by its alphabetically first adult's address even after that adult has opted out or been withheld, and the key is served to every member in `/api/directory/model` and in `/families/<key>`, naming the person the directory promised to leave out beside their household.
Status: open
Severity: medium
---
`buildFamilies` (`internal/who/load.go:1250-1264`) sets `Key` to `slices.Min(hh.adults)`. `removePeople` (`:1561-1588`) takes opted-out and withheld people out of `AdultEmails` and `KidEmails` but keeps the family under the same key, deleting it only when nobody is left. `Family.Key` is serialised and is the `Families` map's key in the model every member fetches (`internal/who/who.go:139-172`).

A parent who opts themselves out while their partner and children stay listed - or a partner who never answered the consent form beside a staff parent listed by default - has their address in the JSON next to the family's name, children and street address.

Fix: serve an opaque family id (a hash of the key, mapped back on the server), keeping the address as the key only inside the loader and the `Families` tab.
