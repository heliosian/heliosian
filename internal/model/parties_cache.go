package model

import (
	"context"
	"log/slog"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

type PartiesCache struct {
	*store.Store[*Parties]
	AdminList
}

func partiesSpec(images blob.Checker) store.Spec[*Parties] {
	return store.Spec[*Parties]{
		App: partiesAppName,
		Tabs: []store.Tab{
			{Name: celebrationsTab, Columns: CelebrationColumns, Key: []string{"Celebration ID"}},
			{Name: partyCategoriesTab, Columns: PartyCategoryColumns, Key: []string{"Category ID"}},
			{Name: partiesTab, Columns: PartyColumns, Key: []string{"Party ID"}, Cascade: carryParty},
			{Name: hostsTab, Columns: HostColumns, Key: []string{"Party ID", "Email"}},
			{Name: ticketsTab, Columns: TicketColumns, Key: []string{"Ticket ID"}},
			{Name: partySettingsTab, Columns: KeyValueColumns, Key: []string{"Key"}},
			AdminsTab,
			{Name: partyRedirectsTab, Columns: RedirectColumns, Key: []string{"Old"}},
			{Name: invoicingTab, Columns: InvoicingColumns, Key: []string{"Date", "Party ID", "Purchaser Email", "Guest Name"}},
			{Name: formerTab, Columns: FormerColumns, Key: []string{"Old"}},
			{Name: id.AliasesTab, Columns: id.AliasColumns, Key: []string{id.AliasColumn}},
		},
		Build: func(ctx context.Context, tables store.Tables) (*Parties, error) {
			return BuildParties(ctx, tables, images)
		},
		Loaded: func(m *Parties, took time.Duration) {
			tickets := 0
			for _, p := range m.Parties {
				tickets += len(p.Tickets)
			}
			slog.Info("loaded celebrate model", "celebrations", len(m.Celebrations), "parties", len(m.Parties),
				"tickets", tickets, "skipped", m.Skipped, "took", took.Round(time.Millisecond))
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
	return []store.Op{store.Insert(partyRedirectsTab, store.Row{"Type": RedirectParty, "Old": was, "New": now, "Date": todayLocal()})}
}

func NewPartiesCache(source data.Source, writer data.Writer, images blob.Checker, superAdmins func() []string, queue *store.Queue) (*PartiesCache, error) {
	s, err := store.New(partiesSpec(images), source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &PartiesCache{Store: s, AdminList: NewAdminList("celebrate", PartiesAdminAllowances, superAdmins, func() []string { return s.Model().admins }, s.Commit)}, nil
}
