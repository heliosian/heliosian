package loop

import (
	"maps"
	"slices"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/filter"
	"heliosian/internal/id"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

func checkAdditions(directory *who.Model, additions []Addition) error {
	for _, added := range additions {
		if p := directory.Person(directory.Resolve(added.Email)); p != nil {
			return access.Invalid("%s is in the directory as %s; add them with a rule", added.Email, p.FullName)
		}
	}
	return nil
}

func sameRule(x, y Rule) bool {
	return maps.Equal(filter.RuleCells(x), filter.RuleCells(y))
}

func groupOps(was Group, g Group, adding bool) []store.Op {
	ops := []store.Op{store.Update(groupsTab, store.Row{idColumn: g.ID}, groupCells(g))}
	if adding {
		ops = []store.Op{store.Insert(groupsTab, groupCells(g))}
	}
	row := func(column, value string) store.Row {
		return store.Row{"Group": g.ID, column: value}
	}
	for _, list := range []struct {
		tab, column, owner string
		was, now           []string
	}{{managersTab, "Email", "Group", was.Managers, g.Managers}, {id.AliasesTab, id.AliasColumn, id.IDColumn, was.Aliases, g.Aliases}} {
		for _, v := range list.was {
			if !slices.Contains(list.now, v) {
				ops = append(ops, store.Delete(list.tab, store.Row{list.owner: g.ID, list.column: v}))
			}
		}
		for _, v := range list.now {
			if !slices.Contains(list.was, v) {
				ops = append(ops, store.Insert(list.tab, store.Row{list.owner: g.ID, list.column: v}))
			}
		}
	}
	if !slices.EqualFunc(was.Rules, g.Rules, sameRule) {
		ops = append(ops, store.Delete(rulesTab, store.Row{"Group": g.ID}))
		for _, r := range g.Rules {
			ops = append(ops, store.Insert(rulesTab, ruleCells(g.ID, r)))
		}
	}
	for _, added := range was.Additions {
		if g.Addition(added.Email) == nil {
			ops = append(ops, store.Delete(additionsTab, row("Email", added.Email)))
		}
	}
	for _, added := range g.Additions {
		ops = append(ops, store.Upsert(additionsTab, row("Email", added.Email), store.Row{"Name": added.Name}))
	}
	for _, e := range was.Excluded {
		if !g.HasExcluded(e.Email) {
			ops = append(ops, store.Delete(excludedTab, row("Email", e.Email)))
		}
	}
	for _, e := range g.Excluded {
		ops = append(ops, store.Upsert(excludedTab, row("Email", e.Email), store.Row{"Note": e.Note, "Timestamp": e.When}))
	}
	return ops
}

func (m *Model) SaveGroup(actor access.Actor, sources Sources, g Group, taken func(string) bool) ([]store.Op, Group, string, error) {
	g = Normalize(g)
	for _, local := range g.Names() {
		if other := m.Resolve(local); other != nil && other.ID != g.ID {
			return nil, Group{}, "", access.Invalid("%s@%s is taken", local, Domain)
		}
	}
	var was Group
	action := "add"
	if g.ID == "" {
		g.ID = id.New(taken)
		if !g.Manages(actor.Email) {
			g.Managers = append([]string{actor.Email}, g.Managers...)
		}
		g.CreatedBy = actor.Email
		g.Created = time.Now().Format("2006-01-02")
	} else {
		current := m.Group(g.ID)
		if current == nil {
			return nil, Group{}, "", access.Missing("no such email list")
		}
		if !current.Edits(actor) {
			return nil, Group{}, "", access.Forbidden("you do not manage this email list")
		}
		if g.Name != current.Name {
			return nil, Group{}, "", access.Invalid("an email list's name is its address and cannot change; make a new email list")
		}
		g.CreatedBy, g.Created = current.CreatedBy, current.Created
		was = *current
		action = "edit"
	}
	if err := filter.Writable(sources, actor.Email, g.Managers, was.Rules, g.Rules); err != nil {
		return nil, Group{}, "", access.Invalid("%v", err)
	}
	if err := CheckGroup(g); err != nil {
		return nil, Group{}, "", access.Invalid("%v", err)
	}
	if err := checkAdditions(sources.Directory, g.Additions); err != nil {
		return nil, Group{}, "", err
	}
	return groupOps(was, g, action == "add"), g, action, nil
}

func (m *Model) DeleteGroup(actor access.Actor, groupID string) ([]store.Op, *Group, error) {
	current := m.Group(groupID)
	if current == nil {
		return nil, nil, access.Missing("no such email list")
	}
	if !current.Edits(actor) {
		return nil, nil, access.Forbidden("you do not manage this email list")
	}
	return []store.Op{store.Delete(groupsTab, store.Row{idColumn: current.ID})}, current, nil
}

func (g Group) subscription(actor access.Actor, onList, subscribed bool) ([]store.Op, error) {
	if !onList {
		return nil, access.Forbidden("you are not on this email list")
	}
	if subscribed == !g.HasExcluded(actor.Email) {
		return nil, access.Invalid("you are already %s", map[bool]string{true: "subscribed", false: "unsubscribed"}[subscribed])
	}
	if subscribed {
		return g.Resubscribe(actor), nil
	}
	return g.Unsubscribe(actor, loopPage), nil
}

func (g Group) Unsubscribe(actor access.Actor, how string) []store.Op {
	if g.HasExcluded(actor.Email) {
		return nil
	}
	cells := store.Row{"Group": g.ID, "Email": actor.Email, "Note": "Unsubscribed by " + how, "Timestamp": time.Now().Format(time.RFC3339)}
	return []store.Op{store.Insert(excludedTab, cells)}
}

func (g Group) Resubscribe(actor access.Actor) []store.Op {
	if !g.HasExcluded(actor.Email) {
		return nil
	}
	return []store.Op{store.Delete(excludedTab, store.Row{"Group": g.ID, "Email": actor.Email})}
}

func (m *Model) archiving(actor access.Actor, g *Group, archived bool) ([]store.Op, error) {
	if m.Archived(g.ID, actor.Email) == archived {
		return nil, access.Invalid("you have already %s this email list", map[bool]string{true: "archived", false: "unarchived"}[archived])
	}
	match := store.Row{"Group": g.ID, "Email": actor.Email}
	if archived {
		return []store.Op{store.Upsert(archivedTab, match, store.Row{})}, nil
	}
	return []store.Op{store.Delete(archivedTab, match)}, nil
}

func recordMessage(actor access.Actor, messageID, groupID string, cells store.Row) []store.Op {
	return []store.Op{store.Upsert(messagesTab, store.Row{"ID": messageID, "Group": groupID}, cells)}
}

func markMessage(actor access.Actor, messageID, groupID, state string, cells store.Row) []store.Op {
	cells["State"] = state
	return []store.Op{store.Update(messagesTab, store.Row{"ID": messageID, "Group": groupID}, cells)}
}

func recordDeliveries(actor access.Actor, rows []store.Row) []store.Op {
	ops := []store.Op{}
	for _, row := range rows {
		ops = append(ops, store.Insert(deliveriesTab, row))
	}
	return ops
}
