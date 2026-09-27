package loop

import (
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/config"
	"heliosian/internal/filter"
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
	ops := []store.Op{store.Update(groupsTab, store.Row{"Name": g.Name}, groupCells(g))}
	if adding {
		ops = []store.Op{store.Insert(groupsTab, groupCells(g))}
	}
	row := func(column, value string) store.Row {
		return store.Row{"Group": g.Name, column: value}
	}
	for _, list := range []struct {
		tab, column string
		was, now    []string
	}{{managersTab, "Email", was.Managers, g.Managers}, {aliasesTab, "Alias", was.Aliases, g.Aliases}} {
		for _, v := range list.was {
			if !slices.Contains(list.now, v) {
				ops = append(ops, store.Delete(list.tab, row(list.column, v)))
			}
		}
		for _, v := range list.now {
			if !slices.Contains(list.was, v) {
				ops = append(ops, store.Insert(list.tab, row(list.column, v)))
			}
		}
	}
	if !slices.EqualFunc(was.Rules, g.Rules, sameRule) {
		ops = append(ops, store.Delete(rulesTab, store.Row{"Group": g.Name}))
		for _, r := range g.Rules {
			ops = append(ops, store.Insert(rulesTab, ruleCells(g.Name, r)))
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

func (m *Model) SaveGroup(actor access.Actor, sources Sources, original string, g Group) ([]store.Op, Group, string, error) {
	g = Normalize(g)
	original = strings.ToLower(strings.TrimSpace(original))
	for _, local := range g.Names() {
		if other := m.Resolve(local); other != nil && other.Name != original {
			return nil, Group{}, "", access.Invalid("%s@%s is taken", local, Domain)
		}
	}
	var was Group
	action := "add"
	if original == "" {
		if !g.Manages(actor.Email) {
			g.Managers = append([]string{actor.Email}, g.Managers...)
		}
		g.CreatedBy = actor.Email
		g.Created = time.Now().Format("2006-01-02")
	} else {
		current := m.Group(original)
		if current == nil {
			return nil, Group{}, "", access.Missing("no such group")
		}
		if !current.Edits(actor) {
			return nil, Group{}, "", access.Forbidden("you do not manage this group")
		}
		if g.Name != original {
			return nil, Group{}, "", access.Invalid("a group's name is its address and cannot change; make a new group")
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

func (m *Model) DeleteGroup(actor access.Actor, name string) ([]store.Op, string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	current := m.Group(name)
	if current == nil {
		return nil, "", access.Missing("no such group")
	}
	if !current.Edits(actor) {
		return nil, "", access.Forbidden("you do not manage this group")
	}
	return []store.Op{store.Delete(groupsTab, store.Row{"Name": name})}, name, nil
}

func (m *Model) visibleGroup(actor access.Actor, s Sources, name string) (*Group, error) {
	g := m.Group(strings.ToLower(strings.TrimSpace(name)))
	if g == nil || !g.VisibleTo(actor, s) {
		return nil, access.Missing("no such group")
	}
	return g, nil
}

func (m *Model) SetSubscription(actor access.Actor, s Sources, name string, subscribed bool) ([]store.Op, *Group, error) {
	g, err := m.visibleGroup(actor, s, name)
	if err != nil {
		return nil, nil, err
	}
	if !OnList(*g, s, actor.Email) {
		return nil, nil, access.Forbidden("you are not on this group")
	}
	if subscribed {
		return g.Resubscribe(actor), g, nil
	}
	return g.Unsubscribe(actor, loopPage), g, nil
}

func (g Group) Unsubscribe(actor access.Actor, how string) []store.Op {
	if g.HasExcluded(actor.Email) {
		return nil
	}
	cells := store.Row{"Group": g.Name, "Email": actor.Email, "Note": "Unsubscribed by " + how, "Timestamp": time.Now().Format(time.RFC3339)}
	return []store.Op{store.Insert(excludedTab, cells)}
}

func (g Group) Resubscribe(actor access.Actor) []store.Op {
	if !g.HasExcluded(actor.Email) {
		return nil
	}
	return []store.Op{store.Delete(excludedTab, store.Row{"Group": g.Name, "Email": actor.Email})}
}

func (m *Model) SetArchived(actor access.Actor, s Sources, name string, archived bool) ([]store.Op, *Group, error) {
	g, err := m.visibleGroup(actor, s, name)
	if err != nil {
		return nil, nil, err
	}
	match := store.Row{"Group": g.Name, "Email": actor.Email}
	if archived {
		return []store.Op{store.Upsert(archivedTab, match, store.Row{})}, g, nil
	}
	return []store.Op{store.Delete(archivedTab, match)}, g, nil
}

func (m *Model) SetAdmins(actor access.Actor, superAdmins, requested []string) ([]store.Op, []string, error) {
	if !actor.Admin {
		return nil, nil, access.Forbidden("admin access required")
	}
	super := map[string]bool{}
	for _, e := range superAdmins {
		super[e] = true
	}
	admins := []string{}
	for _, e := range config.NormalizeEmails(requested) {
		if !super[e] {
			admins = append(admins, e)
		}
	}
	ops := []store.Op{}
	for _, e := range m.admins {
		if !slices.Contains(admins, e) {
			ops = append(ops, store.Delete(adminsTab, store.Row{"Email": e}))
		}
	}
	for _, e := range admins {
		if !slices.Contains(m.admins, e) {
			ops = append(ops, store.Insert(adminsTab, store.Row{"Email": e}))
		}
	}
	return ops, admins, nil
}

func recordMessage(actor access.Actor, id, group string, cells store.Row) []store.Op {
	return []store.Op{store.Upsert(messagesTab, store.Row{"ID": id, "Group": group}, cells)}
}

func markMessage(actor access.Actor, id, group, state string, cells store.Row) []store.Op {
	cells["State"] = state
	return []store.Op{store.Update(messagesTab, store.Row{"ID": id, "Group": group}, cells)}
}

func recordDeliveries(actor access.Actor, rows []store.Row) []store.Op {
	ops := []store.Op{}
	for _, row := range rows {
		ops = append(ops, store.Insert(deliveriesTab, row))
	}
	return ops
}
