package model

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/store"
)

func (s server) read(t *testing.T, as, path string) reply {
	t.Helper()
	rec := s.call(t, as, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
	}
	var out reply
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func canOf(r reply, typ, key string) map[string]any {
	can, _ := r.Resources[typ][key]["can"].(map[string]any)
	return can
}

func TestAParentEditsTheirHouseholdThroughThePeople(t *testing.T) {
	s := newServer(t)
	devID, rohanID := s.personID(dev), s.personID(rohan)
	read := s.read(t, asha, "/api/people/"+devID)
	if can := canOf(read, "people", devID); can["edit"] != true || can["add-photo"] != true {
		t.Fatalf("a parent's can on their kid: %v", can)
	}
	read = s.read(t, asha, "/api/people/"+rohanID)
	if can := canOf(read, "people", rohanID); can["edit"] != false {
		t.Fatalf("a parent's can on another parent: %v", can)
	}
	s.ok(t, asha, "POST", "/api/people/"+devID+"/edit", `{"pronouns":"They/Them","facts":"Likes chess"}`)
	p := s.directory().Person(dev)
	if p.Pronouns != "they/them" || p.Facts != "Likes chess" {
		t.Fatalf("after the edit: pronouns %q facts %q", p.Pronouns, p.Facts)
	}
	for _, line := range s.changeLog(t) {
		if strings.Contains(line, "Preferred Name") {
			t.Errorf("a field that was not sent was written: %s", line)
		}
	}
	if rec := s.call(t, asha, "POST", "/api/people/"+rohanID+"/edit", `{"pronouns":"he/him"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("editing another parent: %d %s", rec.Code, rec.Body)
	}
	if rec := s.call(t, asha, "POST", "/api/people/"+devID+"/edit", `{"preferredName":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("an empty preferred name: %d %s", rec.Code, rec.Body)
	}
}

func TestPhotosAndPronunciationsNameWhatWasUploaded(t *testing.T) {
	s := newServer(t)
	devID := s.personID(dev)
	photo := s.upload(t, asha, "photo", "dev.png", pngBytes(t))
	s.ok(t, asha, "POST", "/api/people/"+devID+"/add-photo", `{"name":"`+photo+`"}`)
	if !slices.ContainsFunc(s.directory().Person(dev).Photos, func(p Photo) bool { return p.Name == photo }) {
		t.Fatalf("photos after the add: %+v", s.directory().Person(dev).Photos)
	}
	if rec := s.call(t, asha, "POST", "/api/people/"+devID+"/add-photo", `{"name":"`+photo+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("adding the same photo twice: %d %s", rec.Code, rec.Body)
	}
	if rec := s.call(t, asha, "POST", "/api/people/"+devID+"/add-photo", `{"name":"../../etc/passwd"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a name nothing uploaded: %d %s", rec.Code, rec.Body)
	}
	voice := s.upload(t, asha, "pronunciation", "dev.webm", []byte("webm audio"))
	s.ok(t, asha, "POST", "/api/people/"+devID+"/pronunciation", `{"name":"`+voice+`"}`)
	if s.count(t, overridesTab, store.Row{"Email": dev, "Pronunciation": voice}) != 1 {
		t.Fatalf("overrides after the pronunciation: %v", s.rows(t, overridesTab))
	}
	s.ok(t, asha, "POST", "/api/people/"+devID+"/pronunciation", `{"name":""}`)
	if s.count(t, overridesTab, store.Row{"Email": dev, "Pronunciation": voice}) != 0 {
		t.Fatal("clearing the pronunciation left it")
	}
}

func TestAFamilysAdultsEditItsCaptionAndPhoto(t *testing.T) {
	s := newServer(t)
	ashas, rohans := familyID(sampleKey, asha), familyID(sampleKey, rohan)
	s.ok(t, asha, "POST", "/api/families/"+ashas+"/edit", `{"photoCaption":"At the beach"}`)
	if got := s.directory().Families[ashas].PhotoCaption; got != "At the beach" {
		t.Fatalf("caption %q", got)
	}
	photo := s.upload(t, asha, "photo", "family.png", pngBytes(t))
	s.ok(t, asha, "POST", "/api/families/"+ashas+"/photo", `{"name":"`+photo+`"}`)
	if got := s.directory().Families[ashas].OriginalPhotoURL; !strings.Contains(got, photo) {
		t.Fatalf("family photo %q, want %s", got, photo)
	}
	if rec := s.call(t, asha, "POST", "/api/families/"+rohans+"/edit", `{"photoCaption":"Mine now"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("editing another family: %d %s", rec.Code, rec.Body)
	}
}

func TestPersonRecordsAreTheAdminsAndEditOnlyWhatWasSent(t *testing.T) {
	s := newServer(t)
	if rec := s.call(t, asha, "GET", "/api/person-records/"+s.record(dev), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("a parent reading a record: %d", rec.Code)
	}
	var none []string
	if err := json.Unmarshal(s.read(t, asha, "/api/person-records").Result, &none); err != nil || len(none) != 0 {
		t.Fatalf("a parent lists records %v", none)
	}
	devRecord := s.record(dev)
	s.ok(t, jordan, "POST", "/api/person-records/"+devRecord+"/edit", `{"legalName":"Devendra Chandra"}`)
	log := s.changeLog(t)
	logged(t, log, jordan+"|insert|Overrides|Email="+dev+"||")
	for _, line := range log {
		if strings.Contains(line, "|Full Name|") || strings.Contains(line, "|Classroom|") {
			t.Errorf("a field that was not sent was written: %s", line)
		}
	}
	if got := overrideStringValue(s.directory().Person(dev), "Legal Name"); got != "Devendra Chandra" {
		t.Fatalf("legal name override %q", got)
	}
	for body, want := range map[string]int{
		`{"jobTitle":"Chef"}`:               http.StatusBadRequest,
		`{"grade":"Grade 99"}`:              http.StatusBadRequest,
		`{"email":"dev2@heliosschool.org"}`: http.StatusBadRequest,
	} {
		if rec := s.call(t, jordan, "POST", "/api/person-records/"+devRecord+"/edit", body); rec.Code != want {
			t.Errorf("%s: %d %s, want %d", body, rec.Code, rec.Body, want)
		}
	}
	s.ok(t, jordan, "POST", "/api/person-records/"+s.record(asha)+"/edit", `{"address":"1 Elm St"}`)
	if got := familyStringValue(s.directory().Families[familyID(sampleKey, asha)], "Address"); got != "1 Elm St" {
		t.Fatalf("the family's address %q", got)
	}
	noaRecord := s.record(noa)
	s.ok(t, jordan, "POST", "/api/person-records/"+noaRecord+"/hide", "")
	if s.directory().Person(noa) != nil {
		t.Fatal("the hidden person is still listed")
	}
	record := s.read(t, jordan, "/api/person-records/"+noaRecord)
	if got := record.Resources["person-records"][noaRecord]; got["hidden"] != true || canOf(record, "person-records", noaRecord)["unhide"] != true {
		t.Fatalf("the hidden record %v", got)
	}
	s.ok(t, jordan, "POST", "/api/person-records/"+noaRecord+"/unhide", "")
	if s.directory().Person(noa) == nil {
		t.Fatal("the unhidden person is not back")
	}
	made := s.made(t, jordan, "/api/person-records", `{"email":"sasha.pike@heliosschool.org","fullName":"Sasha Pike","isStaff":true}`)
	if made != s.record("sasha.pike@heliosschool.org") || s.directory().Person("sasha.pike@heliosschool.org") == nil {
		t.Fatalf("the added person: %s", made)
	}
}

func TestWhoSettingsCarryTheConfigAndItsEdits(t *testing.T) {
	s := newServer(t)
	key := derived(s.store.Model(), kindWhoSettings, "")
	read := s.read(t, asha, "/api/who-settings?include=viewer")
	settings := read.Resources["who-settings"][key]
	if settings["staffColor"] != "#1f4d53" || canOf(read, "who-settings", key)["privacy-links"] != false {
		t.Fatalf("a parent's settings %v", settings)
	}
	if viewer, _ := settings["viewer"].(string); viewer != s.personID(asha) {
		t.Fatalf("viewer %v", settings["viewer"])
	}
	body := `{"veracrossPreferences":"https://example.org/prefs","heliosWhoOptIn":"https://example.org/optin"}`
	if rec := s.call(t, asha, "POST", "/api/who-settings/"+key+"/privacy-links", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent setting privacy links: %d", rec.Code)
	}
	s.ok(t, jordan, "POST", "/api/who-settings/"+key+"/privacy-links", body)
	if got := s.store.Model().Config.PrivacyLinks.HeliosWhoOptIn; got != "https://example.org/optin" {
		t.Fatalf("opt-in link %q", got)
	}
}

func TestAGradeListsItsBandsRoomParents(t *testing.T) {
	s := newServer(t)
	d := s.directory()
	i := slices.IndexFunc(d.Grades, func(g Grade) bool { return len(d.RoomParentsOf(g.Band)) > 0 })
	if i < 0 {
		t.Fatal("the sample has no room parents")
	}
	g := d.Grades[i]
	read := s.read(t, asha, "/api/grades/"+g.ID+"?include=room-parents")
	got := []string{}
	for _, v := range read.Resources["grades"][g.ID]["room-parents"].([]any) {
		got = append(got, read.Resources["people"][v.(string)]["email"].(string))
	}
	want := slices.Sorted(slices.Values(d.RoomParentsOf(g.Band)))
	if slices.Sort(got); !slices.Equal(got, want) {
		t.Fatalf("room parents of %s: %v, want %v", g.Name, got, want)
	}
}
