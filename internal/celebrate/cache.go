package celebrate

import (
	"context"
	"log/slog"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/id"
	"heliosian/internal/model"
	"heliosian/internal/store"
)

type Cache struct {
	*store.Store[*Model]
	model.AdminList
}

func spec(images blob.Checker) store.Spec[*Model] {
	return store.Spec[*Model]{
		App: appName,
		Tabs: []store.Tab{
			{Name: celebrationsTab, Columns: CelebrationColumns, Key: []string{"Celebration ID"}},
			{Name: categoriesTab, Columns: CategoryColumns, Key: []string{"Category ID"}},
			{Name: partiesTab, Columns: PartyColumns, Key: []string{"Party ID"}, Cascade: carryParty},
			{Name: hostsTab, Columns: HostColumns, Key: []string{"Party ID", "Email"}},
			{Name: ticketsTab, Columns: TicketColumns, Key: []string{"Ticket ID"}},
			{Name: settingsTab, Columns: SettingColumns, Key: []string{"Key"}},
			model.AdminsTab,
			{Name: redirectsTab, Columns: RedirectColumns, Key: []string{"Old"}},
			{Name: invoicingTab, Columns: InvoicingColumns, Key: []string{"Date", "Party ID", "Purchaser Email", "Guest Name"}},
			{Name: formerTab, Columns: FormerColumns, Key: []string{"Old"}},
			{Name: id.AliasesTab, Columns: id.AliasColumns, Key: []string{id.AliasColumn}},
		},
		Build: func(ctx context.Context, tables store.Tables) (*Model, error) {
			return BuildModel(ctx, tables, images)
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

func carryParty(_ store.Tables, before, after store.Row) []store.Op {
	switch {
	case before == nil || before["Party ID"] == "":
		return nil
	case after == nil:
		return []store.Op{store.Delete(hostsTab, store.Row{"Party ID": before["Party ID"]})}
	}
	was := partyPath(before["Party ID"], cells.NormalizePretty(before["Pretty ID"]))
	now := partyPath(after["Party ID"], cells.NormalizePretty(after["Pretty ID"]))
	if was == now {
		return nil
	}
	return []store.Op{store.Insert(redirectsTab, store.Row{"Type": RedirectParty, "Old": was, "New": now, "Date": today()})}
}

func NewCache(source data.Source, writer data.Writer, images blob.Checker, superAdmins func() []string, queue *store.Queue) (*Cache, error) {
	s, err := store.New(spec(images), source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s, AdminList: model.NewAdminList("celebrate", AdminAllowances, superAdmins, func() []string { return s.Model().admins }, s.Commit)}, nil
}
