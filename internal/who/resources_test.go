package who

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
)

type resourceReply struct {
	Data     json.RawMessage                      `json:"data"`
	Included map[string]map[string]map[string]any `json:"included"`
}

func resourceGet(t *testing.T, m *Model, path string) (int, resourceReply) {
	t.Helper()
	reg := api.New(api.Config[*Model]{
		Actor: func(*http.Request, *Model) access.Actor { return access.Actor{Email: "ruth.amari@heliosschool.org"} },
		Held:  func(string) []access.Allowance { return nil },
		Now:   time.Now,
	})
	for _, rt := range Resources() {
		reg.Add(rt)
	}
	reg.Publish(m)
	mux := http.NewServeMux()
	reg.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	var out resourceReply
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	return rec.Code, out
}

func resourceID(t *testing.T, m *Model, path string) string {
	t.Helper()
	code, out := resourceGet(t, m, path)
	if code != http.StatusOK {
		t.Fatalf("%s: status %d", path, code)
	}
	var got string
	if err := json.Unmarshal(out.Data, &got); err != nil {
		t.Fatalf("%s: data %s", path, out.Data)
	}
	return got
}

func TestPeopleResolveByAddressAliasAndLocalPart(t *testing.T) {
	m := sampleModel(t)
	hank := m.Person("hank.morrow@heliosschool.org").ID
	for _, path := range []string{"/api/people/" + hank, "/api/people/hank.morrow@heliosschool.org", "/api/people/facilities@heliosschool.org", "/api/people/hank.morrow", "/api/people/Hank.Morrow"} {
		if got := resourceID(t, m, path); got != hank {
			t.Errorf("%s resolved to %s, want %s", path, got, hank)
		}
	}
	_, out := resourceGet(t, m, "/api/people/"+hank)
	person := out.Included["people"][hank]
	if person["email"] != "hank.morrow@heliosschool.org" || person["slug"] != "hank.morrow" || person["path"] != "/people/hank.morrow" || person["app"] != "who" {
		t.Errorf("hank = %v", person)
	}
}

func TestASharedLocalPartIsNobodysAlias(t *testing.T) {
	m := sampleModel(t)
	m.People = append(m.People, Person{ID: "z000000000001", Email: "hank.morrow@elsewhere.org", FullName: "Other Hank"})
	aliases := peopleType().Aliases(m)
	if _, ok := aliases["hank.morrow"]; ok {
		t.Error("a local part two people share resolves to one of them")
	}
	if aliases["hank.morrow@elsewhere.org"] != "z000000000001" {
		t.Error("the full address stopped resolving")
	}
}

func TestListedPeopleAreTheDirectorysListed(t *testing.T) {
	m := sampleModel(t)
	code, out := resourceGet(t, m, "/api/people?listed")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var got []string
	if err := json.Unmarshal(out.Data, &got); err != nil {
		t.Fatal(err)
	}
	want := []string{}
	for _, p := range m.Listed() {
		want = append(want, p.ID)
	}
	if !slices.Equal(got, want) {
		t.Errorf("listed %d people, want %d in the same order", len(got), len(want))
	}
	if code, _ := resourceGet(t, m, "/api/people?listed=no"); code != http.StatusBadRequest {
		t.Errorf("listed with a value: status %d", code)
	}
}

func TestHouseholdRelationsFollowTheFamilies(t *testing.T) {
	m := sampleModel(t)
	idOf := func(email string) string { return m.Person(email).ID }
	jordan, robin, sam, ella := idOf("jordan.whitfield@heliosschool.org"), idOf("robin.whitfield@heliosschool.org"), idOf("sam.whitfield@heliosschool.org"), idOf("ella.whitfield@heliosschool.org")
	_, out := resourceGet(t, m, "/api/people/"+jordan+"?include=partners,children,parents,families.kids")
	sorted := func(ids ...string) []string {
		slices.Sort(ids)
		return ids
	}
	if got := related(out, jordan, "partners"); !slices.Equal(got, []string{robin}) {
		t.Errorf("partners %v", got)
	}
	if got := related(out, jordan, "children"); !slices.Equal(got, sorted(sam, ella)) {
		t.Errorf("children %v", got)
	}
	if got := related(out, jordan, "parents"); len(got) != 0 {
		t.Errorf("a parent has parents %v", got)
	}
	families := out.Included["people"][jordan]["families"].([]any)
	if len(families) != 1 {
		t.Fatalf("families %v", families)
	}
	kids := out.Included["families"][families[0].(string)]["kids"].([]any)
	if len(kids) != 2 {
		t.Errorf("family kids %v", kids)
	}
	_, out = resourceGet(t, m, "/api/people/"+sam+"?include=parents,siblings,partners")
	if got := related(out, sam, "parents"); !slices.Equal(got, sorted(jordan, robin)) {
		t.Errorf("sam's parents %v", got)
	}
	if got := related(out, sam, "siblings"); !slices.Equal(got, []string{ella}) {
		t.Errorf("sam's siblings %v", got)
	}
	if got := related(out, sam, "partners"); len(got) != 0 {
		t.Errorf("a student has partners %v", got)
	}
}

func related(r resourceReply, from, name string) []string {
	out := []string{}
	list, _ := r.Included["people"][from][name].([]any)
	for _, v := range list {
		out = append(out, v.(string))
	}
	slices.Sort(out)
	return out
}

func TestClassroomsAndGradesResolveByTheirPagesSlug(t *testing.T) {
	m := sampleModel(t)
	room := m.Classrooms[0]
	if got := resourceID(t, m, "/api/classrooms/"+ClassroomSlug(room.Name)); got != room.ID {
		t.Errorf("classroom slug resolved to %s", got)
	}
	grade := m.Grades[0]
	if got := resourceID(t, m, "/api/grades/"+ClassroomSlug(grade.Name)); got != grade.ID {
		t.Errorf("grade slug resolved to %s", got)
	}
	_, out := resourceGet(t, m, "/api/classrooms/"+room.ID+"?include=students,crews.teachers")
	students := out.Included["classrooms"][room.ID]["students"].([]any)
	if len(students) == 0 {
		t.Error("a classroom with no students")
	}
	for _, s := range students {
		if out.Included["people"][s.(string)]["classroom"] != room.Name {
			t.Errorf("%v is not in %s", out.Included["people"][s.(string)], room.Name)
		}
	}
	if len(out.Included["crews"]) == 0 {
		t.Error("a classroom with no crews")
	}
}
