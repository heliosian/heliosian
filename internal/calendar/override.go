package calendar

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
)

// overrideColumns are the Overrides cells the Edit form on an imported
// event writes; Day Type and Hidden it leaves as the row has them.
var overrideColumns = []string{"Title", "Start", "End", "Location", "Description", "Tags", "Keywords", "Note", "Address"}

// setOverride is PUT /api/calendar/overrides: an admin correcting an event
// the school's calendars bring, from its page's Edit. The form sends the
// event as the admin wants it; each field is set against the school's own
// version - the event as the load makes it with no Overrides row - and only
// what differs is written, a field put back to the school's value going
// blank so it follows the school again, and one emptied the clearing mark.
// The Note is the admin's reason, shown to the admins on the page. A row
// left with nothing in it is removed.
func (a app) setOverride(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Start       string   `json:"start"`
		End         string   `json:"end"`
		Location    string   `json:"location"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		Keywords    []string `json:"keywords"`
		Note        string   `json:"note"`
		// Address is a friendly web address, /e/{address}, blank for none.
		Address string `json:"address"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := strings.TrimSpace(body.ID)
	e := a.cache.Model().Event(id)
	if e == nil || !e.imported() {
		http.Error(w, "only an event the school's calendars bring is corrected here", http.StatusNotFound)
		return
	}
	tables := a.cache.Tables()
	bare, err := BuildModel(tables.WithoutOverride(id), a.cache.roster())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	school := bare.Event(id)
	if school == nil {
		http.Error(w, "the school's version of the event is not there", http.StatusInternalServerError)
		return
	}
	cells := map[string]string{}
	// A browser's text box hands back its lines broken by \n alone, where
	// the school's calendar may have \r\n; that is no change of anyone's.
	plain := func(s string) string {
		return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"))
	}
	set := func(column, want, was string) {
		switch want = strings.TrimSpace(want); {
		case plain(want) == plain(was):
			cells[column] = ""
		case want == "":
			cells[column] = Clear
		default:
			cells[column] = want
		}
	}
	set("Title", body.Title, school.Title)
	start, end, allDay, err := parseWhen(strings.TrimSpace(body.Start), strings.TrimSpace(body.End))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if start.Equal(school.start) && end.Equal(school.end) && allDay == school.AllDay {
		cells["Start"], cells["End"] = "", ""
	} else {
		cells["Start"], cells["End"] = strings.TrimSpace(body.Start), strings.TrimSpace(body.End)
	}
	set("Location", body.Location, school.Location)
	set("Description", body.Description, school.Description)
	// Lists compare as sets; the tags the app supplies are never the
	// sheet's to name.
	builtIn := map[string]bool{}
	for _, t := range bare.Tags {
		builtIn[t.Name] = t.BuiltIn
	}
	list := func(column string, want, was []string) {
		want = SplitList(JoinList(want))
		was = slices.DeleteFunc(slices.Clone(was), func(t string) bool { return builtIn[t] })
		x, y := slices.Sorted(slices.Values(want)), slices.Sorted(slices.Values(was))
		switch {
		case slices.Equal(x, y):
			cells[column] = ""
		case len(want) == 0:
			cells[column] = Clear
		default:
			cells[column] = JoinList(want)
		}
	}
	list("Tags", body.Tags, school.Tags)
	list("Keywords", body.Keywords, school.Keywords)
	cells["Note"] = strings.TrimSpace(body.Note)
	// The address is the admin's alone - the school's calendar has none - so
	// it is written as given; one taken already refuses the load, below.
	cells["Address"] = strings.ToLower(strings.TrimSpace(body.Address))
	if cells["Address"] != "" && !eventIDForm.MatchString(cells["Address"]) {
		http.Error(w, "an address is letters, digits and dashes, 3 to 40 of them", http.StatusBadRequest)
		return
	}

	var before map[string]string
	for _, row := range tables.Overrides {
		if row["Event ID"] == id {
			before = row
		}
	}
	after := map[string]string{}
	for _, c := range OverrideColumns[1:] {
		after[c] = before[c]
	}
	for c, v := range cells {
		after[c] = v
	}
	empty := !slices.ContainsFunc(OverrideColumns[1:], func(c string) bool { return strings.TrimSpace(after[c]) != "" })
	next := tables.WithOverride(id, cells)
	if empty {
		next = tables.WithoutOverride(id)
	}
	if !a.commit(r.Context(), w, next, func() error {
		switch {
		case empty && before != nil:
			if err := a.writer.Delete(appName, OverridesTab, map[string]string{"Event ID": id}); err != nil {
				return err
			}
		case !empty:
			if err := a.writer.Set(appName, OverridesTab, map[string]string{"Event ID": id}, cells); err != nil {
				return err
			}
		}
		for _, c := range overrideColumns {
			if before[c] != after[c] {
				if err := a.logChange(r, actor, "changed", OverridesTab, id, c, before[c], after[c]); err != nil {
					return err
				}
			}
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: override set", "actor", actor, "event", id, "cleared", empty)
	w.WriteHeader(http.StatusNoContent)
}

// setOverrideImage is PUT /api/calendar/overrides/image: an admin giving an
// event the school's calendars bring a picture, from the page's Add an
// image - the Image cell of its Overrides row, blank to take it away.
func (a app) setOverrideImage(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		ID    string `json:"id"`
		Image string `json:"image"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := strings.TrimSpace(body.ID)
	e := a.cache.Model().Event(id)
	if e == nil || !e.imported() {
		http.Error(w, "only an event the school's calendars bring takes its picture here", http.StatusNotFound)
		return
	}
	id = e.ID
	image := strings.Trim(strings.TrimSpace(body.Image), "/")
	cells := map[string]string{"Image": image}
	was := strings.TrimPrefix(e.Image, "/")
	if !a.commit(r.Context(), w, a.cache.Tables().WithOverride(id, cells), func() error {
		if err := a.writer.Set(appName, OverridesTab, map[string]string{"Event ID": id}, cells); err != nil {
			return err
		}
		return a.logChange(r, actor, "changed", OverridesTab, id, "Image", was, image)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: override image", "actor", actor, "event", id, "image", image)
	w.WriteHeader(http.StatusNoContent)
}
