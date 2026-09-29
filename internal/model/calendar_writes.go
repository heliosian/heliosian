package model

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

func homeOp(email string, cells store.Row) store.Op {
	return store.Upsert(SettingsTab, store.Row{"Email": email}, cells)
}

func feedCells(body feedBody) store.Row {
	return store.Row{
		"Name": strings.TrimSpace(body.Name), "Emoji": feedEmoji(body.Emoji), "Classrooms": cells.JoinList(cells.SplitList(cells.JoinList(body.Classrooms))), "Tags": cells.JoinList(cells.SplitList(cells.JoinList(body.Tags))),
	}
}

func (a calendarApp) newFeed(actor access.Actor, body feedBody) ([]store.Op, store.Row) {
	cells := feedCells(body)
	cells["Token"], cells["Email"], cells["Created"] = id.Token(), actor.Email, now().Format(DateTimeFormat)
	cells["Name"] = a.model().unusedFeedName(actor.Email, strings.TrimSpace(body.Name))
	return []store.Op{store.Insert(FeedsTab, cells)}, cells
}

func (a calendarApp) changeFeed(actor access.Actor, body feedBody) ([]store.Op, store.Row, error) {
	if strings.TrimSpace(body.Name) == "" {
		return nil, nil, access.Invalid("a feed needs a name")
	}
	if strings.TrimSpace(body.Token) == MyHeliosianToken {
		return []store.Op{homeOp(actor.Email, store.Row{"Home Name": strings.TrimSpace(body.Name), "Home Emoji": feedEmoji(body.Emoji)})}, nil, nil
	}
	f := a.model().Feed(strings.TrimSpace(body.Token))
	if f == nil {
		return nil, nil, access.Missing("no such feed")
	}
	if f.Email != actor.Email && !actor.May(FeedsForAnyone) {
		return nil, nil, access.Forbidden("only the person who made a feed, or an admin, can change it")
	}
	cells := feedCells(body)
	return []store.Op{store.Update(FeedsTab, store.Row{"Token": f.Token}, cells)}, cells, nil
}

func (a calendarApp) dropFeed(actor access.Actor, token string) ([]store.Op, string, error) {
	f := a.model().Feed(strings.TrimSpace(token))
	if f == nil {
		return nil, "", access.Missing("no such feed")
	}
	if f.Email != actor.Email && !actor.May(FeedsForAnyone) {
		return nil, "", access.Forbidden("only the person who made a feed, or an admin, can remove it")
	}
	return []store.Op{store.Delete(FeedsTab, store.Row{"Token": f.Token})}, f.Name, nil
}

func (a calendarApp) orderOps(actor access.Actor, tokens []string) ([]store.Op, error) {
	ops := []store.Op{}
	if at := slices.Index(tokens, MyHeliosianToken); at >= 0 {
		ops = append(ops, homeOp(actor.Email, store.Row{"Home Position": strconv.Itoa(at)}))
		tokens = slices.Delete(slices.Clone(tokens), at, at+1)
	}
	mine := map[string]string{}
	for _, f := range a.model().Feeds {
		if mail.Normalize(f.Email) == actor.Email {
			mine[f.Token] = f.order
		}
	}
	if len(tokens) != len(mine) {
		return nil, access.Invalid("the order must name each of your calendars once")
	}
	current := []string{}
	for _, t := range tokens {
		order, ok := mine[t]
		if !ok || slices.Contains(tokens[:len(current)], t) {
			return nil, access.Invalid("the order must name each of your calendars once")
		}
		current = append(current, order)
	}
	keys := store.Order(current)
	for i, t := range tokens {
		if keys[i] != current[i] {
			ops = append(ops, store.Update(FeedsTab, store.Row{"Token": t}, store.Row{store.OrderColumn: keys[i]}))
		}
	}
	return ops, nil
}

func (a calendarApp) defaultOps(actor access.Actor, token string) ([]store.Op, []string, error) {
	tokens := []string{token}
	found := false
	for _, f := range a.model().MyCalendars(actor.Email) {
		if f.Token == token {
			found = true
			continue
		}
		tokens = append(tokens, f.Token)
	}
	if !found {
		return nil, nil, access.Invalid("that is not one of your calendars")
	}
	ops, err := a.orderOps(actor, tokens)
	return ops, tokens, err
}

func (a calendarApp) feedTokenOps(actor access.Actor) ([]store.Op, string) {
	if token := a.model().Settings[actor.Email].FeedToken; token != "" {
		return nil, token
	}
	token := id.Token()
	return []store.Op{homeOp(actor.Email, store.Row{"Feed Token": token})}, token
}

func saveViewOps(actor access.Actor, classrooms, tags []string) ([]store.Op, store.Row) {
	row := store.Row{
		"Classrooms": cells.JoinList(cells.SplitList(cells.JoinList(classrooms))), "Categories": cells.JoinList(cells.SplitList(cells.JoinList(tags))), "Saved": now().Format(DateTimeFormat),
	}
	return []store.Op{store.Upsert(SettingsTab, store.Row{"Email": actor.Email}, row)}, row
}

func (a calendarApp) forgetViewOps(actor access.Actor) []store.Op {
	if _, ok := a.model().Settings[actor.Email]; !ok {
		return nil
	}
	return []store.Op{store.Update(SettingsTab, store.Row{"Email": actor.Email}, store.Row{"Classrooms": "", "Categories": "", "Saved": ""})}
}

var (
	SeeAllEvents   = access.Named("when.see-all")
	CurateCalendar = access.Named("when.curate")
	ActAsHost      = access.Named("when.act-as-host")
	FeedsForAnyone = access.Named("when.feeds-for-anyone")
)

var CalendarAdminAllowances = []access.Allowance{SeeAllEvents, CurateCalendar, ActAsHost, FeedsForAnyone}

func adminOnly(actor access.Actor) error {
	if !actor.May(CurateCalendar) {
		return access.Forbidden("only a calendar admin can do that")
	}
	return nil
}

func (a calendarApp) keywordOps(actor access.Actor, id string, keywords []string) ([]store.Op, *Event, string, error) {
	if err := adminOnly(actor); err != nil {
		return nil, nil, "", err
	}
	e := a.model().Event(id)
	if e == nil {
		return nil, nil, "", access.Missing("that event is not in the sheet")
	}
	words := cells.SplitList(cells.JoinList(keywords))
	cell := Clear
	if len(words) > 0 {
		cell = cells.JoinList(words)
	}
	return []store.Op{store.Upsert(OverridesTab, store.Row{"Event ID": e.ID}, store.Row{"Keywords": cell})}, e, cell, nil
}

func (a calendarApp) newEvents(actor access.Actor, body eventBody) ([]store.Op, []string, bool, error) {
	if !slices.Contains(sharingWords, body.Sharing) {
		return nil, nil, false, access.Invalid("sharing is %s", strings.Join(sharingWords, ", "))
	}
	if body.RepeatTimes < 0 || body.RepeatTimes > 52 || body.RepeatWeeks < 1 && body.RepeatTimes > 0 {
		return nil, nil, false, access.Invalid("repeat up to 52 more times, some whole number of weeks apart")
	}
	model := a.model()
	address := strings.ToLower(strings.TrimSpace(body.Address))
	if address != "" {
		if !validAddress(address) {
			return nil, nil, false, access.Invalid("a web address is 3 to 40 letters, digits and dashes")
		}
		if model.Event(address) != nil || a.store.Count(CalendarApp, OverridesTab, store.Row{"Address": address}) > 0 {
			return nil, nil, false, access.Invalid("that web address is taken")
		}
	}
	if !actor.May(CurateCalendar) {
		body.RepeatTimes, body.DayType = 0, ""
	}
	pending := body.Sharing == SharingPublic
	status := ""
	if pending {
		status = StatusPending
	}
	stamp := now().Format(DateFormat)
	ids := []string{}
	ops := []store.Op{}
	mint := model.Minter()
	for i := 0; i <= body.RepeatTimes; i++ {
		key := mint()
		ids = append(ids, key)
		told := ""
		if i == 0 {
			told = owed + ":" + actor.Email
		}
		ops = append(ops, store.Insert(EventsTab, store.Row{
			"Admins Told": told, "Event ID": key, "Start": shiftWhen(strings.TrimSpace(body.Start), i*body.RepeatWeeks), "End": shiftWhen(strings.TrimSpace(body.End), i*body.RepeatWeeks),
			"Title": strings.TrimSpace(body.Title), "Location": strings.TrimSpace(body.Location), "Description": strings.TrimSpace(body.Description),
			"Tags": cells.JoinList(cells.SplitList(cells.JoinList(body.Tags))), "Day Type": strings.TrimSpace(body.DayType), "Keywords": cells.JoinList(cells.SplitList(cells.JoinList(body.Keywords))),
			"Added By": actor.Email, "Added": stamp, "Source": strings.TrimSpace(body.Source), "Sharing": body.Sharing, "Status": status, "Image": strings.Trim(strings.TrimSpace(body.Image), "/"),
		}))
	}
	if address != "" {
		ops = append(ops, store.Insert(OverridesTab, store.Row{"Event ID": ids[0], "Address": address}))
	}
	return ops, ids, pending, nil
}

func (a calendarApp) changeEvent(actor access.Actor, body eventBody) ([]store.Op, *Event, store.Row, error) {
	if !slices.Contains(sharingWords, body.Sharing) {
		return nil, nil, nil, access.Invalid("sharing is %s", strings.Join(sharingWords, ", "))
	}
	e := a.model().Event(strings.TrimSpace(body.ID))
	if e == nil || e.Source != SourceSheet {
		return nil, nil, nil, access.Missing("that event is not one added by hand")
	}
	if !a.isHost(actor, e) {
		return nil, nil, nil, access.Forbidden("only a host of an event, or an admin, can change it")
	}
	row := store.Row{
		"Title": strings.TrimSpace(body.Title), "Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End),
		"Location": strings.TrimSpace(body.Location), "Description": strings.TrimSpace(body.Description),
		"Tags": cells.JoinList(cells.SplitList(cells.JoinList(body.Tags))), "Keywords": cells.JoinList(cells.SplitList(cells.JoinList(body.Keywords))),
		"Source": strings.TrimSpace(body.Source), "Image": strings.Trim(strings.TrimSpace(body.Image), "/"),
	}
	if body.Sharing != e.Sharing {
		row["Sharing"] = body.Sharing
		row["Status"] = ""
		if body.Sharing == SharingPublic {
			row["Status"] = StatusPending
			row["Admins Told"] = owed + ":" + actor.Email
		}
	}
	return []store.Op{store.Update(EventsTab, store.Row{"Event ID": e.ID}, row)}, e, row, nil
}

func (a calendarApp) statusOps(actor access.Actor, id, status string) ([]store.Op, *Event, error) {
	if err := adminOnly(actor); err != nil {
		return nil, nil, err
	}
	e := a.model().Event(strings.TrimSpace(id))
	if e == nil || e.Source != SourceSheet {
		return nil, nil, access.Missing("that event is not one added by hand")
	}
	if status == StatusDeclined && e.Sharing != SharingPublic {
		return nil, nil, access.Invalid("only a public event is declined")
	}
	return []store.Op{store.Update(EventsTab, store.Row{"Event ID": e.ID}, store.Row{"Status": status})}, e, nil
}

func (a calendarApp) moveOps(actor access.Actor, id, start, end string) ([]store.Op, *Event, error) {
	if err := adminOnly(actor); err != nil {
		return nil, nil, err
	}
	e := a.model().Event(id)
	if e == nil || e.Source != SourceSheet {
		return nil, nil, access.Invalid("only an event of the Events tab moves from here")
	}
	return []store.Op{store.Update(EventsTab, store.Row{"Event ID": e.ID}, store.Row{"Start": start, "End": end})}, e, nil
}

func (a calendarApp) tagOps(actor access.Actor, tags []tagBody) ([]store.Op, int, error) {
	if !actor.May(CurateCalendar) {
		return nil, 0, access.Forbidden("only a calendar admin can change the categories")
	}
	model := a.model()
	current := map[string]CalendarTag{}
	taken := map[string]bool{}
	for _, t := range model.Tags {
		taken[t.Name] = true
		if !t.BuiltIn {
			current[t.ID] = t
		}
	}
	mint := model.Minter()
	keys, names, orders, rows := []string{}, []string{}, []string{}, []store.Row{}
	for _, t := range tags {
		key, name, description, group, image := strings.TrimSpace(t.ID), strings.TrimSpace(t.Name), strings.TrimSpace(t.Description), strings.TrimSpace(t.Group), strings.TrimSpace(t.Image)
		if key == "" {
			if name == "" || taken[name] {
				return nil, 0, access.Invalid("every category needs a name of its own")
			}
			taken[name] = true
			key = mint()
		} else {
			was, ok := current[key]
			switch {
			case BuiltInTag(key):
				return nil, 0, access.Invalid("%s is built in and is changed in the sheet", model.TagName(key))
			case !ok || slices.Contains(keys, key):
				return nil, 0, access.Invalid("%q is not a category, or is listed twice", key)
			}
			name = was.Name
		}
		if description == "" {
			return nil, 0, access.Invalid("%q needs a description", name)
		}
		keys = append(keys, key)
		names = append(names, name)
		orders = append(orders, current[key].order)
		rows = append(rows, store.Row{"Description": description, "Group": group, "Default": cells.YesNoCell(t.Default), "Image": image})
	}
	for key, t := range current {
		if !slices.Contains(keys, key) {
			return nil, 0, access.Invalid("%q is missing - a category cannot be removed from here", t.Name)
		}
	}
	sorted := store.Order(orders)
	ops := []store.Op{}
	added := 0
	for i, key := range keys {
		cells := rows[i]
		cells[store.OrderColumn] = sorted[i]
		was, ok := current[key]
		if !ok {
			cells["Tag ID"], cells["Tag"] = key, names[i]
			ops = append(ops, store.Insert(TagsTab, cells))
			added++
			continue
		}
		if tags[i].Default == was.Default {
			delete(cells, "Default")
		}
		ops = append(ops, store.Update(TagsTab, store.Row{"Tag ID": key}, cells))
	}
	return ops, added, nil
}

func (a calendarApp) mayCorrect(actor access.Actor, e *Event) error {
	if !actor.May(CurateCalendar) && !a.isHost(actor, e) {
		return access.Forbidden("only a host of an event, or an admin, can change it")
	}
	return nil
}

func (a calendarApp) overrideOps(actor access.Actor, body overrideBody) ([]store.Op, string, bool, error) {
	model := a.model()
	e := model.Event(strings.TrimSpace(body.ID))
	if e == nil || !e.imported() {
		return nil, "", false, access.Missing("only an event the school's calendars bring is corrected here")
	}
	if err := a.mayCorrect(actor, e); err != nil {
		return nil, "", false, err
	}
	id := e.ID
	school := model.imports[id]
	row := store.Row{}
	plain := func(s string) string {
		return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"))
	}
	set := func(column, want, was string) {
		switch want = strings.TrimSpace(want); {
		case plain(want) == plain(was):
			row[column] = ""
		case want == "":
			row[column] = Clear
		default:
			row[column] = want
		}
	}
	set("Title", body.Title, school.Title)
	start, end, allDay, err := parseWhen(strings.TrimSpace(body.Start), strings.TrimSpace(body.End))
	if err != nil {
		return nil, "", false, access.Invalid("%v", err)
	}
	if start.Equal(school.start) && end.Equal(school.end) && allDay == school.AllDay {
		row["Start"], row["End"] = "", ""
	} else {
		row["Start"], row["End"] = strings.TrimSpace(body.Start), strings.TrimSpace(body.End)
	}
	set("Location", body.Location, school.Location)
	set("Description", body.Description, school.Description)
	list := func(column string, want, was []string) {
		want = cells.SplitList(cells.JoinList(want))
		was = slices.DeleteFunc(slices.Clone(was), BuiltInTag)
		x, y := slices.Sorted(slices.Values(want)), slices.Sorted(slices.Values(was))
		switch {
		case slices.Equal(x, y):
			row[column] = ""
		case len(want) == 0:
			row[column] = Clear
		default:
			row[column] = cells.JoinList(want)
		}
	}
	list("Tags", body.Tags, school.Tags)
	list("Keywords", body.Keywords, school.Keywords)
	row["Note"] = ""
	if p := model.Provenance[id]; p != nil {
		row["Note"] = p.Note
	}
	if body.Note != nil && actor.May(CurateCalendar) {
		row["Note"] = strings.TrimSpace(*body.Note)
	}
	row["Address"] = strings.ToLower(strings.TrimSpace(body.Address))
	if row["Address"] != "" && !validAddress(row["Address"]) {
		return nil, "", false, access.Invalid("an address is letters, digits and dashes, 3 to 40 of them")
	}
	empty := !slices.ContainsFunc(slices.Collect(maps.Values(row)), func(v string) bool { return v != "" })
	if p := model.Provenance[id]; p != nil && slices.ContainsFunc(p.Corrected, func(c string) bool { return c == "Day Type" || c == "Hidden" || c == "Image" }) {
		empty = false
	}
	if empty {
		return []store.Op{store.Delete(OverridesTab, store.Row{"Event ID": id})}, id, true, nil
	}
	return []store.Op{store.Upsert(OverridesTab, store.Row{"Event ID": id}, row)}, id, false, nil
}

func (a calendarApp) overrideImageOps(actor access.Actor, id, image string) ([]store.Op, *Event, string, error) {
	e := a.model().Event(strings.TrimSpace(id))
	if e == nil || !e.imported() {
		return nil, nil, "", access.Missing("only an event the school's calendars bring takes its picture here")
	}
	if err := a.mayCorrect(actor, e); err != nil {
		return nil, nil, "", err
	}
	image = strings.Trim(strings.TrimSpace(image), "/")
	return []store.Op{store.Upsert(OverridesTab, store.Row{"Event ID": e.ID}, store.Row{"Image": image})}, e, image, nil
}

func (a calendarApp) invitationOps(actor access.Actor, id string, cells store.Row) []store.Op {
	if a.model().Invitations[id] != nil {
		if len(cells) == 0 {
			return nil
		}
		return []store.Op{store.Update(InvitationsTab, store.Row{"Event ID": id}, cells)}
	}
	row := store.Row{"Event ID": id, "Audience": "Both", "Guests": "Yes", "Created By": actor.Email, "Created": now().Format(DateTimeFormat)}
	maps.Copy(row, cells)
	return []store.Op{store.Insert(InvitationsTab, row)}
}

func (a calendarApp) settingsOps(actor access.Actor, body settingsBody) ([]store.Op, *Event, []string, error) {
	e, err := a.hostedEvent(actor, body.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	row := store.Row{}
	if body.HideHosts != nil {
		row["Hide Hosts"] = cells.YesNoCell(*body.HideHosts)
	}
	if body.PublicList != nil {
		row["Public Guest List"] = cells.YesNoCell(*body.PublicList)
	}
	if body.Guests != nil {
		row["Guests"] = cells.YesNoCell(*body.Guests)
	}
	if body.NotifyMe != nil {
		notify := []string{}
		if inv := a.model().Invitations[e.ID]; inv != nil {
			notify = append(notify, inv.Notify...)
		}
		notify = slices.DeleteFunc(notify, func(h string) bool { return h == actor.Email })
		if *body.NotifyMe {
			notify = append(notify, actor.Email)
		}
		row["Notify"] = strings.Join(notify, ", ")
	}
	if body.Flyer != nil {
		flyer := strings.Trim(strings.TrimSpace(*body.Flyer), "/")
		if flyer != "" && a.images.Read(flyer) == nil {
			return nil, nil, nil, access.Invalid("that picture is not here")
		}
		row["Flyer"] = flyer
	}
	if e.linked() {
		for col, v := range map[string]*string{"Title": body.Title, "Location": body.Location, "Description": body.Description} {
			if v != nil {
				if len(*v) > maxTextLength {
					return nil, nil, nil, access.Invalid("the %s is too long", strings.ToLower(col))
				}
				row[col] = strings.TrimSpace(*v)
			}
		}
		if body.Start != nil {
			start, end := strings.TrimSpace(*body.Start), ""
			if body.End != nil {
				end = strings.TrimSpace(*body.End)
			}
			if start != "" {
				if _, _, _, err := parseWhen(start, end); err != nil {
					return nil, nil, nil, access.Invalid("%v", err)
				}
			} else {
				end = ""
			}
			row["Start"], row["End"] = start, end
		}
	}
	switch strings.ToLower(strings.TrimSpace(body.Audience)) {
	case AudienceAdults:
		row["Audience"] = "Adults"
	case AudienceStudents:
		row["Audience"] = "Students"
	case AudienceBoth:
		row["Audience"] = "Both"
	case "":
	default:
		return nil, nil, nil, access.Invalid("the audience is adults, students, or both")
	}
	if body.Message != nil {
		if len(*body.Message) > maxTextLength {
			return nil, nil, nil, access.Invalid("the message is too long")
		}
		row["Message"] = strings.TrimSpace(*body.Message)
	}
	newHosts := []string{}
	if body.Hosts != nil {
		hosts := []string{}
		for _, h := range body.Hosts {
			h = a.directory().Resolve(mail.Normalize(h))
			if a.directory().Person(h) == nil {
				return nil, nil, nil, access.Invalid("%s is not in the directory", h)
			}
			if !slices.Contains(hosts, h) {
				hosts = append(hosts, h)
			}
			if !slices.Contains(a.hostsOf(e), h) {
				newHosts = append(newHosts, h)
			}
		}
		row["Hosts"] = cells.JoinList(hosts)
		if len(newHosts) > 0 {
			tell := []string{}
			if inv := a.model().Invitations[e.ID]; inv != nil {
				tell = append(tell, inv.HostsToTell...)
			}
			for _, h := range newHosts {
				tell = append(tell, h+":"+actor.Email)
			}
			row["Hosts To Tell"] = strings.Join(tell, ", ")
		}
	}
	return append(a.invitationOps(actor, e.ID, row), a.hostYesOps(actor, e, newHosts)...), e, newHosts, nil
}

func (a calendarApp) hostYesOps(actor access.Actor, e *Event, hosts []string) []store.Op {
	model := a.model()
	stamp := now().Format(DateTimeFormat)
	ops := []store.Op{}
	for _, h := range hosts {
		if model.AnswerOf(h, e.ID) != "" {
			continue
		}
		ops = append(ops, store.Upsert(RSVPsTab, store.Row{"Event ID": e.ID, "Email": h}, store.Row{"Answer": AnswerYes, "Answered": stamp, "Answered By": actor.Email, "Via": ViaPage}))
	}
	return ops
}

func (a calendarApp) stepDownOps(actor access.Actor, id, email string) ([]store.Op, *Event, string, bool, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, "", false, err
	}
	who := actor.Email
	if email := a.directory().Resolve(mail.Normalize(email)); email != "" {
		who = email
	}
	inv := a.model().Invitations[e.ID]
	cohost := inv != nil && slices.Contains(inv.Hosts, who)
	poster := e.Source == SourceSheet && !e.PosterLeft && a.directory().Resolve(mail.Normalize(e.AddedBy)) == who
	if !cohost && !poster {
		return nil, nil, "", false, access.Invalid("that person hosts this event on the app that runs it, or not at all - step down there")
	}
	row := store.Row{}
	if poster {
		row["Stepped Down"] = mail.Normalize(e.AddedBy)
	}
	if inv != nil {
		if cohost {
			row["Hosts"] = cells.JoinList(slices.DeleteFunc(slices.Clone(inv.Hosts), func(h string) bool { return h == who }))
		}
		if slices.Contains(inv.Notify, who) {
			row["Notify"] = strings.Join(slices.DeleteFunc(slices.Clone(inv.Notify), func(h string) bool { return h == who }), ", ")
		}
	}
	return a.invitationOps(actor, e.ID, row), e, who, poster, nil
}

func (a calendarApp) inviteOps(actor access.Actor, key string, people []invitee) ([]store.Op, *Event, []string, bool, error) {
	e, host, err := a.inviterEvent(actor, key)
	if err != nil {
		return nil, nil, nil, false, err
	}
	if !host && len(people) > 20 {
		return nil, nil, nil, false, access.Invalid("invite up to twenty people at a time")
	}
	if len(people) == 0 || len(people) > 500 {
		return nil, nil, nil, false, access.Invalid("add between one and five hundred people at a time")
	}
	model := a.model()
	stamp := now().Format(DateTimeFormat)
	ops := a.invitationOps(actor, e.ID, nil)
	emails := []string{}
	mint := model.Minter()
	for _, p := range people {
		name := strings.TrimSpace(p.Name)
		household := mail.Normalize(p.Household)
		email := a.directory().Resolve(mail.Normalize(p.Email))
		token := ""
		switch {
		case email == "" && household != "" && name != "":
			email = guestPrefix + mint()
		case !emailForm.MatchString(email):
			return nil, nil, nil, false, access.Invalid("%q is not an email address", p.Email)
		default:
			token = id.Token()
			if person := a.directory().Person(email); person != nil {
				name, token, household = person.FullName, "", ""
			}
		}
		if slices.Contains(emails, email) || model.InviteOf(e.ID, email) != nil {
			continue
		}
		emails = append(emails, email)
		if len(name) > maxTitleLength {
			return nil, nil, nil, false, access.Invalid("a name is too long")
		}
		if household != "" && !emailForm.MatchString(household) {
			return nil, nil, nil, false, access.Invalid("a family is named by an address")
		}
		via := strings.TrimSpace(p.Via)
		if !host {
			via = ViaInvited
		}
		ops = append(ops, store.Insert(InvitesTab, store.Row{"Event ID": e.ID, "Email": email, "Name": name, "Via": via, "Added By": actor.Email, "Added": stamp, "Token": token, "Household": household}))
	}
	return ops, e, emails, host, nil
}

func (a calendarApp) uninviteOps(actor access.Actor, id, email string) ([]store.Op, *Event, string, bool, error) {
	e, err := a.findEvent(actor, id)
	if err != nil {
		return nil, nil, "", false, err
	}
	email = mail.Normalize(email)
	inv := a.model().InviteOf(e.ID, email)
	if inv == nil {
		return nil, nil, "", false, access.Missing("they are not on the list")
	}
	if !a.isHost(actor, e) && !(inv.GuestOf != "" && a.mayAnswerFor(actor, inv.GuestOf, e)) {
		return nil, nil, "", false, access.Forbidden("only a host can take someone off the list")
	}
	ops := []store.Op{store.Delete(InvitesTab, store.Row{"Event ID": e.ID, "Email": email})}
	gid, ok := strings.CutPrefix(inv.Via, ViaGroup)
	if !ok {
		return ops, e, email, false, nil
	}
	group := a.model().GroupOf(e.ID, gid)
	if group == nil {
		return ops, e, email, false, nil
	}
	ops = append(ops, store.Update(InviteGroupsTab, store.Row{"Event ID": e.ID, "Group ID": gid}, store.Row{"Removed": strings.Join(append(slices.Clone(group.Removed), email), ", ")}))
	return ops, e, email, true, nil
}

type broughtGuest struct {
	event  *Event
	of     string
	key    string
	answer string
	invite bool
}

func (a calendarApp) guestOps(actor access.Actor, g broughtGuest, name, email string) ([]store.Op, broughtGuest, error) {
	model := a.model()
	e := g.event
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxTitleLength {
		return nil, g, access.Invalid("a guest needs a name")
	}
	email = mail.Normalize(email)
	stamp := now().Format(DateTimeFormat)
	row := store.Row{"Event ID": e.ID, "Name": name, "Guest Of": g.of, "Via": ViaGuest, "Added By": actor.Email, "Added": stamp}
	if email != "" {
		if !emailForm.MatchString(email) {
			return nil, g, access.Invalid("that is not an email address")
		}
		email = a.directory().Resolve(email)
		if model.InviteOf(e.ID, email) != nil {
			return nil, g, access.Invalid("they are on the list already")
		}
		if p := a.directory().Person(email); p != nil {
			row["Name"] = p.FullName
		} else {
			row["Token"] = id.Token()
		}
	} else {
		email = guestPrefix + model.Minter()()
		row["Sent"] = stamp
	}
	row["Email"] = email
	g.key = email
	ops := append(a.invitationOps(actor, e.ID, nil), store.Insert(InvitesTab, row))
	if g.answer != "" {
		ops = append(ops, store.Upsert(RSVPsTab, store.Row{"Event ID": e.ID, "Email": email}, store.Row{"Answer": g.answer, "Answered": stamp, "Answered By": actor.Email, "Via": ViaPage}))
	}
	return ops, g, nil
}

func (a calendarApp) bringGuestOps(actor access.Actor, body guestBody) ([]store.Op, broughtGuest, error) {
	e, err := a.findEvent(actor, body.ID)
	if err != nil {
		return nil, broughtGuest{}, err
	}
	model := a.model()
	inv := model.Invitations[e.ID]
	g := broughtGuest{event: e, of: actor.Email, answer: AnswerYes, invite: true}
	if body.Answer != nil {
		g.answer = strings.ToLower(strings.TrimSpace(*body.Answer))
		if g.answer != "" && g.answer != AnswerYes {
			return nil, g, access.Invalid("a guest is put down as yes, or left to answer")
		}
	}
	if body.Invite != nil {
		g.invite = *body.Invite
	}
	if body.Of != "" {
		g.of = a.directory().Resolve(mail.Normalize(body.Of))
	}
	if !a.isHost(actor, e) {
		if inv != nil && !inv.Guests {
			return nil, g, access.Forbidden("this event is not taking guests")
		}
		if !a.mayAnswerFor(actor, g.of, e) {
			return nil, g, access.Forbidden("a guest comes with you or someone in your household")
		}
		if !e.guestsWithoutInvite() && model.InviteOf(e.ID, g.of) == nil {
			return nil, g, access.Forbidden("a guest comes with someone on the list")
		}
	}
	return a.guestOps(actor, g, body.Name, body.Email)
}

func (a calendarApp) extGuestOps(actor access.Actor, e *Event, name, email string) ([]store.Op, broughtGuest, error) {
	g := broughtGuest{event: e, of: actor.Email, answer: AnswerYes, invite: true}
	if settings := a.model().Invitations[e.ID]; settings == nil || !settings.Guests {
		return nil, g, access.Forbidden("this event is not taking guests")
	}
	return a.guestOps(actor, g, name, email)
}

func (a calendarApp) extRemoveGuestOps(actor access.Actor, e *Event, key string) ([]store.Op, string, error) {
	key = mail.Normalize(key)
	guest := a.model().InviteOf(e.ID, key)
	if guest == nil || guest.GuestOf != actor.Email {
		return nil, "", access.Missing("that is not a guest of yours")
	}
	return []store.Op{store.Delete(InvitesTab, store.Row{"Event ID": e.ID, "Email": key})}, key, nil
}

func (a calendarApp) answerOps(actor access.Actor, email, id, answer, via string, invite bool) ([]store.Op, *Event, error) {
	email = mail.Normalize(email)
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "" && !isAnswer(answer) {
		return nil, nil, access.Invalid("an answer is yes, no, maybe, or hidden")
	}
	e := a.eventFor(access.Actor{Email: email}, id)
	if e == nil {
		return nil, nil, access.Invalid(notOnCalendar)
	}
	key := store.Row{"Event ID": e.ID, "Email": email}
	if answer == "" {
		return []store.Op{store.Delete(RSVPsTab, key)}, e, nil
	}
	model := a.model()
	row := store.Row{"Answer": answer, "Answered": now().Format(DateTimeFormat), "Answered By": actor.Email, "Via": via, "Hosts Told": ""}
	if inv := model.Invitations[e.ID]; inv != nil && answer != AnswerHidden && slices.ContainsFunc(inv.Notify, func(h string) bool { return h != actor.Email }) {
		row["Hosts Told"] = owed
	}
	if sent := model.InviteOf(e.ID, email); invite && answer == AnswerYes && !isGuestKey(email) && (sent == nil || sent.Sent == "") {
		row["Invite Mail"] = owed
	}
	return []store.Op{store.Upsert(RSVPsTab, key, row)}, e, nil
}

func (a calendarApp) answerSubject(actor access.Actor, id, email, answer string) (*Event, string, error) {
	e, err := a.findEvent(actor, id)
	if err != nil {
		return nil, "", err
	}
	subject := mail.Normalize(email)
	if !isGuestKey(subject) {
		subject = a.directory().Resolve(subject)
	}
	if !a.mayAnswerFor(actor, subject, e) {
		return nil, "", access.Forbidden("you can answer for yourself and your household")
	}
	if strings.ToLower(strings.TrimSpace(answer)) == AnswerHidden {
		return nil, "", access.Invalid("hiding is a person's own")
	}
	return e, subject, nil
}

func (a calendarApp) extSubject(actor access.Actor, e *Event, key, answer string) (string, error) {
	if strings.ToLower(strings.TrimSpace(answer)) == AnswerHidden {
		return "", access.Invalid("an answer is yes, no, maybe, or blank")
	}
	key = mail.Normalize(key)
	if key == "" || key == actor.Email {
		return actor.Email, nil
	}
	if !slices.Contains(a.householdOn(e, actor.Email), key) {
		return "", access.Forbidden("that is not someone in your family")
	}
	return key, nil
}

func (a calendarApp) cancelWith(actor access.Actor, e *Event, body cancelBody) ([]store.Op, error) {
	ops, e, err := a.cancelOps(actor, e.ID, body.Note)
	if err != nil || len(ops) == 0 {
		return ops, err
	}
	sent := []string{}
	if body.Notify {
		for _, inv := range a.model().Invites[e.ID] {
			if inv.Sent != "" {
				sent = append(sent, inv.Email)
			}
		}
	}
	if len(sent) > 0 {
		ops = append(ops, a.messageOp(e, KindCancelled, "", strings.TrimSpace(body.Note), sent, false, actor.Email))
	}
	return ops, nil
}

func (a calendarApp) sendChoice(e *Event, body sendBody) ([]string, string, error) {
	model := a.model()
	emails := []string{}
	reminder := false
	if len(body.Emails) > 0 {
		body.To = "these"
	}
	for _, inv := range model.Invites[e.ID] {
		switch body.To {
		case "sent":
			if inv.Sent != "" && model.AnswerOf(inv.Email, e.ID) != AnswerNo {
				emails = append(emails, inv.Email)
			}
		case "these":
			if slices.Contains(body.Emails, inv.Email) {
				emails = append(emails, inv.Email)
				reminder = reminder || inv.Sent != ""
			}
		case "new", "":
			if inv.Sent == "" && inv.Requested == "" {
				emails = append(emails, inv.Email)
			}
		case "unanswered":
			if model.AnswerOf(inv.Email, e.ID) == "" {
				emails = append(emails, inv.Email)
				reminder = reminder || inv.Sent != ""
			}
		case "all":
			emails = append(emails, inv.Email)
			reminder = reminder || inv.Sent != ""
		default:
			return nil, "", access.Invalid("send to new, unanswered, sent, or all")
		}
	}
	if len(emails) == 0 {
		return nil, "", access.Invalid("nobody to send to")
	}
	switch {
	case body.Update:
		return emails, inviteUpdate, nil
	case reminder:
		return emails, inviteReminder, nil
	}
	return emails, "", nil
}

func (a calendarApp) skippable(e *Event, emails []string) []string {
	out := []string{}
	for _, email := range emails {
		email = mail.Normalize(email)
		if inv := a.model().InviteOf(e.ID, email); inv != nil && inv.Sent == "" {
			out = append(out, email)
		}
	}
	return out
}

func (a calendarApp) messageWith(actor access.Actor, e *Event, body messageBody) (store.Op, int, error) {
	message := strings.TrimSpace(body.Message)
	if message == "" || len(message) > maxTextLength {
		return store.Op{}, 0, access.Invalid("a message needs some words")
	}
	subject := strings.Join(strings.Fields(body.Subject), " ")
	if subject == "" || len(subject) > maxTitleLength {
		return store.Op{}, 0, access.Invalid("a message needs a subject")
	}
	wanted := map[string]bool{}
	for _, t := range body.To {
		switch t {
		case AnswerYes, AnswerMaybe, AnswerNo, "none":
			wanted[t] = true
		default:
			return store.Op{}, 0, access.Invalid("send to yes, maybe, no, or none")
		}
	}
	if len(wanted) == 0 && len(body.Emails) == 0 {
		return store.Op{}, 0, access.Invalid("pick who to send to")
	}
	model := a.model()
	standing := func(email string) string {
		if answer := model.AnswerOf(email, e.ID); answer != "" && answer != AnswerHidden {
			return answer
		}
		return "none"
	}
	chosen := []string{}
	for _, inv := range model.Invites[e.ID] {
		if wanted[standing(inv.Email)] || slices.Contains(body.Emails, inv.Email) {
			chosen = append(chosen, inv.Email)
		}
	}
	targets := a.reachable(e, chosen)
	if targets == 0 {
		return store.Op{}, 0, access.Invalid("nobody on the list stands where you chose")
	}
	return a.messageOp(e, KindMessage, subject, message, chosen, body.Attach, actor.Email), targets, nil
}

func (a calendarApp) requestOps(actor access.Actor, e *Event, emails []string, host, kind string) []store.Op {
	stamp := now().Format(DateTimeFormat)
	ops := []store.Op{}
	if kind == "" {
		for _, email := range emails {
			ops = append(ops, store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": email}, store.Row{"Requested": stamp, "Requested By": host}))
		}
	} else {
		ops = append(ops, a.messageOp(e, kind, "", "", emails, false, host))
	}
	return append(ops, a.sentOps(actor, e, emails, stamp)...)
}

func (a calendarApp) messageOp(e *Event, kind, subject, text string, recipients []string, attach bool, by string) store.Op {
	return store.Insert(MessagesTab, store.Row{
		"Message ID": a.model().Minter()(), "Event ID": e.ID, "Kind": kind, "Subject": subject, "Text": text, "Recipients": strings.Join(recipients, ", "),
		"Attach": cells.YesNoCell(attach), "Sent By": by, "Created": now().Format(DateTimeFormat),
	})
}

func (a calendarApp) sentOps(actor access.Actor, e *Event, emails []string, stamp string) []store.Op {
	model := a.model()
	ops := []store.Op{}
	if inv := model.Invitations[e.ID]; inv == nil || inv.Sent == "" {
		ops = append(ops, store.Update(InvitationsTab, store.Row{"Event ID": e.ID}, store.Row{"Sent": stamp}))
	}
	for _, g := range model.Groups[e.ID] {
		unsent := slices.ContainsFunc(model.Invites[e.ID], func(inv Invite) bool {
			return inv.Via == ViaGroup+g.ID && inv.Sent == "" && inv.Requested == "" && !slices.Contains(emails, inv.Email)
		})
		if g.Sent == "" && !unsent {
			ops = append(ops, store.Update(InviteGroupsTab, store.Row{"Event ID": e.ID, "Group ID": g.ID}, store.Row{"Sent": stamp}))
		}
	}
	return ops
}

func (a calendarApp) skipOps(actor access.Actor, e *Event, emails []string) []store.Op {
	stamp := now().Format(DateTimeFormat)
	ops := []store.Op{}
	for _, email := range emails {
		ops = append(ops, inviteSentOp(e.ID, email, stamp))
	}
	return append(ops, a.sentOps(actor, e, emails, stamp)...)
}

func eventCellsOp(id string, cells store.Row) store.Op {
	return store.Update(EventsTab, store.Row{"Event ID": id}, cells)
}

func resendOp(e *Event, to, host string) store.Op {
	return store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": to}, store.Row{"Sent": "", "Requested": now().Format(DateTimeFormat), "Requested By": host})
}

func inviteSentOp(id, email, stamp string) store.Op {
	return store.Update(InvitesTab, store.Row{"Event ID": id, "Email": email}, store.Row{"Sent": stamp})
}

func messageSentOp(m EventMessage, sent []string) store.Op {
	return store.Update(MessagesTab, store.Row{"Message ID": m.ID}, store.Row{"Sent To": strings.Join(sent, ", ")})
}

func answerToldOp(id, email string, told store.Row) store.Op {
	return store.Update(RSVPsTab, store.Row{"Event ID": id, "Email": email}, told)
}

func hostsToTellOp(id string, left []string) store.Op {
	return store.Update(InvitationsTab, store.Row{"Event ID": id}, store.Row{"Hosts To Tell": strings.Join(left, ", ")})
}

func adminsToldOp(id string) store.Op {
	return store.Update(EventsTab, store.Row{"Event ID": id}, store.Row{"Admins Told": now().Format(DateTimeFormat)})
}

func (a calendarApp) openedOps(actor access.Actor, e *Event) []store.Op {
	inv := a.model().InviteOf(e.ID, actor.Email)
	if inv == nil || inv.Opened != "" || (inv.Sent == "" && inv.Requested == "") {
		return nil
	}
	return []store.Op{store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": actor.Email}, store.Row{"Opened": now().Format(DateTimeFormat)})}
}

func (a calendarApp) checkRule(actor access.Actor, r Rule, e *Event) (Rule, error) {
	r = r.Clean()
	r.Kind = RuleInclude
	if err := r.Check(); err != nil {
		return r, err
	}
	options := a.sources().Options(actor.Email)
	for _, g := range r.Grades {
		if !slices.Contains(options.Grades, g) {
			return r, fmt.Errorf("the directory has no grade %s", g)
		}
	}
	for _, c := range r.Classrooms {
		if !slices.Contains(options.Classrooms, c) {
			return r, fmt.Errorf("the directory has no classroom %s", c)
		}
	}
	if err := a.sources().Writable(actor.Email, a.hostsOf(e), nil, []Rule{r}); err != nil {
		return r, err
	}
	return r, nil
}

func (a calendarApp) newGroupOps(actor access.Actor, e *Event, g InviteGroup) []store.Op {
	row := store.Row{"Event ID": e.ID, "Group ID": g.ID, "Auto": cells.YesNoCell(g.Auto), "Added By": g.AddedBy, "Added": g.Added}
	maps.Copy(row, g.Rule.Cells())
	return append(a.invitationOps(actor, e.ID, nil), store.Insert(InviteGroupsTab, row))
}

func (a calendarApp) addGroupOps(actor access.Actor, id string, rule Rule, auto *bool) ([]store.Op, *Event, InviteGroup, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, InviteGroup{}, err
	}
	rule, err = a.checkRule(actor, rule, e)
	if err != nil {
		return nil, nil, InviteGroup{}, access.Invalid("%v", err)
	}
	if len(a.model().Groups[e.ID]) >= 20 {
		return nil, nil, InviteGroup{}, access.Invalid("a guest list holds twenty groups at most")
	}
	g := InviteGroup{ID: a.model().Minter()(), Rule: rule, Auto: auto == nil || *auto, AddedBy: actor.Email, Added: now().Format(DateTimeFormat)}
	return a.newGroupOps(actor, e, g), e, g, nil
}

func (a calendarApp) setGroupOps(actor access.Actor, id, gid string, auto bool) ([]store.Op, *Event, *InviteGroup, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, nil, err
	}
	g := a.model().GroupOf(e.ID, gid)
	if g == nil {
		return nil, nil, nil, access.Missing("that group is not on the list")
	}
	return []store.Op{store.Update(InviteGroupsTab, store.Row{"Event ID": e.ID, "Group ID": g.ID}, store.Row{"Auto": cells.YesNoCell(auto)})}, e, g, nil
}

func (a calendarApp) removeGroupOps(actor access.Actor, id, gid string) ([]store.Op, *Event, *InviteGroup, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, nil, err
	}
	model := a.model()
	g := model.GroupOf(e.ID, gid)
	if g == nil {
		return nil, nil, nil, access.Missing("that group is not on the list")
	}
	ops := []store.Op{store.Delete(InviteGroupsTab, store.Row{"Event ID": e.ID, "Group ID": g.ID})}
	for _, inv := range model.Invites[e.ID] {
		if inv.Via == ViaGroup+g.ID && inv.Sent == "" {
			ops = append(ops, store.Delete(InvitesTab, store.Row{"Event ID": e.ID, "Email": inv.Email}))
		}
	}
	return ops, e, g, nil
}

func (a calendarApp) startPartyOps(actor access.Actor, id string) ([]store.Op, *Event, InviteGroup, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, InviteGroup{}, err
	}
	if !e.linked() {
		return nil, nil, InviteGroup{}, access.Invalid("only a party or an HCA event starts this way")
	}
	key := "party:" + e.linkedID()
	if e.Source != SourceCelebrate {
		key = "activity:" + e.linkedID()
	}
	for _, g := range a.model().Groups[e.ID] {
		if slices.Contains(g.Rule.Tags, key) {
			return nil, e, g, nil
		}
	}
	g := InviteGroup{ID: a.model().Minter()(), Rule: Rule{Kind: RuleInclude, Tags: []string{key}}, Auto: true, AddedBy: actor.Email, Added: now().Format(DateTimeFormat)}
	return append(a.newGroupOps(actor, e, g), a.hostYesOps(actor, e, a.hostsOf(e))...), e, g, nil
}

func (a calendarApp) fillOps(actor access.Actor, e *Event, g InviteGroup) ([]store.Op, []string) {
	model := a.model()
	stamp := now().Format(DateTimeFormat)
	ops := []store.Op{}
	emails := []string{}
	guests := a.ticketGuests(g)
	for _, email := range a.members(e, g) {
		if model.InviteOf(e.ID, email) != nil || slices.Contains(g.Removed, email) {
			continue
		}
		name, token := email, ""
		if p := a.directory().Person(email); p != nil {
			name = p.FullName
		} else if guest, ok := guests[email]; ok {
			name, token = guest, id.Token()
			if name == "" {
				name = cells.DisplayName(email)
			}
		}
		row := store.Row{"Event ID": e.ID, "Email": email, "Name": name, "Via": ViaGroup + g.ID, "Added By": g.AddedBy, "Added": stamp, "Token": token}
		if g.Auto && g.Sent != "" {
			row["Requested"], row["Requested By"] = stamp, g.AddedBy
		}
		ops = append(ops, store.Insert(InvitesTab, row))
		emails = append(emails, email)
	}
	return ops, emails
}

func (a calendarApp) sweptEvent(adder access.Actor, id string) *Event {
	e := a.eventFor(access.Actor{Email: adder.Email}, id)
	if e == nil || !a.isHost(adder, e) {
		return nil
	}
	return e
}

func bounceOps(actor access.Actor, email, reason string) []store.Op {
	return []store.Op{store.Insert(BouncesTab, store.Row{"Email": email, "When": now().Format(DateTimeFormat), "Reason": reason})}
}

type addressChange struct {
	event      *Event
	from, to   string
	name       string
	everywhere bool
}

func (a calendarApp) changeAddressOps(actor access.Actor, id, email, to string, everywhere bool) ([]store.Op, addressChange, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, addressChange{}, err
	}
	model := a.model()
	c := addressChange{event: e, from: mail.Normalize(email), to: mail.Normalize(to)}
	inv := model.InviteOf(e.ID, c.from)
	switch {
	case inv == nil:
		return nil, c, access.Missing("that person is not on the list")
	case isGuestKey(c.from):
		return nil, c, access.Invalid("a guest named without an address has none to change")
	case !emailForm.MatchString(c.to):
		return nil, c, access.Invalid("that is not an email address")
	case c.to == c.from:
		return nil, c, nil
	case model.InviteOf(e.ID, c.to) != nil && !everywhere:
		return nil, c, access.Invalid("that address is on the list already")
	}
	if a.directory().Person(c.from) != nil {
		return nil, c, access.Invalid("their address is the directory's to change")
	}
	c.name = inv.Name
	if everywhere {
		if e.Source != SourceCelebrate || !a.all().AdminList("celebrate").IsAdmin(actor.Email) {
			return nil, c, access.Forbidden("only Celebrate's admins move an address on every party")
		}
		c.everywhere = true
		return nil, c, nil
	}
	return []store.Op{store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": c.from}, store.Row{"Email": c.to})}, c, nil
}

func (a calendarApp) moveAddressOps(actor access.Actor, old, to, name string) ([]store.Op, []string) {
	model := a.model()
	person := a.directory().Person(to)
	ops := []store.Op{}
	resend := []string{}
	for key := range model.Invites {
		if a.parties().PartyPeople(key) == nil {
			continue
		}
		row := model.InviteOf(key, old)
		if row == nil {
			continue
		}
		match := store.Row{"Event ID": key, "Email": old}
		if model.InviteOf(key, to) != nil {
			ops = append(ops, store.Delete(InvitesTab, match))
		} else {
			cells := store.Row{"Email": to, "Token": "", "Household": ""}
			switch {
			case person != nil:
				cells["Name"] = person.FullName
			default:
				cells["Token"], cells["Household"] = row.Token, row.Household
				if cells["Token"] == "" {
					cells["Token"] = id.Token()
				}
				if name != "" {
					cells["Name"] = name
				}
			}
			ops = append(ops, store.Update(InvitesTab, match, cells))
			if row.Sent != "" {
				if inv := model.Invitations[key]; inv != nil && inv.Sent != "" {
					resend = append(resend, key)
				}
			}
		}
		ops = append(ops,
			store.Update(InvitesTab, store.Row{"Event ID": key, "Household": old}, store.Row{"Household": to}),
			store.Update(InvitesTab, store.Row{"Event ID": key, "Guest Of": old}, store.Row{"Guest Of": to}),
		)
	}
	return ops, resend
}

func (a calendarApp) deleteInvitationOps(actor access.Actor, id string) ([]store.Op, *Event, bool, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, false, err
	}
	inv := a.model().Invitations[e.ID]
	own := e.Source == SourceSheet
	if own && inv != nil && inv.Sent != "" {
		return nil, nil, false, access.Invalid("the invites are out: cancel the event instead")
	}
	if own {
		return []store.Op{store.Delete(EventsTab, store.Row{"Event ID": e.ID})}, e, true, nil
	}
	return []store.Op{store.Delete(InvitationsTab, store.Row{"Event ID": e.ID})}, e, false, nil
}

func (a calendarApp) cancelOps(actor access.Actor, id, note string) ([]store.Op, *Event, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, err
	}
	if e.Source != SourceSheet {
		return nil, nil, access.Invalid("an event another app runs is cancelled there")
	}
	if e.Cancelled {
		return nil, e, nil
	}
	if len(strings.TrimSpace(note)) > maxTextLength {
		return nil, nil, access.Invalid("the note is too long")
	}
	return []store.Op{store.Update(EventsTab, store.Row{"Event ID": e.ID}, store.Row{"Status": StatusCancelled})}, e, nil
}
