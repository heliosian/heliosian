package model

import (
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

var partiesTabs = []store.Tab{
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
