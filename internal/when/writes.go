package when

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/filter"
	"heliosian/internal/serve"
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

func (a app) newFeed(actor access.Actor, body feedBody) ([]store.Op, store.Row) {
	cells := feedCells(body)
	cells["Token"], cells["Email"], cells["Created"] = serve.ID(24), actor.Email, now().Format(DateTimeFormat)
	cells["Name"] = a.cache.Model().unusedFeedName(actor.Email, strings.TrimSpace(body.Name))
	return []store.Op{store.Insert(FeedsTab, cells)}, cells
}

func (a app) changeFeed(actor access.Actor, body feedBody) ([]store.Op, store.Row, error) {
	if strings.TrimSpace(body.Name) == "" {
		return nil, nil, access.Invalid("a feed needs a name")
	}
	if strings.TrimSpace(body.Token) == MyHeliosianToken {
		return []store.Op{homeOp(actor.Email, store.Row{"Home Name": strings.TrimSpace(body.Name), "Home Emoji": feedEmoji(body.Emoji)})}, nil, nil
	}
	f := a.cache.Model().Feed(strings.TrimSpace(body.Token))
	if f == nil {
		return nil, nil, access.Missing("no such feed")
	}
	if f.Email != actor.Email && !actor.May(FeedsForAnyone) {
		return nil, nil, access.Forbidden("only the person who made a feed, or an admin, can change it")
	}
	cells := feedCells(body)
	return []store.Op{store.Update(FeedsTab, store.Row{"Token": f.Token}, cells)}, cells, nil
}

func (a app) dropFeed(actor access.Actor, token string) ([]store.Op, string, error) {
	f := a.cache.Model().Feed(strings.TrimSpace(token))
	if f == nil {
		return nil, "", access.Missing("no such feed")
	}
	if f.Email != actor.Email && !actor.May(FeedsForAnyone) {
		return nil, "", access.Forbidden("only the person who made a feed, or an admin, can remove it")
	}
	return []store.Op{store.Delete(FeedsTab, store.Row{"Token": f.Token})}, f.Name, nil
}

func (a app) orderOps(actor access.Actor, tokens []string) ([]store.Op, error) {
	ops := []store.Op{}
	if at := slices.Index(tokens, MyHeliosianToken); at >= 0 {
		ops = append(ops, homeOp(actor.Email, store.Row{"Home Position": strconv.Itoa(at)}))
		tokens = slices.Delete(slices.Clone(tokens), at, at+1)
	}
	mine := map[string]string{}
	for _, f := range a.cache.Model().Feeds {
		if config.NormalizeEmail(f.Email) == actor.Email {
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

func (a app) defaultOps(actor access.Actor, token string) ([]store.Op, []string, error) {
	tokens := []string{token}
	found := false
	for _, f := range a.cache.Model().MyCalendars(actor.Email) {
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

func (a app) feedTokenOps(actor access.Actor) ([]store.Op, string) {
	if token := a.cache.Model().Settings[actor.Email].FeedToken; token != "" {
		return nil, token
	}
	token := serve.ID(24)
	return []store.Op{homeOp(actor.Email, store.Row{"Feed Token": token})}, token
}

func saveViewOps(actor access.Actor, classrooms, tags []string) ([]store.Op, store.Row) {
	row := store.Row{
		"Classrooms": cells.JoinList(cells.SplitList(cells.JoinList(classrooms))), "Categories": cells.JoinList(cells.SplitList(cells.JoinList(tags))), "Saved": now().Format(DateTimeFormat),
	}
	return []store.Op{store.Upsert(SettingsTab, store.Row{"Email": actor.Email}, row)}, row
}

func (a app) forgetViewOps(actor access.Actor) []store.Op {
	if _, ok := a.cache.Model().Settings[actor.Email]; !ok {
		return nil
	}
	return []store.Op{store.Update(SettingsTab, store.Row{"Email": actor.Email}, store.Row{"Classrooms": "", "Categories": "", "Saved": ""})}
}

var (
	SeeAll         = access.Standing("when.see-all")
	Curate         = access.Standing("when.curate")
	ActAsHost      = access.Acting("when.act-as-host")
	FeedsForAnyone = access.Acting("when.feeds-for-anyone")
)

var AdminAllowances = []access.Allowance{SeeAll, Curate, ActAsHost, FeedsForAnyone}

func adminOnly(actor access.Actor) error {
	if !actor.May(Curate) {
		return access.Forbidden("only a calendar admin can do that")
	}
	return nil
}

func (a app) keywordOps(actor access.Actor, id string, keywords []string) ([]store.Op, *Event, string, error) {
	if err := adminOnly(actor); err != nil {
		return nil, nil, "", err
	}
	e := a.cache.Model().Event(id)
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

func (a app) newEvents(actor access.Actor, body eventBody) ([]store.Op, []string, bool, error) {
	if !slices.Contains(sharingWords, body.Sharing) {
		return nil, nil, false, access.Invalid("sharing is %s", strings.Join(sharingWords, ", "))
	}
	if body.RepeatTimes < 0 || body.RepeatTimes > 52 || body.RepeatWeeks < 1 && body.RepeatTimes > 0 {
		return nil, nil, false, access.Invalid("repeat up to 52 more times, some whole number of weeks apart")
	}
	chosen := strings.ToLower(strings.TrimSpace(body.ID))
	if chosen != "" {
		if !eventIDForm.MatchString(chosen) {
			return nil, nil, false, access.Invalid("a web address is 3 to 40 letters, digits and dashes")
		}
		if a.cache.Model().Event(chosen) != nil || a.cache.Count(EventsTab, store.Row{"Event ID": chosen}) > 0 {
			return nil, nil, false, access.Invalid("that web address is taken")
		}
	}
	if !actor.May(Curate) {
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
	for i := 0; i <= body.RepeatTimes; i++ {
		id := newEventID()
		if i == 0 && chosen != "" {
			id = chosen
		}
		ids = append(ids, id)
		ops = append(ops, store.Insert(EventsTab, store.Row{
			"Event ID": id, "Start": shiftWhen(strings.TrimSpace(body.Start), i*body.RepeatWeeks), "End": shiftWhen(strings.TrimSpace(body.End), i*body.RepeatWeeks),
			"Title": strings.TrimSpace(body.Title), "Location": strings.TrimSpace(body.Location), "Description": strings.TrimSpace(body.Description),
			"Tags": cells.JoinList(cells.SplitList(cells.JoinList(body.Tags))), "Day Type": strings.TrimSpace(body.DayType), "Keywords": cells.JoinList(cells.SplitList(cells.JoinList(body.Keywords))),
			"Added By": actor.Email, "Added": stamp, "Source": strings.TrimSpace(body.Source), "Sharing": body.Sharing, "Status": status, "Image": strings.Trim(strings.TrimSpace(body.Image), "/"),
		}))
	}
	return ops, ids, pending, nil
}

func (a app) changeEvent(actor access.Actor, body eventBody) ([]store.Op, *Event, store.Row, error) {
	if !slices.Contains(sharingWords, body.Sharing) {
		return nil, nil, nil, access.Invalid("sharing is %s", strings.Join(sharingWords, ", "))
	}
	e := a.cache.Model().Event(strings.TrimSpace(body.ID))
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
		}
	}
	return []store.Op{store.Update(EventsTab, store.Row{"Event ID": e.ID}, row)}, e, row, nil
}

func (a app) statusOps(actor access.Actor, id, status string) ([]store.Op, *Event, error) {
	if err := adminOnly(actor); err != nil {
		return nil, nil, err
	}
	e := a.cache.Model().Event(strings.TrimSpace(id))
	if e == nil || e.Source != SourceSheet {
		return nil, nil, access.Missing("that event is not one added by hand")
	}
	if status == StatusDeclined && e.Sharing != SharingPublic {
		return nil, nil, access.Invalid("only a public event is declined")
	}
	return []store.Op{store.Update(EventsTab, store.Row{"Event ID": e.ID}, store.Row{"Status": status})}, e, nil
}

func (a app) moveOps(actor access.Actor, id, start, end string) ([]store.Op, *Event, error) {
	if err := adminOnly(actor); err != nil {
		return nil, nil, err
	}
	e := a.cache.Model().Event(id)
	if e == nil || e.Source != SourceSheet {
		return nil, nil, access.Invalid("only an event of the Events tab moves from here")
	}
	return []store.Op{store.Update(EventsTab, store.Row{"Event ID": e.ID}, store.Row{"Start": start, "End": end})}, e, nil
}

func (a app) tagOps(actor access.Actor, tags []tagBody) ([]store.Op, int, error) {
	if !actor.May(Curate) {
		return nil, 0, access.Forbidden("only a calendar admin can change the categories")
	}
	current := map[string]Tag{}
	for _, t := range a.cache.Model().Tags {
		current[t.Name] = t
	}
	names, orders, rows := []string{}, []string{}, []store.Row{}
	for _, t := range tags {
		name, description, group, image := strings.TrimSpace(t.Name), strings.TrimSpace(t.Description), strings.TrimSpace(t.Group), strings.TrimSpace(t.Image)
		if name == "" || slices.Contains(names, name) {
			return nil, 0, access.Invalid("every category needs a name of its own")
		}
		if description == "" {
			return nil, 0, access.Invalid("%q needs a description", name)
		}
		if _, ok := current[name]; !ok && slices.ContainsFunc(builtinTags, func(b Tag) bool { return b.Name == name }) {
			return nil, 0, access.Invalid("%q is built in and has no row of its own", name)
		}
		names = append(names, name)
		orders = append(orders, current[name].order)
		rows = append(rows, store.Row{"Description": description, "Group": group, "Default": cells.YesNoCell(t.Default), "Image": image})
	}
	for name := range current {
		if !slices.Contains(names, name) {
			return nil, 0, access.Invalid("%q is missing - a category cannot be removed from here", name)
		}
	}
	keys := store.Order(orders)
	ops := []store.Op{}
	added := 0
	for i, name := range names {
		cells := rows[i]
		cells[store.OrderColumn] = keys[i]
		was, ok := current[name]
		if !ok {
			cells["Tag"] = name
			ops = append(ops, store.Insert(TagsTab, cells))
			added++
			continue
		}
		if tags[i].Default == was.Default {
			delete(cells, "Default")
		}
		ops = append(ops, store.Update(TagsTab, store.Row{"Tag": name}, cells))
	}
	return ops, added, nil
}

func (a app) mayCorrect(actor access.Actor, e *Event) error {
	if !actor.May(Curate) && !a.isHost(actor, e) {
		return access.Forbidden("only a host of an event, or an admin, can change it")
	}
	return nil
}

func (a app) overrideOps(actor access.Actor, body overrideBody) ([]store.Op, string, bool, error) {
	model := a.cache.Model()
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
	builtIn := map[string]bool{}
	for _, t := range model.Tags {
		builtIn[t.Name] = t.BuiltIn
	}
	list := func(column string, want, was []string) {
		want = cells.SplitList(cells.JoinList(want))
		was = slices.DeleteFunc(slices.Clone(was), func(t string) bool { return builtIn[t] })
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
	if body.Note != nil && actor.May(Curate) {
		row["Note"] = strings.TrimSpace(*body.Note)
	}
	row["Address"] = strings.ToLower(strings.TrimSpace(body.Address))
	if row["Address"] != "" && !eventIDForm.MatchString(row["Address"]) {
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

func (a app) overrideImageOps(actor access.Actor, id, image string) ([]store.Op, *Event, string, error) {
	e := a.cache.Model().Event(strings.TrimSpace(id))
	if e == nil || !e.imported() {
		return nil, nil, "", access.Missing("only an event the school's calendars bring takes its picture here")
	}
	if err := a.mayCorrect(actor, e); err != nil {
		return nil, nil, "", err
	}
	image = strings.Trim(strings.TrimSpace(image), "/")
	return []store.Op{store.Upsert(OverridesTab, store.Row{"Event ID": e.ID}, store.Row{"Image": image})}, e, image, nil
}

func (a app) invitationOps(actor access.Actor, id string, cells store.Row) []store.Op {
	if a.cache.Model().Invitations[id] != nil {
		if len(cells) == 0 {
			return nil
		}
		return []store.Op{store.Update(InvitationsTab, store.Row{"Event ID": id}, cells)}
	}
	row := store.Row{"Event ID": id, "Audience": "Both", "Guests": "Yes", "Created By": actor.Email, "Created": now().Format(DateTimeFormat)}
	maps.Copy(row, cells)
	return []store.Op{store.Insert(InvitationsTab, row)}
}

func (a app) settingsOps(actor access.Actor, body settingsBody) ([]store.Op, *Event, []string, error) {
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
		if inv := a.cache.Model().Invitations[e.ID]; inv != nil {
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
			h = a.directory().Resolve(config.NormalizeEmail(h))
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
	}
	return append(a.invitationOps(actor, e.ID, row), a.hostYesOps(actor, e, newHosts)...), e, newHosts, nil
}

func (a app) hostYesOps(actor access.Actor, e *Event, hosts []string) []store.Op {
	model := a.cache.Model()
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

func (a app) stepDownOps(actor access.Actor, id, email string) ([]store.Op, *Event, string, bool, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, "", false, err
	}
	who := actor.Email
	if email := a.directory().Resolve(config.NormalizeEmail(email)); email != "" {
		who = email
	}
	inv := a.cache.Model().Invitations[e.ID]
	cohost := inv != nil && slices.Contains(inv.Hosts, who)
	poster := e.Source == SourceSheet && !e.PosterLeft && a.directory().Resolve(config.NormalizeEmail(e.AddedBy)) == who
	if !cohost && !poster {
		return nil, nil, "", false, access.Invalid("that person hosts this event on the app that runs it, or not at all - step down there")
	}
	row := store.Row{}
	if poster {
		row["Stepped Down"] = config.NormalizeEmail(e.AddedBy)
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

func (a app) inviteOps(actor access.Actor, id string, people []invitee) ([]store.Op, *Event, []string, bool, error) {
	e, host, err := a.inviterEvent(actor, id)
	if err != nil {
		return nil, nil, nil, false, err
	}
	if !host && len(people) > 20 {
		return nil, nil, nil, false, access.Invalid("invite up to twenty people at a time")
	}
	if len(people) == 0 || len(people) > 500 {
		return nil, nil, nil, false, access.Invalid("add between one and five hundred people at a time")
	}
	model := a.cache.Model()
	stamp := now().Format(DateTimeFormat)
	ops := a.invitationOps(actor, e.ID, nil)
	emails := []string{}
	for _, p := range people {
		name := strings.TrimSpace(p.Name)
		household := config.NormalizeEmail(p.Household)
		email := a.directory().Resolve(config.NormalizeEmail(p.Email))
		token := ""
		switch {
		case email == "" && household != "" && name != "":
			email = newGuestKey()
		case !emailForm.MatchString(email):
			return nil, nil, nil, false, access.Invalid("%q is not an email address", p.Email)
		default:
			token = serve.ID(24)
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

func (a app) uninviteOps(actor access.Actor, id, email string) ([]store.Op, *Event, string, bool, error) {
	e, err := a.findEvent(actor, id)
	if err != nil {
		return nil, nil, "", false, err
	}
	email = config.NormalizeEmail(email)
	inv := a.cache.Model().InviteOf(e.ID, email)
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
	group := a.cache.Model().GroupOf(e.ID, gid)
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

func (a app) guestOps(actor access.Actor, g broughtGuest, name, email string) ([]store.Op, broughtGuest, error) {
	model := a.cache.Model()
	e := g.event
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxTitleLength {
		return nil, g, access.Invalid("a guest needs a name")
	}
	email = config.NormalizeEmail(email)
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
			row["Token"] = serve.ID(24)
		}
	} else {
		email = newGuestKey()
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

func (a app) bringGuestOps(actor access.Actor, body guestBody) ([]store.Op, broughtGuest, error) {
	e, err := a.findEvent(actor, body.ID)
	if err != nil {
		return nil, broughtGuest{}, err
	}
	model := a.cache.Model()
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
		g.of = a.directory().Resolve(config.NormalizeEmail(body.Of))
	}
	if !a.isHost(actor, e) {
		if inv != nil && !inv.Guests {
			return nil, g, access.Forbidden("this event is not taking guests")
		}
		if !a.mayAnswerFor(actor, g.of, e) {
			return nil, g, access.Forbidden("a guest comes with you or someone in your household")
		}
		open := (e.Sharing == SharingPublic || e.Sharing == SharingLink) && e.Source != SourceCelebrate
		if !open && model.InviteOf(e.ID, g.of) == nil {
			return nil, g, access.Forbidden("a guest comes with someone on the list")
		}
	}
	return a.guestOps(actor, g, body.Name, body.Email)
}

func (a app) extGuestOps(actor access.Actor, e *Event, name, email string) ([]store.Op, broughtGuest, error) {
	g := broughtGuest{event: e, of: actor.Email, answer: AnswerYes, invite: true}
	if settings := a.cache.Model().Invitations[e.ID]; settings == nil || !settings.Guests {
		return nil, g, access.Forbidden("this event is not taking guests")
	}
	return a.guestOps(actor, g, name, email)
}

func (a app) extRemoveGuestOps(actor access.Actor, e *Event, key string) ([]store.Op, string, error) {
	key = config.NormalizeEmail(key)
	guest := a.cache.Model().InviteOf(e.ID, key)
	if guest == nil || guest.GuestOf != actor.Email {
		return nil, "", access.Missing("that is not a guest of yours")
	}
	return []store.Op{store.Delete(InvitesTab, store.Row{"Event ID": e.ID, "Email": key})}, key, nil
}

func (a app) answerOps(actor access.Actor, email, id, answer, via string) ([]store.Op, *Event, error) {
	email = config.NormalizeEmail(email)
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
	return []store.Op{store.Upsert(RSVPsTab, key, store.Row{"Answer": answer, "Answered": now().Format(DateTimeFormat), "Answered By": actor.Email, "Via": via})}, e, nil
}

func (a app) answerSubject(actor access.Actor, id, email, answer string) (*Event, string, error) {
	e, err := a.findEvent(actor, id)
	if err != nil {
		return nil, "", err
	}
	subject := config.NormalizeEmail(email)
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

func (a app) extSubject(actor access.Actor, e *Event, key, answer string) (string, error) {
	if strings.ToLower(strings.TrimSpace(answer)) == AnswerHidden {
		return "", access.Invalid("an answer is yes, no, maybe, or blank")
	}
	key = config.NormalizeEmail(key)
	if key == "" || key == actor.Email {
		return actor.Email, nil
	}
	if !slices.Contains(a.householdOn(e, actor.Email), key) {
		return "", access.Forbidden("that is not someone in your family")
	}
	return key, nil
}

func (a app) sentOps(actor access.Actor, e *Event, emails []string) []store.Op {
	model := a.cache.Model()
	stamp := now().Format(DateTimeFormat)
	ops := []store.Op{}
	for _, email := range emails {
		ops = append(ops, store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": email}, store.Row{"Sent": stamp}))
	}
	if inv := model.Invitations[e.ID]; inv != nil && inv.Sent == "" {
		ops = append(ops, store.Update(InvitationsTab, store.Row{"Event ID": e.ID}, store.Row{"Sent": stamp}))
	}
	for _, g := range model.Groups[e.ID] {
		unsent := slices.ContainsFunc(model.Invites[e.ID], func(inv Invite) bool {
			return inv.Via == ViaGroup+g.ID && inv.Sent == "" && !slices.Contains(emails, inv.Email)
		})
		if g.Sent == "" && !unsent {
			ops = append(ops, store.Update(InviteGroupsTab, store.Row{"Event ID": e.ID, "Group ID": g.ID}, store.Row{"Sent": stamp}))
		}
	}
	return ops
}

func (a app) openedOps(actor access.Actor, e *Event) []store.Op {
	inv := a.cache.Model().InviteOf(e.ID, actor.Email)
	if inv == nil || inv.Opened != "" || inv.Sent == "" {
		return nil
	}
	return []store.Op{store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": actor.Email}, store.Row{"Opened": now().Format(DateTimeFormat)})}
}

func (a app) checkRule(actor access.Actor, r filter.Rule, e *Event) (filter.Rule, error) {
	r = filter.Clean(r)
	r.Kind = filter.KindInclude
	if err := filter.Check(r); err != nil {
		return r, err
	}
	options := filter.OptionsFor(a.sources(), actor.Email)
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
	if err := filter.Writable(a.sources(), actor.Email, a.hostsOf(e), nil, []filter.Rule{r}); err != nil {
		return r, err
	}
	return r, nil
}

func (a app) newGroupOps(actor access.Actor, e *Event, g InviteGroup) []store.Op {
	row := store.Row{"Event ID": e.ID, "Group ID": g.ID, "Auto": cells.YesNoCell(g.Auto), "Added By": g.AddedBy, "Added": g.Added}
	maps.Copy(row, filter.RuleCells(g.Rule))
	return append(a.invitationOps(actor, e.ID, nil), store.Insert(InviteGroupsTab, row))
}

func (a app) addGroupOps(actor access.Actor, id string, rule filter.Rule, auto *bool) ([]store.Op, *Event, InviteGroup, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, InviteGroup{}, err
	}
	rule, err = a.checkRule(actor, rule, e)
	if err != nil {
		return nil, nil, InviteGroup{}, access.Invalid("%v", err)
	}
	if len(a.cache.Model().Groups[e.ID]) >= 20 {
		return nil, nil, InviteGroup{}, access.Invalid("a guest list holds twenty groups at most")
	}
	g := InviteGroup{ID: strings.ToLower(newEventID()), Rule: rule, Auto: auto == nil || *auto, AddedBy: actor.Email, Added: now().Format(DateTimeFormat)}
	return a.newGroupOps(actor, e, g), e, g, nil
}

func (a app) setGroupOps(actor access.Actor, id, gid string, auto bool) ([]store.Op, *Event, *InviteGroup, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, nil, err
	}
	g := a.cache.Model().GroupOf(e.ID, gid)
	if g == nil {
		return nil, nil, nil, access.Missing("that group is not on the list")
	}
	return []store.Op{store.Update(InviteGroupsTab, store.Row{"Event ID": e.ID, "Group ID": g.ID}, store.Row{"Auto": cells.YesNoCell(auto)})}, e, g, nil
}

func (a app) removeGroupOps(actor access.Actor, id, gid string) ([]store.Op, *Event, *InviteGroup, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, nil, err
	}
	model := a.cache.Model()
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

func (a app) startPartyOps(actor access.Actor, id string) ([]store.Op, *Event, InviteGroup, error) {
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
	for _, g := range a.cache.Model().Groups[e.ID] {
		if slices.Contains(g.Rule.Tags, key) {
			return nil, e, g, nil
		}
	}
	g := InviteGroup{ID: strings.ToLower(newEventID()), Rule: filter.Rule{Kind: filter.KindInclude, Tags: []string{key}}, Auto: true, AddedBy: actor.Email, Added: now().Format(DateTimeFormat)}
	return append(a.newGroupOps(actor, e, g), a.hostYesOps(actor, e, a.hostsOf(e))...), e, g, nil
}

func (a app) fillOps(actor access.Actor, e *Event, g InviteGroup, wait bool) ([]store.Op, []string) {
	model := a.cache.Model()
	at := now()
	stamp := at.Format(DateTimeFormat)
	ops := []store.Op{}
	emails := []string{}
	matching := a.members(e, g)
	guests := a.ticketGuests(g)
	if wait {
		a.clock.keep(e.ID, g.ID, matching)
	}
	for _, email := range matching {
		if model.InviteOf(e.ID, email) != nil || slices.Contains(g.Removed, email) {
			continue
		}
		if wait && !a.clock.ripe(e.ID, g.ID, email, at) {
			slog.Debug("calendar: group match waits the grace", "event", e.ID, "group", g.ID, "email", email)
			continue
		}
		name, token := email, ""
		if p := a.directory().Person(email); p != nil {
			name = p.FullName
		} else if guest, ok := guests[email]; ok {
			name, token = guest, serve.ID(24)
			if name == "" {
				name = cells.DisplayName(email)
			}
		}
		ops = append(ops, store.Insert(InvitesTab, store.Row{"Event ID": e.ID, "Email": email, "Name": name, "Via": ViaGroup + g.ID, "Added By": g.AddedBy, "Added": stamp, "Token": token}))
		emails = append(emails, email)
	}
	return ops, emails
}

func (a app) sweptEvent(adder access.Actor, id string) *Event {
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

func (a app) changeAddressOps(actor access.Actor, id, email, to string, everywhere bool) ([]store.Op, addressChange, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, addressChange{}, err
	}
	model := a.cache.Model()
	c := addressChange{event: e, from: config.NormalizeEmail(email), to: config.NormalizeEmail(to)}
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
		if e.Source != SourceCelebrate || !a.celebrate.IsAdmin(actor.Email) {
			return nil, c, access.Forbidden("only Celebrate's admins move an address on every party")
		}
		c.everywhere = true
		return nil, c, nil
	}
	return []store.Op{store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": c.from}, store.Row{"Email": c.to})}, c, nil
}

func (a app) moveAddressOps(actor access.Actor, old, to, name string) ([]store.Op, []string) {
	model := a.cache.Model()
	person := a.directory().Person(to)
	ops := []store.Op{}
	resend := []string{}
	for id := range model.Invites {
		if !strings.HasPrefix(id, SourceCelebrate+"/") {
			continue
		}
		row := model.InviteOf(id, old)
		if row == nil {
			continue
		}
		match := store.Row{"Event ID": id, "Email": old}
		if model.InviteOf(id, to) != nil {
			ops = append(ops, store.Delete(InvitesTab, match))
		} else {
			cells := store.Row{"Email": to, "Token": "", "Household": ""}
			switch {
			case person != nil:
				cells["Name"] = person.FullName
			default:
				cells["Token"], cells["Household"] = row.Token, row.Household
				if cells["Token"] == "" {
					cells["Token"] = serve.ID(24)
				}
				if name != "" {
					cells["Name"] = name
				}
			}
			ops = append(ops, store.Update(InvitesTab, match, cells))
			if row.Sent != "" {
				if inv := model.Invitations[id]; inv != nil && inv.Sent != "" {
					resend = append(resend, id)
				}
			}
		}
		ops = append(ops,
			store.Update(InvitesTab, store.Row{"Event ID": id, "Household": old}, store.Row{"Household": to}),
			store.Update(InvitesTab, store.Row{"Event ID": id, "Guest Of": old}, store.Row{"Guest Of": to}),
		)
	}
	return ops, resend
}

func (a app) deleteInvitationOps(actor access.Actor, id string) ([]store.Op, *Event, bool, error) {
	e, err := a.hostedEvent(actor, id)
	if err != nil {
		return nil, nil, false, err
	}
	inv := a.cache.Model().Invitations[e.ID]
	own := e.Source == SourceSheet
	if own && inv != nil && inv.Sent != "" {
		return nil, nil, false, access.Invalid("the invites are out: cancel the event instead")
	}
	if own {
		return []store.Op{store.Delete(EventsTab, store.Row{"Event ID": e.ID})}, e, true, nil
	}
	return []store.Op{store.Delete(InvitationsTab, store.Row{"Event ID": e.ID})}, e, false, nil
}

func (a app) cancelOps(actor access.Actor, id, note string) ([]store.Op, *Event, error) {
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
