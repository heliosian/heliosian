Description: Helios Ask opens every document that is not a Loop group's to every asker (`viewer.canRead`), so mail sent to one class's parents list or to `parentsonly` answers a parent of another class, a student, or anyone else signed in.
Status: open
Severity: high
---
`canRead` (`internal/ask/access.go:27-32`) returns true for every kind but `group`. Documents of kind `list` and `announcement` carry the list they went to as `Channel` - `hummingbirds.parents`, `falcons.students`, `parentsonly`, `parentsandstaff` and the rest (`internal/artifacts/channel.go:10-18`) - and nothing compares it with the asker. The same filter feeds `search_documents`, `read_document` and the recent-documents block of the system prompt, so all three behave alike.

`docs/ask/artifacts.md` says this is deliberate ("Helios Ask answers whoever asks it"); `security-audit/README.md` says a document from a class list must not answer a parent of another class. One of the two has to give. As a student, asking what the parents-only list said about something is the demonstration.

Fix: in `canRead`, match `Channel` against who the asker is - parent, student, staff - and their own or their children's classrooms and bands, the way the lists themselves are made up; newsletters, website pages and portal documents stay open.
