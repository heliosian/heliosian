package celebrate

import (
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

type Cache struct {
	*store.Store[*Model]
	superAdmin func(email string) bool
}

func spec(images ImageChecker) store.Spec[*Model] {
	return store.Spec[*Model]{
		App: appName,
		Tabs: []store.Tab{
			{Name: celebrationsTab, Columns: CelebrationColumns, Key: []string{"Code"}, Cascade: carryCelebration},
			{Name: categoriesTab, Columns: CategoryColumns, Key: []string{"Title"}, Cascade: carryCategory},
			{Name: partiesTab, Columns: PartyColumns, Key: []string{"Party ID"}, Cascade: carryParty},
			{Name: hostsTab, Columns: HostColumns, Key: []string{"Party ID", "Email"}},
			{Name: ticketsTab, Columns: TicketColumns, Key: []string{"Ticket ID"}},
			{Name: settingsTab, Columns: SettingColumns, Key: []string{"Key"}},
			{Name: adminsTab, Columns: AdminColumns, Key: []string{"Email"}},
			{Name: redirectsTab, Columns: RedirectColumns, Key: []string{"Old"}},
			{Name: invoicingTab, Columns: InvoicingColumns, Key: []string{"Date", "Party Title", "Purchaser Email", "Guest Name"}},
		},
		Build: func(tables store.Tables) (*Model, error) {
			return BuildModel(tables, images)
		},
		Loaded: func(model *Model, took time.Duration) {
			tickets := 0
			for _, p := range model.Parties {
				tickets += len(p.Tickets)
			}
			slog.Info("loaded celebrate model", "celebrations", len(model.Celebrations), "parties", len(model.Parties),
				"tickets", tickets, "skipped", model.Skipped, "took", took.Round(time.Millisecond))
		},
	}
}

func carryCelebration(before, after store.Row) []store.Op {
	if before == nil || after == nil || before["Code"] == after["Code"] {
		return nil
	}
	return []store.Op{
		store.Update(partiesTab, store.Row{"Celebration": before["Code"]}, store.Row{"Celebration": after["Code"]}),
		store.Update(invoicingTab, store.Row{"Event Code": before["Code"]}, store.Row{"Event Code": after["Code"]}),
	}
}

func carryCategory(before, after store.Row) []store.Op {
	if before == nil || after == nil || before["Title"] == after["Title"] {
		return nil
	}
	return []store.Op{store.Update(partiesTab, store.Row{"Category": before["Title"]}, store.Row{"Category": after["Title"]})}
}

func carryParty(before, after store.Row) []store.Op {
	switch {
	case before == nil || before["Party ID"] == "":
		return nil
	case after == nil:
		return []store.Op{store.Delete(hostsTab, store.Row{"Party ID": before["Party ID"]})}
	}
	was := partyPath(before["Party ID"], NormalizePretty(before["Pretty ID"]))
	now := partyPath(after["Party ID"], NormalizePretty(after["Pretty ID"]))
	if was == now {
		return nil
	}
	return []store.Op{store.Insert(redirectsTab, store.Row{"Type": RedirectParty, "Old": was, "New": now, "Date": today()})}
}

func NewCache(source data.Source, writer data.Writer, images ImageChecker, superAdmin func(string) bool, queue store.Enqueuer) (*Cache, error) {
	s, err := store.New(spec(images), source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s, superAdmin: superAdmin}, nil
}

func (c *Cache) IsSuperAdmin(email string) bool {
	return c.superAdmin(strings.ToLower(strings.TrimSpace(email)))
}

func (c *Cache) IsAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	return slices.Contains(c.Model().admins, email) || c.superAdmin(email)
}

func (c *Cache) Admins(superAdmins []string) []string {
	admins := config.NormalizeEmails(append(slices.Clone(c.Model().admins), superAdmins...))
	sort.Strings(admins)
	return admins
}
