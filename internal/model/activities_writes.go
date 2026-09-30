package model

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

func (m *Activities) find(id string) (*Activity, error) {
	act := m.Activity(strings.TrimSpace(id))
	if act == nil {
		return nil, access.Missing("no activity with id %q", id)
	}
	return act, nil
}

var (
	SeeAllActivities    = access.Named("team.see-all")
	CurateActivities    = access.Named("team.curate")
	ConfigureActivities = access.Named("team.configure")
	ActAsCochair        = access.Named("team.act-as-cochair")
)

var ActivitiesAdminAllowances = []access.Allowance{SeeAllActivities, CurateActivities, ConfigureActivities, ActAsCochair}

type volunteerBody struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Position string `json:"position"`
	Note     string `json:"note"`
	From     string `json:"from"`
}

type signUp struct {
	ops      []store.Op
	act      *Activity
	email    string
	position string
	note     string
	was      string
	action   string
	existed  bool
}

func (m *Activities) saveVolunteer(actor access.Actor, directory *Directory, body volunteerBody) (signUp, error) {
	act, err := m.find(body.ID)
	if err != nil {
		return signUp{}, err
	}
	editor := m.Edits(act, actor)
	var from *Activity
	if strings.TrimSpace(body.From) != "" {
		if from, err = m.find(body.From); err != nil {
			return signUp{}, err
		}
		if from == act {
			from = nil
		}
	}
	email := directory.Resolve(strings.ToLower(strings.TrimSpace(body.Email)))
	if email == "" {
		email = actor.Email
	}
	if !emailForm.MatchString(email) {
		return signUp{}, access.Invalid("that is not an email address")
	}
	offering := body.Position == PositionOpen && act.CoLeaderNeeded
	if !act.DirectSignUp && !editor && !offering {
		return signUp{}, access.Invalid("sign up for one of the things under it instead")
	}
	if !editor && act.Status != StatusOpen {
		return signUp{}, access.Invalid("this is not open for sign-ups")
	}
	if !slices.Contains(Positions, body.Position) {
		return signUp{}, access.Invalid("position must be one of %s", strings.Join(Positions, ", "))
	}
	if len(body.Note) > maxTextLength {
		return signUp{}, access.Invalid("the note is too long")
	}
	ops := []store.Op{}
	current := act.volunteer(email)
	was := ""
	if current != nil {
		was = current.Position
	}
	if from != nil {
		moving := from.volunteer(email)
		if moving == nil {
			return signUp{}, access.Invalid("%s is not signed up for %s", email, from.Title)
		}
		if !actor.Mine(email) && !(m.Edits(from, actor) && editor) {
			return signUp{}, access.Forbidden("only a co-chair or admin of both can move someone else's sign-up")
		}
		if was == "" {
			was = moving.Position
		}
		ops = append(ops, store.Delete(volunteersTab, store.Row{"Event ID": from.ID, "Email": email}))
	}
	existing := current != nil
	if !existing && directory.Person(email) == nil {
		return signUp{}, access.Invalid("that address is not in the directory")
	}
	if !editor {
		if (body.Position == PositionCoChair) != (was == PositionCoChair) {
			return signUp{}, access.Forbidden("only a co-chair or admin can make or unmake a co-chair")
		}
		if body.Position == PositionOpen && was != PositionOpen && !act.CoLeaderNeeded {
			return signUp{}, access.Invalid("this is not looking for a co-chair")
		}
	}
	if existing && !editor && !actor.Mine(email) {
		return signUp{}, access.Forbidden("only a co-chair or admin can change someone else's sign-up")
	}
	if !existing && !editor && act.full() {
		return signUp{}, access.Invalid("every spot is taken")
	}
	note := strings.TrimSpace(body.Note)
	cells := store.Row{"Position": body.Position, "Note": note}
	action := "edit"
	if !existing {
		action = "add"
		cells["Added By"] = actor.Email
		cells["Added"] = todayLocal()
	}
	if from != nil {
		action = "move"
	}
	ops = append(ops, store.Upsert(volunteersTab, store.Row{"Event ID": act.ID, "Email": email}, cells))
	return signUp{ops: ops, act: act, email: email, position: body.Position, note: note, was: was, action: action, existed: existing || from != nil}, nil
}

func (m *Activities) removeVolunteer(actor access.Actor, id, email string) (*Activity, string, []store.Op, error) {
	act, err := m.find(id)
	if err != nil {
		return nil, "", nil, err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !actor.Mine(email) && !m.Edits(act, actor) {
		return nil, "", nil, access.Forbidden("only a co-chair or admin can remove someone else")
	}
	return act, email, []store.Op{store.Delete(volunteersTab, store.Row{"Event ID": act.ID, "Email": email})}, nil
}

type activityPrettyConflict struct {
	Message string `json:"error"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Year    string `json:"year"`
	Prior   bool   `json:"prior"`
	Renamed string `json:"renamed,omitempty"`
}

func (c *activityPrettyConflict) refusal() error {
	return &access.Refusal{Status: http.StatusConflict, Message: c.Message, Body: c}
}

func renamedPretty(acts *Activities, pretty, year string) string {
	base := pretty
	if m := yearSpanForm.FindStringSubmatch(year); m != nil {
		base = pretty + "-" + m[1]
	}
	for i, candidate := 2, base; ; i++ {
		if acts.ByPretty(candidate) == nil && len(candidate) <= cells.MaxPrettyLength {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

func spotsCell(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func highlightCells(h *Highlight) store.Row {
	cells := store.Row{"Highlight Headline": "", "Highlight Body": "", "Highlight Icon": ""}
	if h != nil && (strings.TrimSpace(h.Headline) != "" || strings.TrimSpace(h.Body) != "") {
		cells["Highlight Headline"] = strings.TrimSpace(h.Headline)
		cells["Highlight Body"] = strings.TrimSpace(h.Body)
		cells["Highlight Icon"] = strings.TrimSpace(h.Icon)
	}
	return cells
}

type activitySave struct {
	ops    []store.Op
	id     string
	title  string
	year   string
	status string
	action string
	adding bool
}

var fieldColumns = map[string][]string{
	"year": {"Year"}, "title": {"Title"}, "parent": {"Parent", store.OrderColumn}, "category": {"Category"}, "status": {"Status"},
	"description": {"Description"}, "image": {"Image"}, "flyer": {"Flyer Image"}, "highlight": {"Highlight Headline", "Highlight Body", "Highlight Icon"},
	"timing": {"Timing"}, "start": {"Start"}, "end": {"End"}, "location": {"Location"}, "spots": {"Spots"},
	"coLeaderNeeded": {"Co-Leader Needed"}, "volunteersHidden": {"Volunteers Hidden"}, "volunteersComplete": {CompleteColumn, PriorityColumn},
	"directSignUp": {"Direct Sign-Up"}, "priority": {PriorityColumn}, "prettyId": {"Pretty ID"}, "allowAdding": {"Allow Adding"},
}

func bodyOf(a *Activity) activityBody {
	return activityBody{
		ID: a.ID, Year: a.Year, Title: a.Title, Parent: a.Parent, Category: a.Category, Status: a.Status,
		Description: a.Description, Image: a.Image, Flyer: a.Flyer, Highlight: a.Highlight, Timing: a.Timing, Start: a.Start, End: a.End,
		Location: a.Location, Spots: a.Spots, CoLeaderNeeded: a.CoLeaderNeeded, VolunteersHidden: a.VolunteersHiddenOwn, VolunteersComplete: a.VolunteersComplete,
		DirectSignUp: a.DirectSignUpOwn, Priority: a.Priority, PrettyID: a.PrettyID, AllowAdding: a.AllowAdding,
	}
}

func (m *Activities) saveActivity(actor access.Actor, patch activityPatch) (activitySave, error) {
	body := activityBody{}
	if err := patch.into(&body); err != nil {
		return activitySave{}, access.Invalid("bad request body")
	}
	adding := body.ID == ""
	var current *Activity
	approving := false
	if !adding {
		act, err := m.find(body.ID)
		if err != nil {
			return activitySave{}, err
		}
		current = act
		if !m.Edits(act, actor) {
			if !actor.May(CurateActivities) {
				return activitySave{}, access.Forbidden("only a co-chair or admin can edit this")
			}
			approving = true
		}
		body = bodyOf(current)
		if err := patch.into(&body); err != nil {
			return activitySave{}, access.Invalid("bad request body")
		}
	}
	title := strings.TrimSpace(body.Title)
	year := strings.TrimSpace(body.Year)
	status := body.Status
	switch {
	case adding && !actor.May(CurateActivities):
		status = StatusOpen
	case adding && status == "":
		status = StatusOpen
	case !adding && !actor.May(CurateActivities):
		approver := current.Parent != "" && m.Runs(m.Activity(current.Parent), actor.Email)
		switch {
		case status == current.Status:
		case current.Status == StatusPending && !approver:
			status = StatusPending
		case current.Status == StatusPending && status == StatusHidden:
			return activitySave{}, access.Invalid("only an admin can turn a suggestion down")
		case status != StatusOpen && status != StatusDone && status != StatusHidden:
			return activitySave{}, access.Invalid("a co-chair may only mark this open, done or hidden")
		}
		year = current.Year
	}
	parent := strings.TrimSpace(body.Parent)
	if parent != "" {
		p := m.Activity(parent)
		if p == nil {
			return activitySave{}, access.Invalid("no activity with id %q to sit under", parent)
		}
		parent = p.ID
		if p.Year != year {
			return activitySave{}, access.Invalid("a parent has to be in the same school year")
		}
		if current != nil {
			for node := p; node != nil; node = m.Activity(node.Parent) {
				if node.ID == current.ID {
					return activitySave{}, access.Invalid("that would put this inside itself")
				}
			}
		}
	}
	if current != nil && !actor.May(CurateActivities) && parent != current.Parent && (parent == "" || !m.Runs(m.Activity(parent), actor.Email)) {
		return activitySave{}, access.Forbidden("only an admin can move this there")
	}
	category := strings.TrimSpace(body.Category)
	if category == UncategorizedID {
		category = ""
	}
	editor := m.addsAsEditor(actor, parent)
	if category != "" {
		c := m.Category(category)
		if c == nil {
			return activitySave{}, access.Invalid("no category with id %q", category)
		}
		category = c.ID
		if parent == "" && c.EventID != "" {
			return activitySave{}, access.Invalid("%q belongs to one event, not the page", c.Title)
		}
		if parent != "" && c.EventID != m.Root(m.Activity(parent)).ID {
			return activitySave{}, access.Invalid("%q is not one of this event's categories", c.Title)
		}
	}
	if adding && !editor {
		suggested, err := m.addedStatus(actor, parent, category)
		if err != nil {
			return activitySave{}, err
		}
		status = suggested
	}
	mint := m.minter()
	var key string
	if adding {
		key = mint()
	} else {
		key = current.ID
	}
	joining := ""
	if adding {
		switch strings.TrimSpace(body.SignUp) {
		case PositionVolunteer, PositionOpen:
			joining = strings.TrimSpace(body.SignUp)
		case "", "none":
		default:
			return activitySave{}, access.Invalid("sign up as a volunteer, as one open to co-chairing, or not at all")
		}
	}
	pretty := cells.NormalizePretty(body.PrettyID)
	if err := cells.CheckPretty(pretty); err != nil {
		return activitySave{}, access.Invalid("%s", err.Error())
	}
	allowAdding, err := checkAdding(body.AllowAdding)
	if err != nil {
		return activitySave{}, access.Invalid("%s", err.Error())
	}
	direct, err := cells.YesNoBlank(body.DirectSignUp)
	if err != nil {
		return activitySave{}, access.Invalid("allow volunteers %s", err.Error())
	}
	volunteersHidden, err := cells.YesNoBlank(body.VolunteersHidden)
	if err != nil {
		return activitySave{}, access.Invalid("show volunteers %s", err.Error())
	}
	if !editor && (current == nil || !m.Edits(current, actor)) {
		allowAdding = ""
		if current != nil {
			allowAdding = current.AllowAdding
		}
	}
	var displaced *Activity
	renamed := ""
	if pretty != "" && parent != "" {
		for _, sibling := range m.Activity(parent).Children {
			if sibling.ID != key && sibling.PrettyID == pretty {
				conflict := &activityPrettyConflict{ID: sibling.ID, Title: sibling.Title, Year: sibling.Year,
					Message: fmt.Sprintf("%q is already the address of %q under the same parent", pretty, sibling.Title)}
				return activitySave{}, conflict.refusal()
			}
		}
	}
	if other := m.ByPretty(pretty); pretty != "" && parent == "" && other != nil && other.ID != key {
		conflict := &activityPrettyConflict{ID: other.ID, Title: other.Title, Year: other.Year, Prior: other.Year < year}
		if conflict.Prior {
			conflict.Renamed = renamedPretty(m, pretty, other.Year)
		}
		if !conflict.Prior || !body.TakeOver {
			if conflict.Prior {
				conflict.Message = fmt.Sprintf("%q is the address of %q from %s", pretty, other.Title, other.Year)
			} else {
				conflict.Message = fmt.Sprintf("%q is already the address of %q (%s)", pretty, other.Title, other.Year)
			}
			return activitySave{}, conflict.refusal()
		}
		displaced, renamed = other, conflict.Renamed
	}
	row := store.Row{
		"Event ID": key, "Year": year, "Title": title, "Parent": parent,
		"Category": category, "Status": status,
		"Description": strings.TrimSpace(body.Description), "Image": strings.TrimSpace(body.Image), "Flyer Image": strings.TrimSpace(body.Flyer),
		"Timing": strings.TrimSpace(body.Timing), "Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End),
		"Location": strings.TrimSpace(body.Location), "Spots": spotsCell(body.Spots),
		"Co-Leader Needed": cells.YesNoCell(body.CoLeaderNeeded), "Volunteers Hidden": volunteersHidden,
		CompleteColumn:   cells.YesNoCell(body.VolunteersComplete),
		"Direct Sign-Up": direct, "Pretty ID": pretty, "Allow Adding": allowAdding,
	}
	priority := body.Priority
	if !actor.May(CurateActivities) {
		priority = current != nil && current.Priority
	}
	if body.VolunteersComplete {
		priority = false
	}
	row[PriorityColumn] = cells.YesNoCell(priority)
	for k, v := range highlightCells(body.Highlight) {
		row[k] = v
	}
	ops := []store.Op{}
	action := "edit"
	if adding {
		action = "add"
		row["Added By"] = actor.Email
		row["Added"] = todayLocal()
		row[CalendarEventColumn] = mint()
		ops = append(ops, store.Insert(activitiesTab, row))
		if joining != "" {
			ops = append(ops, store.Insert(volunteersTab, store.Row{"Event ID": key, "Email": actor.Email, "Position": joining, "Added By": actor.Email, "Added": todayLocal()}))
		}
	} else {
		if current.Parent != parent {
			row[store.OrderColumn] = ""
		}
		cells := store.Row{}
		for _, field := range patch.fields() {
			for _, column := range fieldColumns[field] {
				if value, ok := row[column]; ok {
					cells[column] = value
				}
			}
		}
		ops = append(ops, store.Update(activitiesTab, store.Row{"Event ID": key}, cells))
	}
	if displaced != nil {
		ops = append(ops, store.Update(activitiesTab, store.Row{"Event ID": displaced.ID}, store.Row{"Pretty ID": renamed}))
	}
	if approving && (!slices.Equal(patch.fields(), []string{"status"}) || current.Status != StatusPending || (status != StatusOpen && status != StatusHidden)) {
		return activitySave{}, access.Forbidden("only a co-chair or admin can edit this")
	}
	return activitySave{ops: ops, id: key, title: title, year: year, status: status, action: action, adding: adding}, nil
}

func (m *Activities) addsAsEditor(actor access.Actor, parent string) bool {
	if parent == "" {
		return actor.May(CurateActivities)
	}
	return actor.May(ActAsCochair) || m.Runs(m.Activity(parent), actor.Email)
}

func (m *Activities) addedStatus(actor access.Actor, parent, category string) (string, error) {
	if m.addsAsEditor(actor, parent) {
		return "", nil
	}
	if category == "" && parent == "" {
		return "", access.Invalid("pick a category")
	}
	policy := AddingNo
	if parent != "" {
		policy = m.Activity(parent).Adding
	}
	if c := m.Category(category); c != nil {
		policy = c.Adding
	}
	switch policy {
	case AddingYes:
		return StatusOpen, nil
	case AddingApproval:
		return StatusPending, nil
	}
	return "", access.Invalid("new things cannot be added here")
}

func (m *Activities) deleteActivity(actor access.Actor, id string) (*Activity, []store.Op, error) {
	if err := require(actor, CurateActivities); err != nil {
		return nil, nil, err
	}
	act, err := m.find(id)
	if err != nil {
		return nil, nil, err
	}
	if len(act.Volunteers) > 0 {
		return nil, nil, access.Invalid("remove its volunteers first")
	}
	if len(act.Children) > 0 {
		return nil, nil, access.Invalid("delete the things under it first")
	}
	return act, []store.Op{store.Delete(activitiesTab, store.Row{"Event ID": act.ID})}, nil
}

type linkBody struct {
	ID          string `json:"id"`
	Link        string `json:"link"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Image       string `json:"image"`
}

func (m *Activities) findLink(key string) (*Activity, string, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	act := m.links[key]
	if act == nil {
		return nil, "", access.Missing("no link with id %q", key)
	}
	return act, key, nil
}

func (m *Activities) saveLink(actor access.Actor, body linkBody) (*Activity, string, []store.Op, error) {
	adding := strings.TrimSpace(body.Link) == ""
	var act *Activity
	var key string
	var err error
	if adding {
		act, err = m.find(body.ID)
		key = id.New(m.taken)
	} else {
		act, key, err = m.findLink(body.Link)
	}
	if err != nil {
		return nil, "", nil, err
	}
	if !m.Edits(act, actor) {
		return nil, "", nil, access.Forbidden("only a co-chair or admin can add links")
	}
	cells := store.Row{
		"Title": strings.TrimSpace(body.Title),
		"URL":   strings.TrimSpace(body.URL), "Image": strings.TrimSpace(body.Image),
		"Description": strings.TrimSpace(body.Description),
	}
	if adding {
		cells["Link ID"], cells["Event ID"] = key, act.ID
		return act, key, []store.Op{store.Insert(linksTab, cells)}, nil
	}
	return act, key, []store.Op{store.Update(linksTab, store.Row{"Link ID": key}, cells)}, nil
}

func (m *Activities) deleteLink(actor access.Actor, link string) (*Activity, []store.Op, error) {
	act, key, err := m.findLink(link)
	if err != nil {
		return nil, nil, err
	}
	if !m.Edits(act, actor) {
		return nil, nil, access.Forbidden("only a co-chair or admin can remove links")
	}
	return act, []store.Op{store.Delete(linksTab, store.Row{"Link ID": key})}, nil
}

func orderOps(tab, keyColumn string, ids, current []string) []store.Op {
	keys := store.Order(current)
	ops := []store.Op{}
	for i, id := range ids {
		if keys[i] != current[i] {
			ops = append(ops, store.Update(tab, store.Row{keyColumn: id}, store.Row{store.OrderColumn: keys[i]}))
		}
	}
	return ops
}

func (m *Activities) orderChildren(actor access.Actor, parentID string, order []string) (*Activity, []store.Op, error) {
	parent, err := m.find(parentID)
	if err != nil {
		return nil, nil, err
	}
	if !m.Edits(parent, actor) {
		return nil, nil, access.Forbidden("only a co-chair or admin can reorder these")
	}
	children := map[string]*Activity{}
	for _, c := range parent.Children {
		children[c.ID] = c
	}
	if len(order) != len(children) {
		return nil, nil, access.Invalid("the order must name every thing under %s exactly once", parent.Title)
	}
	ids, current := []string{}, []string{}
	for _, id := range order {
		c := children[m.aliases.Resolve(strings.TrimSpace(id))]
		if c == nil {
			return nil, nil, access.Invalid("%q is not one of the things under %s", id, parent.Title)
		}
		delete(children, c.ID)
		ids, current = append(ids, c.ID), append(current, c.Order)
	}
	return parent, orderOps(activitiesTab, "Event ID", ids, current), nil
}

func (m *Activities) editsCategories(actor access.Actor, eventID string) error {
	if eventID == "" {
		if actor.May(ConfigureActivities) {
			return nil
		}
		return access.Forbidden("only an admin can change the page's categories")
	}
	if actor.May(ActAsCochair) {
		return nil
	}
	event := m.Activity(eventID)
	if event == nil || !m.Runs(event, actor.Email) {
		return access.Forbidden("only a co-chair or admin can change this event's categories")
	}
	return nil
}

type categoryBody struct {
	ID          string `json:"id"`
	EventID     string `json:"eventId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
	AllowAdding string `json:"allowAdding"`
	ShowOnMain  *bool  `json:"showOnMain"`
}

type categorySave struct {
	op      store.Op
	id      string
	eventID string
	title   string
	action  string
}

func (m *Activities) saveCategory(actor access.Actor, body categoryBody) (categorySave, error) {
	title := strings.TrimSpace(body.Title)
	eventID := strings.TrimSpace(body.EventID)
	adding := body.ID == ""
	key := strings.TrimSpace(body.ID)
	if !adding {
		current := m.Category(key)
		if current == nil {
			return categorySave{}, access.Missing("no category with id %q", key)
		}
		if current.BuiltIn {
			return categorySave{}, access.Invalid("Uncategorized is built in and cannot be changed")
		}
		key, eventID = current.ID, current.EventID
	} else if eventID != "" {
		event := m.Activity(eventID)
		if event == nil || event.Parent != "" {
			return categorySave{}, access.Invalid("an event's category has to belong to a root event")
		}
		eventID = event.ID
	}
	if err := m.editsCategories(actor, eventID); err != nil {
		return categorySave{}, err
	}
	allowAdding, err := checkAdding(body.AllowAdding)
	if err != nil {
		return categorySave{}, access.Invalid("%s", err.Error())
	}
	if adding {
		key = id.New(m.taken)
	}
	row := store.Row{
		"Category ID":       key,
		"Event ID":          eventID,
		"Title":             title,
		"Description":       strings.TrimSpace(body.Description),
		"Image":             strings.TrimSpace(body.Image),
		"Allow Adding":      allowAdding,
		"Show On Main Page": "",
	}
	if eventID == "" {
		row["Show On Main Page"] = cells.YesNoCell(body.ShowOnMain == nil || *body.ShowOnMain)
	}
	save := categorySave{op: store.Insert(activityCategoriesTab, row), id: key, eventID: eventID, title: title, action: "add"}
	if !adding {
		save.op = store.Update(activityCategoriesTab, store.Row{"Category ID": key}, row)
		save.action = "edit"
	}
	return save, nil
}

type categoryFlagsBody struct {
	ID    string            `json:"id"`
	Flags map[string]string `json:"flags"`
}

var categoryFlagColumns = map[string]string{"directSignUp": "Direct Sign-Up", "volunteersHidden": "Volunteers Hidden", "hidden": "Hidden", "allowAdding": "Allow Adding"}

func (m *Activities) saveCategoryFlags(actor access.Actor, body categoryFlagsBody) (store.Op, *ActivityCategory, error) {
	c := m.Category(strings.TrimSpace(body.ID))
	if c == nil {
		return store.Op{}, nil, access.Missing("no category with id %q", body.ID)
	}
	if c.EventID == "" {
		return store.Op{}, nil, access.Invalid("only an event's own categories have volunteer settings")
	}
	if err := m.editsCategories(actor, c.EventID); err != nil {
		return store.Op{}, nil, err
	}
	if len(body.Flags) == 0 {
		return store.Op{}, nil, access.Invalid("nothing to change")
	}
	row := store.Row{}
	for flag, value := range body.Flags {
		column, ok := categoryFlagColumns[flag]
		if !ok {
			return store.Op{}, nil, access.Invalid("%q is not a category setting", flag)
		}
		check := cells.YesNoBlank
		if flag == "allowAdding" {
			check = checkAdding
		}
		cell, err := check(value)
		if err != nil {
			return store.Op{}, nil, access.Invalid("%s %s", flag, err.Error())
		}
		row[column] = cell
	}
	return store.Update(activityCategoriesTab, store.Row{"Category ID": c.ID}, row), c, nil
}

func (m *Activities) reorderCategories(actor access.Actor, eventID string, order []string) ([]store.Op, error) {
	if err := m.editsCategories(actor, eventID); err != nil {
		return nil, err
	}
	list := m.Categories
	if eventID != "" {
		event := m.Activity(eventID)
		if event == nil {
			return nil, access.Missing("no activity with id %q", eventID)
		}
		list = event.Categories
	}
	inScope := map[string]ActivityCategory{}
	for _, c := range list {
		if !c.BuiltIn {
			inScope[c.ID] = c
		}
	}
	if len(order) != len(inScope) {
		return nil, access.Invalid("the order must name every category exactly once")
	}
	ids, current := []string{}, []string{}
	for _, key := range order {
		c, ok := inScope[m.aliases.Resolve(key)]
		if !ok {
			return nil, access.Invalid("the order must name every category exactly once")
		}
		delete(inScope, c.ID)
		ids, current = append(ids, c.ID), append(current, c.Order)
	}
	return orderOps(activityCategoriesTab, "Category ID", ids, current), nil
}

func (s *Store) deleteActivityCategory(actor access.Actor, id string) (*ActivityCategory, []store.Op, error) {
	m := s.Model().Activities
	cat := m.Category(strings.TrimSpace(id))
	if cat == nil {
		return nil, nil, access.Missing("no such category")
	}
	if cat.BuiltIn {
		return nil, nil, access.Invalid("Uncategorized is built in and cannot be deleted")
	}
	if err := m.editsCategories(actor, cat.EventID); err != nil {
		return nil, nil, err
	}
	if s.Count(activitiesAppName, activitiesTab, store.Row{"Category": cat.ID}) > 0 {
		return nil, nil, access.Invalid("move or delete its activities first")
	}
	return cat, []store.Op{store.Delete(activityCategoriesTab, store.Row{"Category ID": cat.ID})}, nil
}

func (m *Activities) copyActivity(actor access.Actor, id string) (*Activity, string, string, []store.Op, error) {
	if err := require(actor, CurateActivities); err != nil {
		return nil, "", "", nil, err
	}
	act, err := m.find(id)
	if err != nil {
		return nil, "", "", nil, err
	}
	if act.Parent != "" {
		return nil, "", "", nil, access.Invalid("copy the whole activity it sits under instead")
	}
	year := ShiftYearSpan(act.Year, 1)
	for _, other := range m.Activities {
		if other.Year == year && other.Title == act.Title {
			return nil, "", "", nil, access.Invalid("%q already exists in %s", act.Title, year)
		}
	}
	mint := m.minter()
	fresh := map[string]string{act.ID: mint()}
	ops := []store.Op{}
	for _, c := range act.Categories {
		fresh[c.ID] = mint()
		ops = append(ops, store.Insert(activityCategoriesTab, store.Row{
			"Category ID": fresh[c.ID], "Event ID": fresh[act.ID], "Title": c.Title, "Description": c.Description,
			"Image": c.Image, "Allow Adding": c.AllowAdding, store.OrderColumn: c.Order,
			"Direct Sign-Up": c.DirectSignUpOwn, "Volunteers Hidden": c.VolunteersHiddenOwn, "Hidden": c.HiddenOwn,
		}))
	}
	remap := func(id string) string {
		if to, ok := fresh[id]; ok {
			return to
		}
		if id == UncategorizedID {
			return ""
		}
		return id
	}
	rowFor := func(c *Activity, parent string) store.Row {
		row := store.Row{
			"Event ID": fresh[c.ID], CalendarEventColumn: mint(), "Year": year, "Title": c.Title, "Parent": parent, "Category": remap(c.Category),
			"Status": c.Status, "Description": c.Description, "Image": c.Image, "Flyer Image": c.Flyer, "Timing": c.Timing,
			"Location": c.Location, "Spots": spotsCell(c.Spots),
			"Co-Leader Needed": cells.YesNoCell(c.CoLeaderNeeded), "Volunteers Hidden": c.VolunteersHiddenOwn,
			"Direct Sign-Up": c.DirectSignUpOwn, "Allow Adding": c.AllowAdding, "Added By": actor.Email, "Added": todayLocal(),
			store.OrderColumn: c.Order, CompleteColumn: cells.YesNoCell(false),
		}
		for k, v := range highlightCells(c.Highlight) {
			row[k] = v
		}
		return row
	}
	root := rowFor(act, "")
	root["Status"] = StatusOpen
	ops = append(ops, store.Insert(activitiesTab, root))
	copied := []*Activity{act}
	for _, c := range act.Descendants() {
		if c.Status == StatusPending {
			continue
		}
		if _, ok := fresh[c.Parent]; !ok {
			continue
		}
		fresh[c.ID] = mint()
		ops = append(ops, store.Insert(activitiesTab, rowFor(c, fresh[c.Parent])))
		copied = append(copied, c)
	}
	for _, node := range copied {
		for _, l := range node.Links {
			ops = append(ops, store.Insert(linksTab, store.Row{"Link ID": mint(), "Event ID": fresh[node.ID], "Title": l.Title, "URL": l.URL, "Image": l.Image, "Description": l.Description}))
		}
	}
	return act, fresh[act.ID], year, ops, nil
}

func activitySettingsOps(actor access.Actor, expenseFormURL, intro string) ([]store.Op, error) {
	if err := require(actor, ConfigureActivities); err != nil {
		return nil, err
	}
	values := map[string]string{ExpenseFormKey: strings.TrimSpace(expenseFormURL), IntroKey: strings.TrimSpace(intro)}
	ops := []store.Op{}
	for _, key := range activitySettingKeys {
		ops = append(ops, store.Upsert(activitySettingsTab, store.Row{"Key": key}, store.Row{"Value": values[key]}))
	}
	return ops, nil
}

func activityNotifyOps(actor access.Actor, wanted []string) (string, []store.Op, error) {
	if err := require(actor, ConfigureActivities); err != nil {
		return "", nil, err
	}
	kinds := []string{}
	for _, k := range NotifyKinds {
		if slices.Contains(wanted, k) {
			kinds = append(kinds, k)
		}
	}
	value := strings.Join(kinds, ",")
	return value, []store.Op{store.Upsert(notificationsTab, store.Row{"Email": actor.Email}, store.Row{"Kinds": value})}, nil
}

var reservedPaths = map[string]bool{"": true, "my": true, "calendar": true, "approvals": true, "admin": true, "years": true, "api": true, "auth": true, "hooks": true, "open": true, "blob": true}

type redirectSave struct {
	op     store.Op
	old    string
	to     string
	action string
}

func (m *Activities) saveRedirect(actor access.Actor, original, oldCell, newCell string) (redirectSave, error) {
	if err := require(actor, ConfigureActivities); err != nil {
		return redirectSave{}, err
	}
	old, to := activityRedirectPath(oldCell), redirectTo(newCell)
	if old == "" {
		return redirectSave{}, access.Invalid("say which address to redirect")
	}
	if to == "" {
		return redirectSave{}, access.Invalid("say where the address should go")
	}
	first, _, _ := strings.Cut(strings.TrimPrefix(old, "/"), "/")
	if reservedPaths[strings.ToLower(first)] {
		return redirectSave{}, access.Invalid("%s is one of the portal's own addresses and cannot be redirected", old)
	}
	if act := m.walk(old); act != nil {
		return redirectSave{}, access.Refuse(http.StatusConflict, "%s is the address of “%s” (%s); rename it from its page instead", old, act.Title, act.Year)
	}
	if strings.EqualFold(old, to) {
		return redirectSave{}, access.Invalid("an address cannot redirect to itself")
	}
	var replacing *ActivityRedirect
	kind := RedirectAdmin
	if from := activityRedirectPath(original); from != "" {
		replacing = m.redirect(from)
		if replacing == nil {
			return redirectSave{}, access.Missing("no redirect from %s", from)
		}
		if replacing.Type != "" {
			kind = replacing.Type
		}
	} else if m.redirect(old) != nil {
		return redirectSave{}, access.Refuse(http.StatusConflict, "%s is already redirected; edit that one", old)
	}
	if !isURL(to) && m.withRedirect(ActivityRedirect{Type: kind, Old: old, New: to}, replacing).Destination(old) == "" {
		return redirectSave{}, access.Invalid("%s leads back to %s", to, old)
	}
	cells := store.Row{"Type": kind, "Old": old, "New": to, "Date": todayLocal()}
	if replacing != nil {
		return redirectSave{op: store.Update(activityRedirectsTab, store.Row{"Old": replacing.cell}, cells), old: old, to: to, action: "edit"}, nil
	}
	return redirectSave{op: store.Insert(activityRedirectsTab, cells), old: old, to: to, action: "add"}, nil
}

func (m *Activities) deleteRedirect(actor access.Actor, old string) (*ActivityRedirect, []store.Op, error) {
	if err := require(actor, ConfigureActivities); err != nil {
		return nil, nil, err
	}
	redirect := m.redirect(activityRedirectPath(old))
	if redirect == nil {
		return nil, nil, access.Missing("no redirect from %s", old)
	}
	return redirect, []store.Op{store.Delete(activityRedirectsTab, store.Row{"Old": redirect.cell})}, nil
}
