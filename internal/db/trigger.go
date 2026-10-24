package db

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/store"
)

type stagedTrigger struct {
	table string
	fire  func(m *Model, c Change) ([]Edit, error)
}

type committedTrigger struct {
	table string
	to    inboxName
}

var stagedTriggers = []stagedTrigger{
	{table: "PERSON", fire: schoolGroups},
	{table: "PERSON", fire: personFamilyNames},
	{table: "MEMBER", fire: memberFamilyName},
	{table: "PHOTO", fire: photoNotReady},
	{table: "PERSON", fire: madeNotMade},
	{table: "GROUP", fire: madeNotMade},
	{table: "GROUP", fire: servingNames},
	{table: "GROUP", fire: groupSlugMoved},
	{table: "GROUP", fire: hiddenFor},
	{table: "GROUP", fire: answerGroups},
	{table: "GROUP", fire: answerSharing},
	{table: "GROUP", fire: hiddenGone},
	{table: "MEMBER", fire: oneAnswer},
	{table: "DOCUMENT", fire: wikiAddressMoved},
	{table: "MESSAGE", fire: postDocument},
	{table: "DOCUMENT", fire: postSentTo},
	{table: "SAVED_SEARCH", fire: savedSearchSlugs},
}

// Staged before the row's own delete; a blank table is every table.
var deleteTriggers = []stagedTrigger{
	{fire: aliasesGone},
	{table: "DOCUMENT", fire: toDosGone},
	{table: "DOCUMENT", fire: linksGone},
	{table: "DOCUMENT", fire: photosGone},
	{table: "TODO", fire: marksGone},
}

var committedTriggers = []committedTrigger{
	{table: "MEMBER", to: mailInbox},
	{table: "GROUP", to: mailInbox},
	{table: "PHOTO", to: picturesInbox},
	{table: "PERSON", to: picturesInbox},
	{table: "GROUP", to: picturesInbox},
	{table: "GROUP", to: searchInbox},
	{table: "PERSON", to: searchInbox},
	{table: "DOCUMENT", to: searchInbox},
	{table: "CONTENT", to: searchInbox},
	{table: "PERSON", to: consentInbox},
	{table: "MEMBER", to: consentInbox},
	{table: "GROUP", to: consentInbox},
	{table: "DOCUMENT", to: fetchInbox},
	{table: "DOCUMENT", to: classifyInbox},
	{table: "DOCUMENT_GROUP", to: classifyInbox},
	{table: "DOCUMENT", to: digestInbox},
	{table: "DOCUMENT_GROUP", to: digestInbox},
	{table: "DOCUMENT", to: extractInbox},
	{table: "CONTENT", to: extractInbox},
	{table: "GROUP", to: geocodeInbox},
	{table: "GEOCODE", to: geocodeInbox},
	{table: "CONTENT", to: sweepInbox},
	{table: "DOCUMENT", to: sweepInbox},
	{table: "MESSAGE", to: sweepInbox},
	{table: "REPORT", to: sweepInbox},
	{table: "ALIAS", to: sweepInbox},
	{table: "PHOTO", to: sweepInbox},
	{table: "PERSON", to: sweepInbox},
	{table: "GROUP", to: sweepInbox},
	{table: "STOCK", to: sweepInbox},
	{table: "PERSON", to: warmInbox},
	{table: "PERSON_EMAIL", to: warmInbox},
	{table: "PHOTO", to: warmInbox},
	{table: "PERSON_SETTING", to: warmInbox},
	{table: "BIRTHDAY_YEAR", to: warmInbox},
	{table: "SAVED_SEARCH", to: warmInbox},
	{table: "GROUP", to: warmInbox},
	{table: "GROUP_SOURCE", to: warmInbox},
	{table: "MEMBER", to: warmInbox},
	{table: "RULE", to: warmInbox},
	{table: "MEMBER", to: inviteInbox},
	{table: "RULE", to: inviteInbox},
	{table: "GROUP", to: inviteInbox},
	{table: "PERSON", to: inviteInbox},
	{table: "MESSAGE", to: inviteInbox},
	{table: "RECIPIENT", to: inviteInbox},
	{table: "BIRTHDAY_YEAR", to: birthdayInbox},
	{table: "PERSON", to: birthdayInbox},
	{table: "GROUP", to: birthdayInbox},
	{table: "SETTING", to: birthdayInbox},
}

func committedInboxes(s *Store, table string) []*inbox {
	out := []*inbox{}
	for _, t := range committedTriggers {
		if t.table == table {
			out = append(out, s.Model().services.inbox(t.to))
		}
	}
	return out
}

func receivedPost(row store.Row) bool {
	return row != nil && row["direction"] == "in" && row["kind"] == "post"
}

func postDocument(m *Model, c Change) ([]Edit, error) {
	if !receivedPost(c.New) || c.New["content"] == "" || c.New["state"] != postSent {
		return nil, nil
	}
	if c.Old != nil && c.Old["content"] != "" && c.Old["state"] == postSent {
		return nil, nil
	}
	if len(m.Table("DOCUMENT").Referencing("message", c.New["id"])) > 0 {
		return nil, nil
	}
	row := map[string]any{"kind": "mail", "content": c.New["content"], "message": c.New["id"]}
	for column, from := range map[string]string{"name": "subject", "published": "created", "author": "from_person"} {
		if v := c.New[from]; v != "" {
			row[column] = v
		}
	}
	return []Edit{{Insert: "DOCUMENT", Row: row}}, nil
}

func postSentTo(m *Model, c Change) ([]Edit, error) {
	if c.Old != nil || c.New == nil || c.New["message"] == "" {
		return nil, nil
	}
	post, ok := m.Table("MESSAGE").Get(c.New["message"])
	if !ok || !receivedPost(post) || post["group"] == "" {
		return nil, nil
	}
	if _, held := m.Table("DOCUMENT_GROUP").Find(c.New["id"], post["group"], "sent_to"); held {
		return nil, nil
	}
	return []Edit{{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": c.New["id"], "group": post["group"], "relation": "sent_to"}}}, nil
}

func wikiAddressMoved(m *Model, c Change) ([]Edit, error) {
	if c.Old == nil || c.New == nil || c.Old["kind"] != "wiki" {
		return nil, nil
	}
	old := m.wikiPath(c.Old["parent"], wikiSegment(c.Old))
	if strings.EqualFold(old, m.wikiPath(c.New["parent"], wikiSegment(c.New))) {
		return nil, nil
	}
	now := wikiPathPrefix + c.New["id"]
	if row, ok := m.Table("REDIRECT").Find("wiki", old); ok {
		if row["new"] == now {
			return nil, nil
		}
		return []Edit{{Set: row["id"], Cells: map[string]any{"new": now}}}, nil
	}
	return []Edit{{Insert: "REDIRECT", Row: map[string]any{"app": "wiki", "old": old, "new": now}}}, nil
}

func groupSlugMoved(m *Model, c Change) ([]Edit, error) {
	if c.Old == nil || c.New == nil || c.Old["slug"] == "" || strings.EqualFold(c.Old["slug"], c.New["slug"]) {
		return nil, nil
	}
	old := strings.ToLower(c.Old["slug"])
	for _, a := range m.Table("ALIAS").All() {
		if a["alias"] == old {
			return nil, nil
		}
	}
	return []Edit{{Insert: "ALIAS", Row: map[string]any{"alias": old, "target": c.New["id"], "hidden": true}}}, nil
}

func oneAnswer(m *Model, c Change) ([]Edit, error) {
	if c.New == nil || memberAs(c.New) != "yes" || (c.Old != nil && memberAs(c.Old) == "yes") {
		return nil, nil
	}
	groups := m.Table("GROUP")
	writes := []Edit{}
	for _, pair := range [][2]string{{"rsvp_yes", "rsvp_no"}, {"rsvp_no", "rsvp_yes"}} {
		for _, answered := range groups.Referencing(pair[0], c.New["group"]) {
			other := answered[pair[1]]
			if other == "" {
				continue
			}
			if row, ok := m.Table("MEMBER").Find(other, c.New["person"]); ok {
				writes = append(writes, Edit{Delete: row["id"]})
			}
		}
	}
	return writes, nil
}

func servingNames(m *Model, c Change) ([]Edit, error) {
	if c.Old == nil || c.New == nil || c.Old["name"] == c.New["name"] {
		return nil, nil
	}
	writes := []Edit{}
	for _, served := range []struct{ column, suffix string }{{"managed_by", " Managers"}, {"waitlist", " Waitlist"}, {"rsvp_yes", " Going"}, {"rsvp_no", " Not Going"}, {"unsubscribed", " Unsubscribed"}, {"chair_offers", " Co-Chair Offers"}, {"leads", " Leads"}, {"audience", " Audience"}, {"hidden_for", " Hidden"}} {
		id, suffix := c.New[served.column], served.suffix
		if id == "" || id == c.New["id"] {
			continue
		}
		g, ok := m.Table("GROUP").Get(id)
		if !ok || g["name"] != c.Old["name"]+suffix {
			continue
		}
		writes = append(writes, Edit{Set: id, Cells: map[string]any{"name": c.New["name"] + suffix}})
	}
	return writes, nil
}

var hideKinds = []string{"event", "party", "activity"}

func hiddenFor(_ *Model, c Change) ([]Edit, error) {
	if c.Old != nil || c.New == nil || !slices.Contains(hideKinds, c.New["kind"]) || c.New["hidden_for"] != "" {
		return nil, nil
	}
	return []Edit{
		{Insert: "GROUP", As: "hidden", Row: map[string]any{
			"kind": "group", "name": c.New["name"] + " Hidden", "status": "open", "listed": false,
			"posting": "none", "replying": "none", "join": "self",
		}},
		{Set: c.New["id"], Cells: map[string]any{"hidden_for": "@hidden"}},
	}, nil
}

func answerGroups(_ *Model, c Change) ([]Edit, error) {
	if c.Old != nil || c.New == nil || c.New["kind"] != "event" {
		return nil, nil
	}
	return answerGroupEdits(c.New), nil
}

func answerGroupEdits(g store.Row) []Edit {
	writes := []Edit{}
	for _, answer := range []struct{ column, suffix string }{{"rsvp_yes", " Going"}, {"rsvp_no", " Not Going"}} {
		if g[answer.column] != "" {
			continue
		}
		writes = append(writes,
			Edit{Insert: "GROUP", As: answer.column, Row: map[string]any{
				"kind": "group", "parent": g["id"], "name": g["name"] + answer.suffix, "status": "open", "listed": true,
				"visible_to": g["visible_to"], "members_visible_to": g["members_visible_to"], "posting": "none", "replying": "none",
			}},
			Edit{Set: g["id"], Cells: map[string]any{answer.column: "@" + answer.column}},
		)
	}
	return writes
}

func answerSharing(m *Model, c Change) ([]Edit, error) {
	if c.Old == nil || c.New == nil || c.New["kind"] != "event" {
		return nil, nil
	}
	if c.Old["visible_to"] == c.New["visible_to"] && c.Old["members_visible_to"] == c.New["members_visible_to"] {
		return nil, nil
	}
	writes := []Edit{}
	for _, column := range []string{"rsvp_yes", "rsvp_no"} {
		if c.New[column] == "" {
			continue
		}
		writes = append(writes, Edit{Set: c.New[column], Cells: map[string]any{"visible_to": c.New["visible_to"], "members_visible_to": c.New["members_visible_to"]}})
	}
	return writes, nil
}

func hiddenGone(m *Model, c Change) ([]Edit, error) {
	if c.New != nil || c.Old == nil || c.Old["hidden_for"] == "" {
		return nil, nil
	}
	writes := []Edit{}
	for _, row := range m.Table("MEMBER").Referencing("group", c.Old["hidden_for"]) {
		writes = append(writes, Edit{Delete: row["id"]})
	}
	return append(writes, Edit{Delete: c.Old["hidden_for"]}), nil
}

func toDosGone(m *Model, c Change) ([]Edit, error) {
	writes := []Edit{}
	for _, row := range m.Table("TODO").Referencing("document", c.Old["id"]) {
		writes = append(writes, Edit{Delete: row["id"]})
	}
	return writes, nil
}

func linksGone(m *Model, c Change) ([]Edit, error) {
	writes := []Edit{}
	for _, row := range m.Table("DOCUMENT_GROUP").Referencing("document", c.Old["id"]) {
		writes = append(writes, Edit{Delete: row["id"]})
	}
	return writes, nil
}

func marksGone(m *Model, c Change) ([]Edit, error) {
	writes := []Edit{}
	for _, row := range m.Table("TODO_MARK").Referencing("todo", c.Old["id"]) {
		writes = append(writes, Edit{Delete: row["id"]})
	}
	return writes, nil
}

func aliasesGone(m *Model, c Change) ([]Edit, error) {
	if c.Table == "ALIAS" {
		return nil, nil
	}
	writes := []Edit{}
	for _, row := range m.Table("ALIAS").Referencing("target", c.Old["id"]) {
		writes = append(writes, Edit{Delete: row["id"]})
	}
	return writes, nil
}

func beforeDelete(s *Store, tx *store.Tx, c Change, where string) ([]Change, error) {
	staged := []Change{}
	for _, t := range deleteTriggers {
		if t.table != "" && t.table != c.Table {
			continue
		}
		writes, err := t.fire(s.In(tx), c)
		if err != nil {
			return nil, access.Invalid("%s: %v", where, err)
		}
		for _, w := range writes {
			_, before, err := stageWrite(s, tx, w, where, nil, true, unchecked)
			if err != nil {
				return nil, err
			}
			staged = append(staged, before)
		}
	}
	return staged, nil
}

func inOrder(c Change) []Change {
	out := []Change{}
	for _, b := range c.Before {
		out = append(out, inOrder(b)...)
	}
	return append(out, c)
}

func personFamilyNames(m *Model, c Change) ([]Edit, error) {
	p := c.New
	if p == nil {
		p = c.Old
	}
	families := []string{}
	for _, row := range m.Table("MEMBER").Referencing("person", p["id"]) {
		families = append(families, row["group"])
	}
	return m.renameFamilies(families), nil
}

func memberFamilyName(m *Model, c Change) ([]Edit, error) {
	families := []string{}
	for _, row := range []store.Row{c.Old, c.New} {
		if row == nil {
			continue
		}
		families = append(families, row["group"])
		for _, other := range m.Table("MEMBER").Referencing("person", row["person"]) {
			families = append(families, other["group"])
		}
	}
	return m.renameFamilies(families), nil
}

func (m *Model) renameFamilies(groups []string) []Edit {
	slices.Sort(groups)
	writes := []Edit{}
	for _, id := range slices.Compact(groups) {
		g, ok := m.Table("GROUP").Get(id)
		if !ok || g["kind"] != "family" {
			continue
		}
		if name := m.familyName(id); name != g["name"] {
			writes = append(writes, Edit{Set: id, Cells: map[string]any{"name": name}})
		}
	}
	return writes
}

func (m *Model) familyName(group string) string {
	students := m.students()
	order := []string{}
	for _, first := range []bool{true, false} {
		for _, row := range m.Table("MEMBER").Referencing("group", group) {
			if memberAs(row) == "yes" && students[row["person"]] == first {
				order = append(order, row["person"])
			}
		}
	}
	found := []string{}
	for _, id := range order {
		p, ok := m.Table("PERSON").Get(id)
		if !ok || !named(p) {
			continue
		}
		fields := strings.Fields(p["name_long"])
		if len(fields) == 0 {
			continue
		}
		s := fields[len(fields)-1]
		if !slices.ContainsFunc(found, func(n string) bool { return strings.EqualFold(n, s) }) {
			found = append(found, s)
		}
	}
	kept := []string{}
	for _, s := range found {
		within := slices.ContainsFunc(found, func(other string) bool {
			return !strings.EqualFold(other, s) && slices.ContainsFunc(strings.Split(other, "-"), func(part string) bool { return strings.EqualFold(part, s) })
		})
		if !within {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, " & ") + " Family"
}

func named(p store.Row) bool {
	hidden, _ := cells.YesNo(p["hidden"], false)
	return !hidden && p["deactivated"] == "" && (p["source"] == "guest" || strings.EqualFold(p["consent"], "listed"))
}

func photoNotReady(_ *Model, c Change) ([]Edit, error) {
	if c.Old == nil || c.New == nil || c.New["ready"] == "" {
		return nil, nil
	}
	if !slices.ContainsFunc(photoInputs, func(col string) bool { return c.Old[col] != c.New[col] }) {
		return nil, nil
	}
	return []Edit{{Set: c.New["id"], Cells: map[string]any{"ready": ""}}}, nil
}

func madeNotMade(_ *Model, c Change) ([]Edit, error) {
	if c.Old == nil || c.New == nil {
		return nil, nil
	}
	blank := map[string]any{}
	for _, m := range madeColumns {
		if m.table == c.Table && c.New[m.made] != "" && c.Old[m.original] != c.New[m.original] {
			blank[m.made] = ""
		}
	}
	if len(blank) == 0 {
		return nil, nil
	}
	return []Edit{{Set: c.New["id"], Cells: blank}}, nil
}

func photosGone(m *Model, c Change) ([]Edit, error) {
	out := []Edit{}
	for _, row := range m.Table("PHOTO").Referencing("document", c.Old["id"]) {
		out = append(out, Edit{Delete: row["id"]})
	}
	return out, nil
}

func unchecked(*Model, Change) error {
	return nil
}

func fire(s *Store, tx *store.Tx, c Change, env Env, where string) error {
	for pending := inOrder(c); len(pending) > 0; {
		next := pending[0]
		pending = pending[1:]
		for _, in := range committedInboxes(s, next.Table) {
			s.Model().services.collect(tx, in, delivery{change: next, env: env})
		}
		for _, t := range stagedTriggers {
			if t.table != next.Table {
				continue
			}
			writes, err := t.fire(s.In(tx), next)
			if err != nil {
				return access.Invalid("%s: %v", where, err)
			}
			names := map[string]string{}
			for _, w := range writes {
				id, c, err := stageWrite(s, tx, w, where, names, true, unchecked)
				if err != nil {
					return err
				}
				if w.As != "" {
					names[w.As] = id
				}
				pending = append(pending, inOrder(c)...)
			}
		}
	}
	return nil
}

var (
	schoolKinds   = []string{"classroom", "crew", "grade", "gradeband", "department"}
	schoolColumns = []string{"grade_override", "vc_grade", "classroom_override", "vc_classroom", "crew_override", "vc_crew", "department_override", "vc_department", "deactivated"}
)

func schoolGroups(m *Model, c Change) ([]Edit, error) {
	if c.New == nil {
		return nil, nil
	}
	if c.Old != nil && !slices.ContainsFunc(schoolColumns, func(col string) bool { return c.Old[col] != c.New[col] }) {
		return nil, nil
	}
	want, err := m.schoolGroupsOf(c.New)
	if err != nil {
		return nil, err
	}
	person := c.New["id"]
	groups := m.Table("GROUP")
	writes := []Edit{}
	for _, row := range m.Table("MEMBER").Referencing("person", person) {
		g, _ := groups.Get(row["group"])
		if !slices.Contains(schoolKinds, g["kind"]) {
			continue
		}
		if !want[row["group"]] {
			writes = append(writes, Edit{Delete: row["id"]})
			continue
		}
		delete(want, row["group"])
		if memberAs(row) != "yes" {
			writes = append(writes, Edit{Set: row["id"], Cells: map[string]any{"member": "yes"}})
		}
	}
	for _, group := range slices.Sorted(maps.Keys(want)) {
		writes = append(writes, Edit{Insert: "MEMBER", Row: map[string]any{"group": group, "person": person, "member": "yes"}})
	}
	return writes, nil
}

func (m *Model) schoolGroupsOf(p store.Row) (map[string]bool, error) {
	out := map[string]bool{}
	if p["deactivated"] != "" {
		return out, nil
	}
	for _, column := range []string{"classroom", "crew", "department"} {
		if g := overrideOr(p, column); g != "" {
			out[g] = true
		}
	}
	grade := overrideOr(p, "grade")
	if grade == "" {
		return out, nil
	}
	groups := m.Table("GROUP")
	slug := "grade-" + strings.ToLower(grade)
	i := slices.IndexFunc(groups.All(), func(g store.Row) bool { return g["kind"] == "grade" && strings.EqualFold(g["slug"], slug) })
	if i < 0 {
		return nil, fmt.Errorf("%s is in grade %s, and no grade group has the slug %s", p["id"], grade, slug)
	}
	out[groups.All()[i]["id"]] = true
	return out, nil
}
