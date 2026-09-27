package team

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/serve"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

func (m *Model) find(id string) (*Activity, error) {
	act := m.Activity(strings.TrimSpace(id))
	if act == nil {
		return nil, access.Missing("no activity with id %q", id)
	}
	return act, nil
}

func requireAdmin(actor access.Actor) error {
	if !actor.Admin {
		return access.Forbidden("admin access required")
	}
	return nil
}

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

func (m *Model) saveVolunteer(actor access.Actor, directory *who.Model, body volunteerBody) (signUp, error) {
	act, err := m.find(body.ID)
	if err != nil {
		return signUp{}, err
	}
	editor := m.Edits(act, actor)
	var from *Activity
	if strings.TrimSpace(body.From) != "" && strings.TrimSpace(body.From) != act.ID {
		if from, err = m.find(body.From); err != nil {
			return signUp{}, err
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
	if !existing && !editor && act.Spots > 0 && len(act.Volunteers) >= act.Spots {
		return signUp{}, access.Invalid("every spot is taken")
	}
	note := strings.TrimSpace(body.Note)
	cells := store.Row{"Position": body.Position, "Note": note}
	action := "edit"
	if !existing {
		action = "add"
		cells["Added By"] = actor.Email
		cells["Added"] = today()
	}
	if from != nil {
		action = "move"
	}
	ops = append(ops, store.Upsert(volunteersTab, store.Row{"Event ID": act.ID, "Email": email}, cells))
	return signUp{ops: ops, act: act, email: email, position: body.Position, note: note, was: was, action: action, existed: existing || from != nil}, nil
}

func (m *Model) removeVolunteer(actor access.Actor, id, email string) (*Activity, string, []store.Op, error) {
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

type prettyConflict struct {
	Message string `json:"error"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Year    string `json:"year"`
	Prior   bool   `json:"prior"`
	Renamed string `json:"renamed,omitempty"`
}

func (c *prettyConflict) refusal() error {
	return &access.Refusal{Status: http.StatusConflict, Message: c.Message, Body: c}
}

func renamedPretty(model *Model, pretty, year string) string {
	base := pretty
	if m := yearForm.FindStringSubmatch(year); m != nil {
		base = pretty + "-" + m[1]
	}
	for i, candidate := 2, base; ; i++ {
		if model.ByPretty(candidate) == nil && len(candidate) <= cells.MaxPrettyLength {
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

func (m *Model) saveActivity(actor access.Actor, body activityBody) (activitySave, error) {
	title := strings.TrimSpace(body.Title)
	year := strings.TrimSpace(body.Year)
	adding := body.ID == ""
	var current *Activity
	if !adding {
		act, err := m.find(body.ID)
		if err != nil {
			return activitySave{}, err
		}
		current = act
		if !m.Edits(act, actor) {
			return activitySave{}, access.Forbidden("only a co-chair or admin can edit this")
		}
	}
	status := body.Status
	switch {
	case adding && !actor.Admin:
		status = StatusOpen
	case adding && status == "":
		status = StatusOpen
	case !adding && !actor.Admin:
		approver := current.Parent != "" && m.Runs(m.Activity(current.Parent), actor.Email)
		switch {
		case status == current.Status:
		case current.Status == StatusPending && !approver:
			status = StatusPending
		case status != StatusOpen && status != StatusDone:
			return activitySave{}, access.Invalid("a co-chair may only mark this open or done")
		}
		year = current.Year
	}
	parent := strings.TrimSpace(body.Parent)
	if parent != "" {
		p := m.Activity(parent)
		if p == nil {
			return activitySave{}, access.Invalid("no activity with id %q to sit under", parent)
		}
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
	if current != nil && !actor.Admin && parent != current.Parent && (parent == "" || !m.Runs(m.Activity(parent), actor.Email)) {
		return activitySave{}, access.Forbidden("only an admin can move this there")
	}
	category := strings.TrimSpace(body.Category)
	if category == UncategorizedID {
		category = ""
	}
	editor := actor.Admin
	if parent != "" {
		editor = editor || m.Runs(m.Activity(parent), actor.Email)
	}
	policy := AddingNo
	if parent != "" {
		policy = m.Activity(parent).Adding
	}
	if category == "" && parent == "" && adding && !editor {
		return activitySave{}, access.Invalid("pick a category")
	}
	if category != "" {
		c := m.Category(category)
		if c == nil {
			return activitySave{}, access.Invalid("no category with id %q", category)
		}
		if parent == "" && c.EventID != "" {
			return activitySave{}, access.Invalid("%q belongs to one event, not the page", c.Title)
		}
		if parent != "" && c.EventID != m.Root(m.Activity(parent)).ID {
			return activitySave{}, access.Invalid("%q is not one of this event's categories", c.Title)
		}
		policy = c.Adding
	}
	if adding && !editor {
		switch policy {
		case AddingYes:
			status = StatusOpen
		case AddingApproval:
			status = StatusPending
		default:
			return activitySave{}, access.Invalid("new things cannot be added here")
		}
	}
	id := body.ID
	if adding {
		id = serve.ID(8)
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
	if !editor {
		allowAdding = ""
		if current != nil {
			allowAdding = current.AllowAdding
		}
	}
	var displaced *Activity
	renamed := ""
	if pretty != "" && parent != "" {
		for _, sibling := range m.Activity(parent).Children {
			if sibling.ID != id && sibling.PrettyID == pretty {
				conflict := &prettyConflict{ID: sibling.ID, Title: sibling.Title, Year: sibling.Year,
					Message: fmt.Sprintf("%q is already the address of %q under the same parent", pretty, sibling.Title)}
				return activitySave{}, conflict.refusal()
			}
		}
	}
	if other := m.ByPretty(pretty); pretty != "" && parent == "" && other != nil && other.ID != id {
		conflict := &prettyConflict{ID: other.ID, Title: other.Title, Year: other.Year, Prior: other.Year < year}
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
		"Event ID": id, "Year": year, "Title": title, "Parent": parent,
		"Category": category, "Status": status,
		"Description": strings.TrimSpace(body.Description), "Image": strings.TrimSpace(body.Image), "Flyer Image": strings.TrimSpace(body.Flyer),
		"Timing": strings.TrimSpace(body.Timing), "Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End),
		"Location": strings.TrimSpace(body.Location), "Spots": spotsCell(body.Spots),
		"Co-Leader Needed": cells.YesNoCell(body.CoLeaderNeeded), "Volunteers Hidden": cells.YesNoCell(body.VolunteersHidden),
		CompleteColumn:   cells.YesNoCell(body.VolunteersComplete),
		"Direct Sign-Up": cells.YesNoCell(body.DirectSignUp), "Pretty ID": pretty, "Allow Adding": allowAdding,
	}
	priority := body.Priority
	if !actor.Admin {
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
		row["Added"] = today()
		ops = append(ops, store.Insert(activitiesTab, row))
		if joining != "" {
			ops = append(ops, store.Insert(volunteersTab, store.Row{"Event ID": id, "Email": actor.Email, "Position": joining, "Added By": actor.Email, "Added": today()}))
		}
	} else {
		if current.Parent != parent {
			row[store.OrderColumn] = ""
		}
		ops = append(ops, store.Update(activitiesTab, store.Row{"Event ID": id}, row))
	}
	if displaced != nil {
		ops = append(ops, store.Update(activitiesTab, store.Row{"Event ID": displaced.ID}, store.Row{"Pretty ID": renamed}))
	}
	return activitySave{ops: ops, id: id, title: title, year: year, status: status, action: action, adding: adding}, nil
}

func (m *Model) deleteActivity(actor access.Actor, id string) (*Activity, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
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
	Original    string `json:"original"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Image       string `json:"image"`
}

func (m *Model) saveLink(actor access.Actor, body linkBody) (*Activity, string, []store.Op, error) {
	act, err := m.find(body.ID)
	if err != nil {
		return nil, "", nil, err
	}
	if !m.Edits(act, actor) {
		return nil, "", nil, access.Forbidden("only a co-chair or admin can add links")
	}
	cells := store.Row{
		"Event ID": act.ID, "Title": strings.TrimSpace(body.Title),
		"URL": strings.TrimSpace(body.URL), "Image": strings.TrimSpace(body.Image),
		"Description": strings.TrimSpace(body.Description),
	}
	if body.Original == "" {
		return act, "add", []store.Op{store.Insert(linksTab, cells)}, nil
	}
	if !slices.ContainsFunc(act.Links, func(l Link) bool { return l.Title == body.Original }) {
		return nil, "", nil, access.Missing("%s has no link %q", act.Title, body.Original)
	}
	return act, "edit", []store.Op{store.Update(linksTab, store.Row{"Event ID": act.ID, "Title": body.Original}, cells)}, nil
}

func (m *Model) deleteLink(actor access.Actor, id, title string) (*Activity, []store.Op, error) {
	act, err := m.find(id)
	if err != nil {
		return nil, nil, err
	}
	if !m.Edits(act, actor) {
		return nil, nil, access.Forbidden("only a co-chair or admin can remove links")
	}
	return act, []store.Op{store.Delete(linksTab, store.Row{"Event ID": act.ID, "Title": strings.TrimSpace(title)})}, nil
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

func (m *Model) orderChildren(actor access.Actor, parentID string, order []string) (*Activity, []store.Op, error) {
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
		c := children[strings.TrimSpace(id)]
		if c == nil {
			return nil, nil, access.Invalid("%q is not one of the things under %s", id, parent.Title)
		}
		delete(children, c.ID)
		ids, current = append(ids, c.ID), append(current, c.Order)
	}
	return parent, orderOps(activitiesTab, "Event ID", ids, current), nil
}

func (m *Model) editsCategories(actor access.Actor, eventID string) error {
	if actor.Admin {
		return nil
	}
	if eventID == "" {
		return access.Forbidden("only an admin can change the page's categories")
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

func (m *Model) saveCategory(actor access.Actor, body categoryBody) (categorySave, error) {
	title := strings.TrimSpace(body.Title)
	eventID := strings.TrimSpace(body.EventID)
	adding := body.ID == ""
	id := strings.TrimSpace(body.ID)
	if !adding {
		current := m.Category(id)
		if current == nil {
			return categorySave{}, access.Missing("no category with id %q", id)
		}
		if current.BuiltIn {
			return categorySave{}, access.Invalid("Uncategorized is built in and cannot be changed")
		}
		eventID = current.EventID
	} else if eventID != "" {
		event := m.Activity(eventID)
		if event == nil || event.Parent != "" {
			return categorySave{}, access.Invalid("an event's category has to belong to a root event")
		}
	}
	if err := m.editsCategories(actor, eventID); err != nil {
		return categorySave{}, err
	}
	allowAdding, err := checkAdding(body.AllowAdding)
	if err != nil {
		return categorySave{}, access.Invalid("%s", err.Error())
	}
	if adding {
		id = serve.ID(8)
	}
	row := store.Row{
		"Category ID":       id,
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
	save := categorySave{op: store.Insert(categoriesTab, row), id: id, eventID: eventID, title: title, action: "add"}
	if !adding {
		save.op = store.Update(categoriesTab, store.Row{"Category ID": id}, row)
		save.action = "edit"
	}
	return save, nil
}

func (m *Model) reorderCategories(actor access.Actor, eventID string, order []string) ([]store.Op, error) {
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
	inScope := map[string]Category{}
	for _, c := range list {
		if !c.BuiltIn {
			inScope[c.ID] = c
		}
	}
	if len(order) != len(inScope) {
		return nil, access.Invalid("the order must name every category exactly once")
	}
	ids, current := []string{}, []string{}
	for _, id := range order {
		c, ok := inScope[id]
		if !ok {
			return nil, access.Invalid("the order must name every category exactly once")
		}
		delete(inScope, id)
		ids, current = append(ids, c.ID), append(current, c.Order)
	}
	return orderOps(categoriesTab, "Category ID", ids, current), nil
}

func (c *Cache) deleteCategory(actor access.Actor, id string) (*Category, []store.Op, error) {
	m := c.Model()
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
	if c.Count(activitiesTab, store.Row{"Category": cat.ID}) > 0 {
		return nil, nil, access.Invalid("move or delete its activities first")
	}
	return cat, []store.Op{store.Delete(categoriesTab, store.Row{"Category ID": cat.ID})}, nil
}

func (m *Model) copyActivity(actor access.Actor, id string) (*Activity, string, string, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, "", "", nil, err
	}
	act, err := m.find(id)
	if err != nil {
		return nil, "", "", nil, err
	}
	if act.Parent != "" {
		return nil, "", "", nil, access.Invalid("copy the whole activity it sits under instead")
	}
	year := ShiftYear(act.Year, 1)
	for _, other := range m.Activities {
		if other.Year == year && other.Title == act.Title {
			return nil, "", "", nil, access.Invalid("%q already exists in %s", act.Title, year)
		}
	}
	fresh := map[string]string{act.ID: serve.ID(8)}
	ops := []store.Op{}
	for _, c := range act.Categories {
		fresh[c.ID] = serve.ID(8)
		ops = append(ops, store.Insert(categoriesTab, store.Row{
			"Category ID": fresh[c.ID], "Event ID": fresh[act.ID], "Title": c.Title, "Description": c.Description,
			"Image": c.Image, "Allow Adding": c.AllowAdding, store.OrderColumn: c.Order,
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
			"Event ID": fresh[c.ID], "Year": year, "Title": c.Title, "Parent": parent, "Category": remap(c.Category),
			"Status": c.Status, "Description": c.Description, "Image": c.Image, "Flyer Image": c.Flyer, "Timing": c.Timing,
			"Location": c.Location, "Spots": spotsCell(c.Spots),
			"Co-Leader Needed": cells.YesNoCell(c.CoLeaderNeeded), "Volunteers Hidden": cells.YesNoCell(c.VolunteersHidden),
			"Direct Sign-Up": cells.YesNoCell(c.DirectSignUp), "Allow Adding": c.AllowAdding, "Added By": actor.Email, "Added": today(),
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
		fresh[c.ID] = serve.ID(8)
		ops = append(ops, store.Insert(activitiesTab, rowFor(c, fresh[c.Parent])))
		copied = append(copied, c)
	}
	for _, node := range copied {
		for _, l := range node.Links {
			ops = append(ops, store.Insert(linksTab, store.Row{"Event ID": fresh[node.ID], "Title": l.Title, "URL": l.URL, "Image": l.Image, "Description": l.Description}))
		}
	}
	return act, fresh[act.ID], year, ops, nil
}

func saveSettings(actor access.Actor, expenseFormURL, intro string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	values := map[string]string{ExpenseFormKey: strings.TrimSpace(expenseFormURL), IntroKey: strings.TrimSpace(intro)}
	ops := []store.Op{}
	for _, key := range settingKeys {
		ops = append(ops, store.Upsert(settingsTab, store.Row{"Key": key}, store.Row{"Value": values[key]}))
	}
	return ops, nil
}

func saveNotify(actor access.Actor, wanted []string) (string, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
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

func (m *Model) setAdmins(actor access.Actor, superAdmins, wanted []string) ([]string, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	super := map[string]bool{}
	for _, e := range superAdmins {
		super[e] = true
	}
	admins := []string{}
	for _, e := range config.NormalizeEmails(wanted) {
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
	return admins, ops, nil
}

var reservedPaths = map[string]bool{"": true, "my": true, "calendar": true, "approvals": true, "admin": true, "years": true, "api": true, "auth": true, "hooks": true, "open": true, "blob": true}

type redirectSave struct {
	op     store.Op
	old    string
	to     string
	action string
}

func (m *Model) saveRedirect(actor access.Actor, original, oldCell, newCell string) (redirectSave, error) {
	if err := requireAdmin(actor); err != nil {
		return redirectSave{}, err
	}
	old, to := redirectPath(oldCell), redirectTo(newCell)
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
	var replacing *Redirect
	kind := RedirectAdmin
	if from := redirectPath(original); from != "" {
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
	if !isURL(to) && m.withRedirect(Redirect{Type: kind, Old: old, New: to}, replacing).Destination(old) == "" {
		return redirectSave{}, access.Invalid("%s leads back to %s", to, old)
	}
	cells := store.Row{"Type": kind, "Old": old, "New": to, "Date": today()}
	if replacing != nil {
		return redirectSave{op: store.Update(redirectsTab, store.Row{"Old": replacing.cell}, cells), old: old, to: to, action: "edit"}, nil
	}
	return redirectSave{op: store.Insert(redirectsTab, cells), old: old, to: to, action: "add"}, nil
}

func (m *Model) deleteRedirect(actor access.Actor, old string) (*Redirect, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	redirect := m.redirect(redirectPath(old))
	if redirect == nil {
		return nil, nil, access.Missing("no redirect from %s", old)
	}
	return redirect, []store.Op{store.Delete(redirectsTab, store.Row{"Old": redirect.cell})}, nil
}
