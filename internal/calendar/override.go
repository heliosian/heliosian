package calendar

import (
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/store"
)

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
		Address     string   `json:"address"`
	}
	if !decode(w, r, &body) {
		return
	}
	model := a.cache.Model()
	e := model.Event(strings.TrimSpace(body.ID))
	if e == nil || !e.imported() {
		http.Error(w, "only an event the school's calendars bring is corrected here", http.StatusNotFound)
		return
	}
	id := e.ID
	school := model.imports[id]
	cells := store.Row{}
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
	builtIn := map[string]bool{}
	for _, t := range model.Tags {
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
	cells["Address"] = strings.ToLower(strings.TrimSpace(body.Address))
	if cells["Address"] != "" && !eventIDForm.MatchString(cells["Address"]) {
		http.Error(w, "an address is letters, digits and dashes, 3 to 40 of them", http.StatusBadRequest)
		return
	}
	empty := !slices.ContainsFunc(slices.Collect(maps.Values(cells)), func(v string) bool { return v != "" })
	if p := model.Provenance[id]; p != nil && slices.ContainsFunc(p.Corrected, func(c string) bool { return c == "Day Type" || c == "Hidden" || c == "Image" }) {
		empty = false
	}
	op := store.Set(OverridesTab, store.Row{"Event ID": id}, cells)
	if empty {
		op = store.Delete(OverridesTab, store.Row{"Event ID": id})
	}
	if !a.commit(w, r, actor, op) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: override set", "actor", actor, "event", id, "cleared", empty)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) setOverrideImage(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		ID    string `json:"id"`
		Image string `json:"image"`
	}
	if !decode(w, r, &body) {
		return
	}
	e := a.cache.Model().Event(strings.TrimSpace(body.ID))
	if e == nil || !e.imported() {
		http.Error(w, "only an event the school's calendars bring takes its picture here", http.StatusNotFound)
		return
	}
	image := strings.Trim(strings.TrimSpace(body.Image), "/")
	if !a.commit(w, r, actor, store.Set(OverridesTab, store.Row{"Event ID": e.ID}, store.Row{"Image": image})) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: override image", "actor", actor, "event", e.ID, "image", image)
	w.WriteHeader(http.StatusNoContent)
}
