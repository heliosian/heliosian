package model

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
	Result    json.RawMessage                      `json:"result"`
	Resources map[string]map[string]map[string]any `json:"resources"`
}

func resourceGet(t *testing.T, m *Directory, path string) (int, resourceReply) {
	t.Helper()
	return resourceGetAs(t, m, "ruth.amari@heliosschool.org", path)
}

func resourceGetAs(t *testing.T, m *Directory, as, path string) (int, resourceReply) {
	t.Helper()
	reg := api.New(api.Config[*Directory]{
		Actor: func(*http.Request, *Directory) access.Actor { return access.Actor{Email: as} },
		Held:  func(string) []access.Allowance { return nil },
		Now:   time.Now,
		Scope: func(m *Directory, _ api.Query) *Directory { return m },
	})
	for _, rt := range DirectoryResources() {
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

func resourceID(t *testing.T, m *Directory, path string) string {
	t.Helper()
	code, out := resourceGet(t, m, path)
	if code != http.StatusOK {
		t.Fatalf("%s: status %d", path, code)
	}
	var got string
	if err := json.Unmarshal(out.Result, &got); err != nil {
		t.Fatalf("%s: result %s", path, out.Result)
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
	person := out.Resources["people"][hank]
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
	if err := json.Unmarshal(out.Result, &got); err != nil {
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
	families := out.Resources["people"][jordan]["families"].([]any)
	if len(families) != 1 {
		t.Fatalf("families %v", families)
	}
	kids := out.Resources["families"][families[0].(string)]["kids"].([]any)
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
	list, _ := r.Resources["people"][from][name].([]any)
	for _, v := range list {
		out = append(out, v.(string))
	}
	slices.Sort(out)
	return out
}

func TestDepartmentsKeepTheDirectorysOrder(t *testing.T) {
	m := sampleModel(t)
	_, out := resourceGet(t, m, "/api/departments")
	var keys []string
	if err := json.Unmarshal(out.Result, &keys); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, key := range keys {
		names = append(names, out.Resources["departments"][key]["name"].(string))
	}
	if !slices.Equal(names, m.Departments) {
		t.Errorf("departments %v, want %v", names, m.Departments)
	}
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
	students := out.Resources["classrooms"][room.ID]["students"].([]any)
	if len(students) == 0 {
		t.Error("a classroom with no students")
	}
	for _, s := range students {
		if out.Resources["people"][s.(string)]["classroom"] != room.Name {
			t.Errorf("%v is not in %s", out.Resources["people"][s.(string)], room.Name)
		}
	}
	if len(out.Resources["crews"]) == 0 {
		t.Error("a classroom with no crews")
	}
}

func TestEnrolledGradesAreTheOnesWithStudents(t *testing.T) {
	m := sampleModel(t)
	empty := m.Grades[0]
	for i := range m.People {
		if m.People[i].Grade == empty.Name {
			m.People[i].Grade = ""
		}
	}
	_, out := resourceGet(t, m, "/api/grades?enrolled")
	var got []string
	if err := json.Unmarshal(out.Result, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(m.Grades)-1 || slices.Contains(got, empty.ID) {
		t.Fatalf("enrolled %v of %d, with %s emptied", got, len(m.Grades), empty.Name)
	}
	for _, key := range got {
		name := out.Resources["grades"][key]["name"]
		if !slices.ContainsFunc(m.People, func(p Person) bool { return p.IsStudent && p.Grade == name }) {
			t.Errorf("%s has no student", name)
		}
	}
}

func TestATagIsItsOwnersAndItsManagersAlone(t *testing.T) {
	m := sampleModel(t)
	const (
		carpool  = "dtg0000000001"
		soccer   = "dtg0000000002"
		bookClub = "dtg0000000003"
	)
	for as, want := range map[string][]string{
		"jordan.whitfield@heliosschool.org": {carpool, soccer, bookClub},
		"asha.chandra@heliosschool.org":     {soccer},
		"ruth.amari@heliosschool.org":       {},
	} {
		_, out := resourceGetAs(t, m, as, "/api/tags?include=people,owner")
		got := []string{}
		if err := json.Unmarshal(out.Result, &got); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s lists %v, want %v", as, got, want)
		}
		for _, key := range got {
			tag := out.Resources["tags"][key]
			mine := tag["me"].(map[string]any)["mine"]
			if owner := out.Resources["people"][tag["owner"].(string)]["email"]; (owner == as) != mine {
				t.Errorf("%s: tag %v owned by %v reads mine %v", as, tag["name"], owner, mine)
			}
			if len(tag["people"].([]any)) == 0 {
				t.Errorf("%s: tag %v holds nobody", as, tag["name"])
			}
		}
	}
	if code, _ := resourceGetAs(t, m, "ruth.amari@heliosschool.org", "/api/tags/"+carpool); code != http.StatusNotFound {
		t.Errorf("a stranger fetching a tag got %d", code)
	}
}
