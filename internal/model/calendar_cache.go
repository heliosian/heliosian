package model

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"heliosian/internal/blob"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

var guestListTabs = []string{InvitesTab, InviteGroupsTab, RSVPsTab}

var calendarTabs = []store.Tab{
	{Name: GoogleTab, Columns: GoogleColumns, Key: []string{"Key"}},
	{Name: PDFTab, Columns: PDFColumns, Key: []string{"Key"}},
	{Name: EventsTab, Columns: EventColumns, Key: []string{"Event ID"}, Cascade: carryEvent},
	{Name: EnrichmentTab, Columns: EnrichmentColumns, Key: []string{"Event ID"}},
	{Name: OverridesTab, Columns: OverrideColumns, Key: []string{"Event ID"}},
	{Name: DayTypesTab, Columns: DayTypeColumns, Key: []string{"Day Type ID"}},
	{Name: DayOverridesTab, Columns: DayOverrideColumns, Key: []string{"Date", "Classrooms"}},
	{Name: TagsTab, Columns: TagColumns, Key: []string{"Tag ID"}},
	AdminsTab,
	{Name: FeedsTab, Columns: FeedColumns, Key: []string{"Token"}},
	{Name: SettingsTab, Columns: SettingColumns, Key: []string{"Email"}},
	{Name: RSVPsTab, Columns: RSVPColumns, Key: []string{"Event ID", "Email"}},
	{Name: InvitationsTab, Columns: InvitationColumns, Key: []string{"Event ID"}, Cascade: carryInvitation},
	{Name: InvitesTab, Columns: InviteColumns, Key: []string{"Event ID", "Email"}, Cascade: carryInvite},
	{Name: InviteGroupsTab, Columns: InviteGroupColumns, Key: []string{"Event ID", "Group ID"}},
	{Name: BouncesTab, Columns: BounceColumns, Key: []string{"Email", "When"}, AppendOnly: true},
	{Name: MessagesTab, Columns: MessageColumns, Key: []string{"Message ID"}},
	{Name: id.AliasesTab, Columns: id.AliasColumns, Key: []string{id.AliasColumn}},
}

func carryEvent(_ store.Tables, before, after store.Row) []store.Op {
	if before == nil || after != nil || before["Event ID"] == "" {
		return nil
	}
	return append(dropGuestList(before["Event ID"]), store.Delete(InvitationsTab, store.Row{"Event ID": before["Event ID"]}), store.Delete(OverridesTab, store.Row{"Event ID": before["Event ID"]}))
}

func carryInvitation(_ store.Tables, before, after store.Row) []store.Op {
	if before == nil || after != nil || before["Event ID"] == "" {
		return nil
	}
	return dropGuestList(before["Event ID"])
}

func dropGuestList(id string) []store.Op {
	ops := []store.Op{}
	for _, tab := range guestListTabs {
		ops = append(ops, store.Delete(tab, store.Row{"Event ID": id}))
	}
	return ops
}

func carryInvite(_ store.Tables, before, after store.Row) []store.Op {
	if before == nil || before["Email"] == "" {
		return nil
	}
	was := store.Row{"Event ID": before["Event ID"], "Email": before["Email"]}
	if after == nil {
		return []store.Op{store.Delete(RSVPsTab, was)}
	}
	if mail.Normalize(after["Email"]) == mail.Normalize(before["Email"]) {
		return nil
	}
	return []store.Op{store.Update(RSVPsTab, was, store.Row{"Email": after["Email"]})}
}

func resolveImages(ctx context.Context, images blob.Checker, model *Calendar) {
	if images == nil {
		return
	}
	names := []string{}
	for _, t := range model.Tags {
		if t.Image != "" {
			names = append(names, t.Image)
		}
	}
	pictures := map[string]string{}
	for _, e := range slices.Concat(model.Events, model.Pending) {
		if (e.Source == SourceSheet || e.imported()) && e.Image != "" {
			pictures["event "+e.ID] = strings.TrimPrefix(e.Image, "/")
		}
	}
	for id, inv := range model.Invitations {
		if inv.Flyer != "" {
			pictures["flyer "+id] = inv.Flyer
		}
	}
	for _, name := range pictures {
		names = append(names, name)
	}
	if err := images.Prefetch(ctx, names); err != nil {
		slog.Error("calendar: prefetch images", "error", err)
	}
	for i := range model.Tags {
		t := &model.Tags[i]
		if t.Image == "" {
			continue
		}
		found, err := images.Has(t.Image)
		if err != nil {
			slog.Error("calendar: tag image", "tag", t.Name, "image", t.Image, "error", err)
		} else if !found {
			slog.Warn("calendar: tag image does not exist", "tag", t.Name, "image", t.Image)
		} else {
			t.ImageURL = "/" + t.Image
		}
	}
	for what, name := range pictures {
		found, err := images.Has(name)
		if err != nil {
			slog.Error("calendar: picture", "of", what, "image", name, "error", err)
		} else if !found {
			slog.Warn("calendar: picture does not exist", "of", what, "image", name)
		}
	}
}
