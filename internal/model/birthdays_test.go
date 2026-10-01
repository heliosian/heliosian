package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/testkit/mailtest"
)

var birthdaysSent *mailtest.Recorder

const parent = "robin.whitfield@heliosschool.org"

const (
	secondHarvest = "chy0000000001"
	rocketDog     = "chy0000000002"
	birthfund     = "chy0000000003"
	wikipedia     = "chy0000000005"
	sierraClub    = "chy0000000007"
	issueSep11    = "nwd0000000024"
	issueOct23    = "nwd0000000030"
	unknownID     = "zzz9999999999"
)

func birthdaysSheet(t *testing.T) {
	t.Helper()
	sampleSheet(t)
	was := now
	t.Cleanup(func() { now = was })
	now = func() time.Time { return testkit.MustTime("2026-09-09") }
}

func birthdaysOver(t *testing.T) (*Store, *http.ServeMux) {
	t.Helper()
	s := sampleStore(t, sheet, queue, sampleDeps(sampleKey))
	mux := http.NewServeMux()
	birthdaysSent = mailtest.NewRecorder("Helios Staff Birthdays <birthday@example.org>")
	reg := typedRegistry(s, queue, DirectoryResources(s), BirthdayResources(s))
	reg.Register(mux)
	RegisterBirthdays(mux, BirthdaysDeps{
		Store:     s,
		Queue:     queue,
		Describer: describe.New("test", claude.NewLimiter()),
		Mailer:    birthdaysSent.Mailgun,
		Base:      "https://birthday.example.org",
		About:     BirthdaysAbout(func() string { return "Helios Birthday Team" }, func() string { return "Staff birthday donations" }),
		Taken:     reg.Taken,
	})
	return s, mux
}

func birthdaysServer(t *testing.T) (*Store, *http.ServeMux) {
	t.Helper()
	birthdaysSheet(t)
	return birthdaysOver(t)
}

type birthdaysReply struct {
	Result    json.RawMessage                      `json:"result"`
	Resources map[string]map[string]map[string]any `json:"resources"`
}

func get(t *testing.T, mux http.Handler, as, path string) birthdaysReply {
	t.Helper()
	rec := testkit.Call(t, mux, as, "GET", path, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
	}
	var out birthdaysReply
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return out
}

func (r birthdaysReply) ids(t *testing.T) []string {
	t.Helper()
	out := []string{}
	if err := json.Unmarshal(r.Result, &out); err != nil {
		t.Fatalf("result %s: %v", r.Result, err)
	}
	return out
}

func (r birthdaysReply) id(t *testing.T) string {
	t.Helper()
	var out string
	if err := json.Unmarshal(r.Result, &out); err != nil {
		t.Fatalf("result %s: %v", r.Result, err)
	}
	return out
}

func (r birthdaysReply) follow(from map[string]any, relation, typ string) map[string]any {
	key, _ := from[relation].(string)
	return r.Resources[typ][key]
}

const staffIncludes = "?include=person,assignee,donation,last-donation,notes,invites"

func staff(t *testing.T, mux http.Handler, as, email string) (map[string]any, birthdaysReply) {
	t.Helper()
	out := get(t, mux, as, "/api/birthdays/"+email+staffIncludes)
	return out.Resources["birthdays"][out.id(t)], out
}

func everyone(t *testing.T, mux http.Handler, as string) map[string]map[string]any {
	t.Helper()
	out := get(t, mux, as, "/api/birthdays")
	byEmail := map[string]map[string]any{}
	for _, key := range out.ids(t) {
		b := out.Resources["birthdays"][key]
		byEmail[b["email"].(string)] = b
	}
	return byEmail
}

func birthdaysCall(t *testing.T, mux http.Handler, as, method, path string, body any, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := testkit.Call(t, mux, as, method, path, body)
	if rec.Code != want {
		t.Fatalf("%s %s as %s: %d %s, want %d", method, path, as, rec.Code, rec.Body, want)
	}
	return rec
}

func birthdaysCreated(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == "" {
		t.Fatalf("created %s: %v", rec.Body, err)
	}
	return out.ID
}

func settingsID(t *testing.T, mux http.Handler) string {
	t.Helper()
	return get(t, mux, parent, "/api/birthday-settings").ids(t)[0]
}

type birthdaysWrite struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   any    `json:"body,omitempty"`
}

func TestSampleLoads(t *testing.T) {
	s, _ := birthdaysServer(t)
	m := s.Model().Birthdays
	if len(m.Birthdays) != 18 || len(m.Charities) != 7 || len(m.NewsletterDates) != 57 || len(m.Invites) != 3 {
		t.Fatalf("got %d birthdays, %d charities, %d newsletter dates, %d invites", len(m.Birthdays), len(m.Charities), len(m.NewsletterDates), len(m.Invites))
	}
	if !m.Skipped("hank.morrow@heliosschool.org") || m.InPipeline("hank.morrow@heliosschool.org") || m.Skipped("omar.farouk@heliosschool.org") || !m.InPipeline("omar.farouk@heliosschool.org") {
		t.Fatal("participation levels did not load")
	}
	if _, err := BuildBirthdays(store.Tables{birthdaysTab: {{"Email": "x@heliosschool.org", "Participation": LevelNoNewsletter}}, charitiesTab: m.charityRows(), birthdaySettingsTab: m.settingRows()}, nil); err == nil {
		t.Fatal("a blank birthday without a skip loaded")
	}
	if b := m.Birthday("kate.doyle@heliosschool.org"); b.Override != issueOct23 || m.NewsletterDate(b.Override).Date != "2026-10-23" {
		t.Fatalf("kate's override: %+v", b)
	}
	if _, err := BuildBirthdays(store.Tables{birthdaysTab: {{"Email": "x@heliosschool.org", "Birthday": "09-01", "Newsletter Override": "2026-10-23"}}, charitiesTab: m.charityRows(), birthdaySettingsTab: m.settingRows()}, nil); err == nil {
		t.Fatal("an override naming a date rather than a newsletter date's id loaded")
	}
	unnamed := m.charityRows()
	unnamed[0]["Charity ID"] = ""
	if _, err := BuildBirthdays(store.Tables{charitiesTab: unnamed, birthdaySettingsTab: m.settingRows()}, nil); err == nil {
		t.Fatal("a charity without an id loaded")
	}
	twice := m.charityRows()
	twice[1]["Charity ID"] = twice[0]["Charity ID"]
	if _, err := BuildBirthdays(store.Tables{charitiesTab: twice, birthdaySettingsTab: m.settingRows()}, nil); err == nil {
		t.Fatal("two charities sharing an id loaded")
	}
	if _, err := BuildBirthdays(store.Tables{charitiesTab: m.charityRows(), birthdaySettingsTab: m.settingRows(), newsletterDatesTab: {{"Newsletter Date ID": secondHarvest, "Date": "2026-09-11"}}}, nil); err == nil {
		t.Fatal("a newsletter date sharing a charity's id loaded")
	}
	if _, err := BuildBirthdays(store.Tables{charitiesTab: m.charityRows(), birthdaySettingsTab: m.settingRows(), birthdayInvitesTab: {{"Invite ID": "bnv0000000009", "Email": "nobody@heliosschool.org", "Year": "2026 - 2027", "Requested On": "2026-09-01", "Requested By": parent}}}, nil); err == nil {
		t.Fatal("an invite for someone with no birthday loaded")
	}
	half := store.Tables{birthdaysTab: {{"Email": "bill.ryder@heliosschool.org", "Birthday": "09-15"}}, charitiesTab: m.charityRows(), birthdaySettingsTab: m.settingRows(),
		birthdayInvitesTab: {{"Invite ID": "bnv0000000009", "Email": "bill.ryder@heliosschool.org", "Year": "2026 - 2027", "Requested On": "2026-09-01", "Requested By": parent, "Sent On": "2026-09-01"}}}
	if _, err := BuildBirthdays(half, nil); err == nil {
		t.Fatal("an invite sent to nobody loaded")
	}
}

func TestBirthdaysYears(t *testing.T) {
	y := BirthdayYearContaining(testkit.MustTime("2026-09-09"), time.August, 14)
	if y.Label != "2026 - 2027" || y.Start != testkit.MustTime("2026-08-14") || y.End != testkit.MustTime("2027-08-14") {
		t.Fatalf("year: %+v", y)
	}
	if got := BirthdayYearContaining(testkit.MustTime("2026-08-13"), time.August, 14).Label; got != "2025 - 2026" {
		t.Errorf("the day before the turnover: %s", got)
	}
	if got := y.Occurrence(time.June, 20); got != testkit.MustTime("2027-06-20") {
		t.Errorf("summer birthday: %s", got)
	}
	if got := y.Occurrence(time.February, 29); got != testkit.MustTime("2027-02-28") {
		t.Errorf("leap day: %s", got)
	}
	if got := y.Occurrence(time.August, 13); got != testkit.MustTime("2027-08-13") {
		t.Errorf("last day of the year: %s", got)
	}
	dates := []string{"2026-08-21", "2026-09-04", "2026-09-18", "2027-06-04", "2027-09-03"}
	if d, ok := y.Newsletter(testkit.MustTime("2026-09-10"), dates); !ok || d != testkit.MustTime("2026-09-04") {
		t.Errorf("the last newsletter before: %s %v", d, ok)
	}
	if d, ok := y.Newsletter(testkit.MustTime("2026-09-04"), dates); !ok || d != testkit.MustTime("2026-08-21") {
		t.Errorf("a birthday on an issue's day goes out the issue before: %s %v", d, ok)
	}
	if d, ok := y.Newsletter(testkit.MustTime("2026-08-20"), dates); !ok || d != testkit.MustTime("2026-08-21") {
		t.Errorf("a birthday before the first issue lands in it: %s %v", d, ok)
	}
	if d, ok := y.Newsletter(testkit.MustTime("2027-06-20"), dates); !ok || d != testkit.MustTime("2027-06-04") {
		t.Errorf("last newsletter before a summer birthday: %s %v", d, ok)
	}
	if _, ok := y.Newsletter(testkit.MustTime("2026-09-01"), []string{"2025-09-05"}); ok {
		t.Error("a date outside the year was picked")
	}
	if got := RequestBy(testkit.MustTime("2026-08-21"), DefaultRequestLeadDays); got != testkit.MustTime("2026-08-13") {
		t.Errorf("request by: %s", got)
	}
	if err := CheckYearSpan("2026 - 2028"); err == nil {
		t.Error("a two-year span passed")
	}
}

func TestBirthdayResources(t *testing.T) {
	st, mux := birthdaysServer(t)
	settings := get(t, mux, parent, "/api/birthday-settings")
	s := settings.Resources["birthday-settings"][settings.ids(t)[0]]
	year := s["year"].(map[string]any)
	if year["current"] != "2026 - 2027" || year["last"] != "2025 - 2026" || year["start"] != "2026-08-14" || year["end"] != "2027-08-13" {
		t.Fatalf("year: %+v", year)
	}
	if me := s["me"].(map[string]any); me["team"] != true || s["settings"] == nil {
		t.Fatalf("robin's standing: %+v", s)
	}
	all := everyone(t, mux, parent)
	want := map[string]string{
		"dana.hawkins@heliosschool.org":   StageComplete,
		"bill.ryder@heliosschool.org":     StageResponse,
		"ruth.amari@heliosschool.org":     StageOutreach,
		"miguel.santos@heliosschool.org":  StageOutreach,
		"alice.fontaine@heliosschool.org": StageWait,
		"omar.farouk@heliosschool.org":    StageComplete,
		"tom.grady@heliosschool.org":      StageWait,
	}
	for email, stage := range want {
		if b := all[email]; b == nil || b["stage"] != stage {
			t.Errorf("%s: want %s, got %+v", email, stage, b)
		}
	}
	dana, out := staff(t, mux, parent, "dana.hawkins@heliosschool.org")
	if dana["birthdayThisYear"] != "2026-08-20" || dana["newsletterDate"] != "2026-08-21" || dana["requestBy"] != "2026-08-13" || out.follow(dana, "assignee", "people")["fullName"] != "Jordan Whitfield" || dana["assignedTo"] != nil {
		t.Errorf("dana: %+v", dana)
	}
	if last, this := out.follow(dana, "last-donation", "donations"), out.follow(dana, "donation", "donations"); last["charity"] != birthfund || this["usedOn"] == nil {
		t.Errorf("dana's donations: %+v %+v", this, last)
	}
	miguel, out := staff(t, mux, parent, "miguel.santos@heliosschool.org")
	if miguel["assigned"] != false || out.follow(miguel, "person", "people")["department"] != "Classroom Teachers" {
		t.Errorf("miguel: %+v", miguel)
	}
	if u := miguel["urgency"].(map[string]any); u["when"] != "late" || u["step"] != LateOutreach {
		t.Errorf("miguel's urgency: %+v", u)
	}
	if kate := all["kate.doyle@heliosschool.org"]; kate["newsletterDate"] != "2026-10-23" || kate["requestBy"] != "2026-10-15" {
		t.Errorf("override: %+v", kate)
	}
	if hana := all["hana.ito@heliosschool.org"]; hana["birthdayThisYear"] != "2027-02-28" || hana["newsletterDate"] != "2027-02-26" {
		t.Errorf("leap day: %+v", hana)
	}
	if tom := all["tom.grady@heliosschool.org"]; tom["newsletterDate"] != "2027-06-04" || tom["requestBy"] != "2027-05-27" {
		t.Errorf("summer: %+v", tom)
	}
	omar, out := staff(t, mux, parent, "omar.farouk@heliosschool.org")
	if d := out.follow(omar, "donation", "donations"); omar["level"] != LevelNoNewsletter || d == nil || d["usedOn"] != nil {
		t.Errorf("no newsletter: %+v %+v", omar, d)
	}
	for _, unlisted := range []string{"former.teacher@heliosschool.org", "grace.kim@heliosschool.org"} {
		if all[unlisted] != nil {
			t.Errorf("%s, whom the directory does not list, reached the pipeline", unlisted)
		}
	}
	if bill, _ := staff(t, mux, parent, "bill.ryder@heliosschool.org"); len(bill["notes"].([]any)) != 1 {
		t.Errorf("notes: %+v", bill["notes"])
	}
	if hank := all["hank.morrow@heliosschool.org"]; hank["level"] != LevelSkip || hank["missing"] != nil {
		t.Errorf("skipped: %+v", hank)
	}
	missing := []string{}
	for email, b := range all {
		if b["missing"] == true {
			missing = append(missing, email)
		}
	}
	slices.Sort(missing)
	if !slices.Equal(missing, []string{"luis.ortega@heliosschool.org", "noa.adler@heliosschool.org"}) {
		t.Errorf("missing: %v", missing)
	}
	if b, _ := staff(t, mux, parent, "dana.hawkins"); b["email"] != "dana.hawkins@heliosschool.org" || b["path"] != "/staff/dana.hawkins" {
		t.Errorf("by the local part: %+v", b)
	}
	if can := all["dana.hawkins@heliosschool.org"]["can"].(map[string]any); can["delete"] != false || can["assign"] != true {
		t.Errorf("robin's can: %+v", can)
	}
	if can := everyone(t, mux, jordan)["noa.adler@heliosschool.org"]["can"].(map[string]any); can["set"] != true || can["delete"] != false {
		t.Errorf("an admin's can on a missing birthday: %+v", can)
	}
	departments := get(t, mux, parent, "/api/departments")
	if names := departments.ids(t); len(names) == 0 || departments.Resources["departments"][names[0]]["name"] != st.Model().Directory.Departments[0] {
		t.Errorf("departments: %+v", departments)
	}
}

func TestPipeline(t *testing.T) {
	st, mux := birthdaysServer(t)
	const miguelPath = "/api/birthdays/Miguel.Santos@heliosschool.org/"
	birthdaysCall(t, mux, parent, "POST", miguelPath+"assign", nil, http.StatusNoContent)
	sv, out := staff(t, mux, parent, "miguel.santos@heliosschool.org")
	if out.follow(sv, "assignee", "people")["email"] != parent || sv["assignedOn"] != "2026-09-09" || sv["stage"] != StageOutreach || sv["me"].(map[string]any)["mine"] != true {
		t.Fatalf("after assign: %+v", sv)
	}
	invites := birthdaysSent.Wait(t, 1)
	m := invites[len(invites)-1]
	if len(m.To) != 1 || m.To[0] != parent || m.Subject != "Ask Miguel Santos about their birthday charity" || !strings.Contains(m.HTML, "/staff/miguel.santos\"") || m.Headers["Message-ID"] == "" {
		t.Fatalf("invite mail: %+v", m)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Name != "invite.ics" {
		t.Fatalf("invite attachment: %+v", m.Attachments)
	}
	ics := strings.ReplaceAll(string(m.Attachments[0].Content), "\r\n ", "")
	for _, want := range []string{"METHOD:REQUEST", "DTSTART;VALUE=DATE:20260903", "DTEND;VALUE=DATE:20260904", "SUMMARY:Ask Miguel Santos about their birthday charity", "ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=FALSE:mailto:" + parent, "UID:birthday-miguel.santos@heliosschool.org-2026-2027@heliosian.com", "/staff/miguel.santos"} {
		if !strings.Contains(ics, want) {
			t.Fatalf("invite lacks %q:\n%s", want, ics)
		}
	}
	queue.Flush()
	sv, out = staff(t, mux, parent, "miguel.santos@heliosschool.org")
	if invites := sv["invites"].([]any); len(invites) != 1 || out.Resources["birthday-invites"][invites[0].(string)]["sentTo"] != parent {
		t.Fatalf("the sent invite was not recorded: %+v %+v", sv, out.Resources["birthday-invites"])
	}
	birthdaysCall(t, mux, parent, "POST", miguelPath+"contact", nil, http.StatusNoContent)
	if sv, _ = staff(t, mux, parent, "miguel.santos@heliosschool.org"); sv["stage"] != StageResponse || sv["contactedBy"] != parent || sv["can"].(map[string]any)["contact"] != false {
		t.Fatalf("after outreach: %+v", sv)
	}
	birthdaysCall(t, mux, parent, "POST", miguelPath+"donate", map[string]any{"charity": sierraClub}, http.StatusBadRequest)
	birthdaysCall(t, mux, parent, "POST", miguelPath+"donate", map[string]any{"charity": rocketDog, "note": "For the dogs"}, http.StatusNoContent)
	sv, out = staff(t, mux, parent, "miguel.santos@heliosschool.org")
	donation := out.follow(sv, "donation", "donations")
	if sv["stage"] != StageNewsletter || donation["note"] != "For the dogs" || donation["charity"] != rocketDog {
		t.Fatalf("after donation: %+v %+v", sv, donation)
	}
	donationPath := "/api/donations/" + sv["donation"].(string)
	birthdaysCall(t, mux, parent, "POST", donationPath+"/unuse", nil, http.StatusBadRequest)
	birthdaysCall(t, mux, parent, "POST", donationPath+"/use", nil, http.StatusNoContent)
	sv, out = staff(t, mux, parent, "miguel.santos@heliosschool.org")
	if sv["stage"] != StageComplete || out.follow(sv, "donation", "donations")["usedBy"] != parent {
		t.Fatalf("after used: %+v", sv)
	}
	birthdaysCall(t, mux, parent, "POST", donationPath+"/unuse", nil, http.StatusNoContent)
	sv, out = staff(t, mux, parent, "miguel.santos@heliosschool.org")
	if sv["stage"] != StageNewsletter || out.follow(sv, "donation", "donations")["usedOn"] != nil {
		t.Fatalf("after unused: %+v", sv)
	}
	birthdaysCall(t, mux, parent, "DELETE", donationPath, nil, http.StatusNoContent)
	birthdaysCall(t, mux, parent, "POST", miguelPath+"uncontact", nil, http.StatusNoContent)
	birthdaysCall(t, mux, parent, "POST", miguelPath+"unassign", nil, http.StatusNoContent)
	if sv, _ = staff(t, mux, parent, "miguel.santos@heliosschool.org"); sv["stage"] != StageOutreach || sv["assigned"] != false || sv["donation"] != nil {
		t.Fatalf("back to the start: %+v", sv)
	}
	birthdaysCall(t, mux, parent, "POST", "/api/birthdays/hank.morrow@heliosschool.org/assign", nil, http.StatusBadRequest)
	birthdaysCall(t, mux, parent, "POST", "/api/birthdays/noa.adler@heliosschool.org/assign", nil, http.StatusNotFound)
	birthdaysCall(t, mux, parent, "POST", miguelPath+"assign", map[string]any{"to": "x@elsewhere.example"}, http.StatusBadRequest)
	birthdaysCall(t, mux, parent, "POST", miguelPath+"assign", map[string]any{"to": jordan}, http.StatusNoContent)
	if sv, out = staff(t, mux, parent, "miguel.santos@heliosschool.org"); out.follow(sv, "assignee", "people")["email"] != jordan {
		t.Fatalf("after assigning to an admin: %+v", sv)
	}
	if m := birthdaysSent.Wait(t, 2)[1]; m.To[0] != jordan || strings.Contains(m.Text, "moved from") {
		t.Fatalf("a new assignee's invite: %+v", m)
	}
	birthdaysCall(t, mux, parent, "POST", miguelPath+"unassign", nil, http.StatusNoContent)
	birthdaysCall(t, mux, parent, "POST", miguelPath+"participation", map[string]any{"level": LevelSkip, "note": "Asked in person"}, http.StatusNoContent)
	birthdaysCall(t, mux, parent, "POST", miguelPath+"assign", nil, http.StatusBadRequest)
	if sv, _ = staff(t, mux, parent, "miguel.santos@heliosschool.org"); sv["level"] != LevelSkip {
		t.Fatalf("a skipped birthday: %+v", sv)
	}
	birthdaysCall(t, mux, parent, "POST", miguelPath+"clear-participation", nil, http.StatusNoContent)
	if b := st.Model().Birthdays.Birthday("miguel.santos@heliosschool.org"); b == nil || b.Level != "" || b.Birthday != "09-14" {
		t.Fatalf("after unskipping: %+v", b)
	}
	const noaPath = "/api/birthdays/noa.adler@heliosschool.org/"
	birthdaysCall(t, mux, parent, "POST", noaPath+"participation", map[string]any{"level": LevelNoNewsletter, "note": "Asked by email"}, http.StatusBadRequest)
	birthdaysCall(t, mux, parent, "POST", noaPath+"participation", map[string]any{"level": LevelSkip, "note": "Asked by email"}, http.StatusNoContent)
	if noa, _ := staff(t, mux, parent, "noa.adler@heliosschool.org"); noa["level"] != LevelSkip || noa["missing"] != nil {
		t.Fatalf("skipping someone with no birthday: %+v", noa)
	}
	birthdaysCall(t, mux, parent, "POST", noaPath+"clear-participation", nil, http.StatusNoContent)
	if noa, _ := staff(t, mux, parent, "noa.adler@heliosschool.org"); st.Model().Birthdays.Birthday("noa.adler@heliosschool.org") != nil || noa["missing"] != true {
		t.Fatal("a preference-only row survived clearing the preference")
	}
	birthdaysCall(t, mux, parent, "POST", miguelPath+"note", map[string]any{"note": "Out until Monday"}, http.StatusNoContent)
	sv, _ = staff(t, mux, parent, "miguel.santos@heliosschool.org")
	notePath := "/api/birthday-notes/" + sv["notes"].([]any)[0].(string)
	birthdaysCall(t, mux, "someone.else@heliosschool.org", "DELETE", notePath, nil, http.StatusNotFound)
	birthdaysCall(t, mux, "sam.whitfield@heliosschool.org", "POST", "/api/birthday-team", nil, http.StatusOK)
	birthdaysCall(t, mux, "sam.whitfield@heliosschool.org", "DELETE", notePath, nil, http.StatusForbidden)
	birthdaysCall(t, mux, jordan, "DELETE", notePath, nil, http.StatusNoContent)
}

func TestBirthdays(t *testing.T) {
	_, mux := birthdaysServer(t)
	const noaPath = "/api/birthdays/noa.adler@heliosschool.org"
	birthdaysCall(t, mux, parent, "POST", noaPath+"/set", map[string]any{"birthday": "09-12", "override": ""}, http.StatusNoContent)
	if noa, _ := staff(t, mux, parent, "noa.adler@heliosschool.org"); noa["newsletterDate"] != "2026-09-11" || noa["missing"] != nil {
		t.Fatalf("after adding a birthday: %+v", noa)
	}
	for _, bad := range []string{"September 12", "1995-09-12"} {
		birthdaysCall(t, mux, parent, "POST", noaPath+"/set", map[string]any{"birthday": bad}, http.StatusBadRequest)
	}
	if dana, _ := staff(t, mux, parent, "dana.hawkins@heliosschool.org"); dana["birthday"] != "08-20" {
		t.Fatalf("the resource carries more than a month and day: %+v", dana)
	}
	birthdaysCall(t, mux, parent, "DELETE", noaPath, nil, http.StatusForbidden)
	birthdaysCall(t, mux, jordan, "DELETE", "/api/birthdays/dana.hawkins@heliosschool.org", nil, http.StatusBadRequest)
	birthdaysCall(t, mux, jordan, "DELETE", noaPath, nil, http.StatusNoContent)
	if noa, _ := staff(t, mux, parent, "noa.adler@heliosschool.org"); noa["missing"] != true {
		t.Fatal("the removed birthday did not return to missing")
	}
	birthdaysCall(t, mux, parent, "POST", "/api/birthdays", map[string]any{"email": "nobody@elsewhere.example", "birthday": "09-12"}, http.StatusBadRequest)
	key := birthdaysCreated(t, birthdaysCall(t, mux, parent, "POST", "/api/birthdays", map[string]any{"email": "Noa.Adler@heliosschool.org", "level": LevelSkip, "note": "By email"}, http.StatusOK))
	if noa, _ := staff(t, mux, parent, "noa.adler@heliosschool.org"); noa["id"] != key || noa["level"] != LevelSkip {
		t.Fatalf("a preference added by address: %+v", noa)
	}
}

func TestCharities(t *testing.T) {
	st, mux := birthdaysServer(t)
	rec := birthdaysCall(t, mux, parent, "POST", "/api/charities", map[string]any{"name": "Oceana", "donationLink": "https://oceana.org/", "about": "Oceans", "allowed": false, "whyNotAllowed": "ignored"}, http.StatusOK)
	key := birthdaysCreated(t, rec)
	oceana := st.Model().Birthdays.Charity(key)
	if oceana == nil || oceana.Name != "Oceana" || !oceana.Allowed || oceana.WhyNotAllowed != "" || oceana.AddedOn != "2026-09-09" {
		t.Fatalf("a non-admin's addition: %+v", oceana)
	}
	if parsed, ok := id.Parse(key); !ok || parsed != key || strings.HasPrefix(key, "chy") {
		t.Fatalf("the minted charity id %q", key)
	}
	if got := get(t, mux, parent, "/api/charities/Oceana").id(t); got != key {
		t.Fatalf("by name: %s", got)
	}
	prohibit := map[string]any{"allowed": false, "whyNotAllowed": "Politics"}
	birthdaysCall(t, mux, parent, "POST", "/api/charities/"+key+"/allow", prohibit, http.StatusForbidden)
	birthdaysCall(t, mux, jordan, "POST", "/api/charities/"+key+"/allow", prohibit, http.StatusNoContent)
	if c := st.Model().Birthdays.Charity(key); c.Allowed || c.WhyNotAllowed != "Politics" {
		t.Fatalf("after prohibiting: %+v", c)
	}
	birthdaysCall(t, mux, parent, "POST", "/api/charities/"+key+"/edit", map[string]any{"name": "Oceana", "donationLink": "https://oceana.org/", "about": "The oceans"}, http.StatusNoContent)
	if c := st.Model().Birthdays.Charity(key); c.Allowed || c.About != "The oceans" {
		t.Fatalf("an edit touched whether it is allowed: %+v", c)
	}
	birthdaysCall(t, mux, jordan, "POST", "/api/charities/"+rocketDog+"/edit", map[string]any{"name": "Rocket Dog Rescue, Inc.", "donationLink": "https://www.rocketdogrescue.org/"}, http.StatusNoContent)
	if d, _ := st.Model().Birthdays.Donation("dana.hawkins@heliosschool.org", "2026 - 2027"); d.Charity != rocketDog || st.Model().Birthdays.charityName(d.Charity) != "Rocket Dog Rescue, Inc." {
		t.Fatalf("the donation lost its charity in the rename: %+v", d)
	}
	birthdaysCall(t, mux, jordan, "POST", "/api/charities/"+birthfund+"/edit", map[string]any{"name": "Oceana", "donationLink": "https://x.org/"}, http.StatusBadRequest)
	birthdaysCall(t, mux, jordan, "POST", "/api/charities/"+unknownID+"/edit", map[string]any{"name": "Nobody", "donationLink": "https://x.org/"}, http.StatusNotFound)
	birthdaysCall(t, mux, jordan, "DELETE", "/api/charities/"+secondHarvest, nil, http.StatusBadRequest)
	birthdaysCall(t, mux, jordan, "DELETE", "/api/charities/"+birthfund, nil, http.StatusBadRequest)
	birthdaysCall(t, mux, parent, "DELETE", "/api/charities/"+key, nil, http.StatusForbidden)
	birthdaysCall(t, mux, jordan, "DELETE", "/api/charities/"+key, nil, http.StatusNoContent)
	if st.Model().Birthdays.charityNamed("Oceana") != nil {
		t.Fatal("the charity survived removal")
	}
	settings := "/api/birthday-settings/" + settingsID(t, mux) + "/edit"
	birthdaysCall(t, mux, jordan, "POST", settings, map[string]any{"defaultCharity": rocketDog, "yearStart": "08-14", "emailSubject": "Hi", "emailBody": "Body", "noNewsletterNote": "Note", "requestLeadDays": 12}, http.StatusNoContent)
	birthdaysCall(t, mux, jordan, "POST", settings, map[string]any{"defaultCharity": sierraClub, "yearStart": "08-14", "emailSubject": "Hi", "emailBody": "Body", "noNewsletterNote": "Note"}, http.StatusBadRequest)
	birthdaysCall(t, mux, jordan, "POST", settings, map[string]any{"defaultCharity": "Rocket Dog Rescue, Inc.", "yearStart": "08-14", "emailSubject": "Hi", "emailBody": "Body", "noNewsletterNote": "Note"}, http.StatusBadRequest)
	birthdaysCall(t, mux, parent, "POST", settings, map[string]any{"defaultCharity": rocketDog, "yearStart": "08-14", "emailSubject": "Hi", "emailBody": "Body", "noNewsletterNote": "Note"}, http.StatusForbidden)
}

func TestTeam(t *testing.T) {
	st, mux := birthdaysServer(t)
	birthdaysCall(t, mux, parent, "POST", "/api/birthday-team", map[string]any{"email": "someone.new@gmail.com", "role": RoleVolunteer}, http.StatusForbidden)
	birthdaysCall(t, mux, parent, "POST", "/api/birthday-team", map[string]any{"role": RoleComms}, http.StatusForbidden)
	birthdaysCall(t, mux, jordan, "POST", "/api/birthday-team", map[string]any{"email": parent, "role": "Boss"}, http.StatusBadRequest)
	keys := []string{}
	for _, role := range []string{RoleVolunteer, RoleComms, RoleVolunteer} {
		rec := birthdaysCall(t, mux, jordan, "POST", "/api/birthday-team", map[string]any{"email": "Robin.Whitfield@heliosschool.org ", "role": role}, http.StatusOK)
		keys = append(keys, birthdaysCreated(t, rec))
	}
	if keys[0] != keys[2] || keys[0] == keys[1] {
		t.Fatalf("team ids: %v", keys)
	}
	roles := func(email string) []string {
		out := []string{}
		for _, m := range st.Model().Birthdays.Team {
			if m.Email == email {
				out = append(out, m.Role)
			}
		}
		return out
	}
	if got := roles(parent); len(got) != 2 || got[0] != RoleVolunteer || got[1] != RoleComms {
		t.Fatalf("roles after adding: %v", got)
	}
	birthdaysCall(t, mux, parent, "DELETE", "/api/birthday-team/"+keys[0], nil, http.StatusForbidden)
	birthdaysCall(t, mux, jordan, "DELETE", "/api/birthday-team/"+keys[0], nil, http.StatusNoContent)
	if got := roles(parent); len(got) != 1 || got[0] != RoleComms {
		t.Fatalf("roles after removing: %v", got)
	}
	birthdaysCall(t, mux, jordan, "POST", "/api/birthday-team", map[string]any{"email": "someone.new@gmail.com", "role": RoleVolunteer}, http.StatusOK)
	out := get(t, mux, parent, "/api/birthday-team?include=person")
	byEmail := map[string]map[string]any{}
	for _, key := range out.ids(t) {
		m := out.Resources["birthday-team"][key]
		if person := out.follow(m, "person", "people"); person != nil {
			if m["email"] != nil {
				t.Errorf("a directory member carries an address too: %+v", m)
			}
			byEmail[person["email"].(string)] = m
			continue
		}
		byEmail[m["email"].(string)] = m
	}
	if m := byEmail["someone.new@gmail.com"]; m == nil || m["role"] != RoleVolunteer {
		t.Fatalf("someone outside the directory: %+v", byEmail)
	}
	if m := byEmail[parent]; m == nil || m["me"].(map[string]any)["mine"] != true {
		t.Fatalf("robin's own row: %+v", m)
	}
	settings := get(t, mux, parent, "/api/birthday-settings")
	if me := settings.Resources["birthday-settings"][settings.ids(t)[0]]["me"].(map[string]any); me["comms"] != true || me["volunteer"] != false || me["commsOnly"] != true {
		t.Fatalf("robin's standing: %+v", me)
	}
}

func TestMovedNewsletterRefreshesInvite(t *testing.T) {
	_, mux := birthdaysServer(t)
	birthdaysCall(t, mux, parent, "POST", "/api/birthdays/miguel.santos@heliosschool.org/assign", nil, http.StatusNoContent)
	birthdaysSent.Wait(t, 1)
	birthdaysCall(t, mux, jordan, "POST", "/api/newsletter-dates/"+issueSep11+"/move", map[string]any{"date": "2026-09-12"}, http.StatusNoContent)
	if sv, _ := staff(t, mux, parent, "miguel.santos@heliosschool.org"); sv["requestBy"] != "2026-09-04" {
		t.Fatalf("request by after the move: %+v", sv)
	}
	msgs := birthdaysSent.Wait(t, 4)
	var m *mail.Message
	for i := range msgs[1:] {
		if strings.Contains(msgs[i+1].Subject, "Miguel Santos") {
			m = &msgs[i+1]
		}
	}
	if m == nil || m.To[0] != parent || m.Subject != "Re: Ask Miguel Santos about their birthday charity" || m.Headers["In-Reply-To"] == "" || !strings.Contains(m.Text, "moved from September 3, 2026 to September 4, 2026") {
		t.Fatalf("updated invite: %+v", msgs)
	}
	if ics := string(m.Attachments[0].Content); !strings.Contains(ics, "DTSTART;VALUE=DATE:20260904") {
		t.Fatalf("updated invite's day:\n%s", ics)
	}
	birthdaysCall(t, mux, parent, "POST", "/api/birthdays/miguel.santos@heliosschool.org/note", map[string]any{"note": "Loves the Giants"}, http.StatusNoContent)
	time.Sleep(50 * time.Millisecond)
	if got := len(birthdaysSent.Wait(t, 4)); got != 4 {
		t.Fatalf("a note sent mail: %d messages", got)
	}
}

func TestReminders(t *testing.T) {
	st, _ := birthdaysServer(t)
	app := birthdaysApp{store: st, queue: queue, mailer: birthdaysSent.Mailgun, base: "https://birthday.example.org"}
	kinds := func(day string) []string {
		out := []string{}
		for _, r := range app.dueReminders(st.Model(), testkit.MustTime(day)) {
			out = append(out, r.sv.Name+":"+r.kind)
		}
		return out
	}
	if got := kinds("2026-09-02"); len(got) != 0 {
		t.Fatalf("the day before, due: %v", got)
	}
	if got := kinds("2026-09-03"); len(got) != 1 || got[0] != "Ruth Amari:ask" {
		t.Fatalf("on the day, due: %v", got)
	}
	if n := app.sendDueReminders(context.Background(), testkit.MustTime("2026-09-03")); n != 1 {
		t.Fatalf("sent %d", n)
	}
	ask := birthdaysSent.Wait(t, 1)[0]
	if ask.To[0] != "mina.park@heliosschool.org" || ask.Subject != "Re: Ask Ruth Amari about their birthday charity" || ask.Headers["In-Reply-To"] == "" {
		t.Fatalf("ask reminder: %+v", ask)
	}
	for _, want := range []string{"mailto:ruth.amari@heliosschool.org?", "cc=hca%40heliosschool.org", "Hi Ruth,", "Mina Park", "/staff/ruth.amari"} {
		if !strings.Contains(ask.Text, want) {
			t.Fatalf("ask reminder lacks %q:\n%s", want, ask.Text)
		}
	}
	if got := kinds("2026-09-03"); len(got) != 0 {
		t.Fatalf("after sending, due: %v", got)
	}
	if got := kinds("2026-09-04"); len(got) != 0 {
		t.Fatalf("the day after, due: %v", got)
	}
	if got := kinds("2026-09-05"); len(got) != 1 || got[0] != "Ruth Amari:late" {
		t.Fatalf("two days on, due: %v", got)
	}
	app.sendDueReminders(context.Background(), testkit.MustTime("2026-09-05"))
	late := birthdaysSent.Wait(t, 2)[1]
	if !strings.Contains(late.Text, "not marked done") || !strings.Contains(late.Text, "mailto:ruth.amari") {
		t.Fatalf("late reminder: %s", late.Text)
	}
	got := kinds("2026-09-09")
	if len(got) != 2 || got[0] != "Bill Ryder:donation" || got[1] != "Ruth Amari:donation" {
		t.Fatalf("before the newsletter, due: %v", got)
	}
	app.sendDueReminders(context.Background(), testkit.MustTime("2026-09-09"))
	donation := birthdaysSent.Wait(t, 4)[2]
	if !strings.Contains(donation.Text, "no need to ask again") || !strings.Contains(donation.Text, "Second Harvest of Silicon Valley") {
		t.Fatalf("donation reminder: %s", donation.Text)
	}
	if got := kinds("2026-09-10"); len(got) != 0 {
		t.Fatalf("the next day, due again: %v", got)
	}
	if n := st.Count(birthdaysAppName, remindersTab, nil); n != 4 {
		t.Fatalf("reminder rows: %d", n)
	}
}

func TestCharityRenameKeepsDonationsAndTheDefault(t *testing.T) {
	st, mux := birthdaysServer(t)
	const old, name = "Second Harvest of Silicon Valley", "Second Harvest"
	donations := st.Count(birthdaysAppName, donationsTab, store.Row{"Charity": secondHarvest})
	if donations == 0 || st.Model().Birthdays.Settings.DefaultCharity != secondHarvest {
		t.Fatal("the sample has no donation or default naming the charity")
	}
	c := st.Model().Birthdays.Charity(secondHarvest)
	birthdaysCall(t, mux, jordan, "POST", "/api/charities/"+secondHarvest+"/edit", map[string]any{"name": name, "donationLink": c.DonationLink, "about": c.About, "ein": c.EIN}, http.StatusNoContent)
	m := st.Model().Birthdays
	if m.Charity(secondHarvest).Name != name || m.charityNamed(old) != nil {
		t.Fatalf("after the rename: %+v", m.Charity(secondHarvest))
	}
	if st.Count(birthdaysAppName, donationsTab, store.Row{"Charity": secondHarvest}) != donations || m.Settings.DefaultCharity != secondHarvest {
		t.Fatal("the donations or the default lost the charity in the rename")
	}
	settings := get(t, mux, parent, "/api/birthday-settings")
	if s := settings.Resources["birthday-settings"][settings.ids(t)[0]]["settings"].(map[string]any); s["defaultCharity"] != secondHarvest {
		t.Fatalf("the settings' default: %v", s)
	}
	queue.Flush()
	_, log, err := sheet.Table(birthdaysAppName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0]["Actor"] != jordan || log[0]["Action"] != "set" || log[0]["Tab"] != charitiesTab || log[0]["Column"] != "Name" || log[0]["Previous"] != old {
		t.Fatalf("logged %v", log)
	}
}

func TestARunOfNewsletterDatesIsOneBatch(t *testing.T) {
	st, mux := birthdaysServer(t)
	run := []birthdaysWrite{}
	for day := testkit.MustTime("2027-08-19"); !day.After(testkit.MustTime("2027-09-30")); day = day.AddDate(0, 0, 7) {
		run = append(run, birthdaysWrite{Method: "POST", Path: "/api/newsletter-dates", Body: map[string]string{"date": day.Format(DateFormat)}})
	}
	birthdaysCall(t, mux, parent, "POST", "/api/act", run, http.StatusForbidden)
	clash := append(slices.Clone(run), birthdaysWrite{Method: "POST", Path: "/api/newsletter-dates", Body: map[string]string{"date": "2027-06-04"}})
	birthdaysCall(t, mux, jordan, "POST", "/api/act", clash, http.StatusBadRequest)
	if len(st.Model().Birthdays.NewsletterDates) != 57 {
		t.Fatal("a refused run added dates")
	}
	rec := birthdaysCall(t, mux, jordan, "POST", "/api/act", run, http.StatusOK)
	var out struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Results) != 7 {
		t.Fatalf("results %s: %v", rec.Body, err)
	}
	m := st.Model().Birthdays
	minted := map[string]bool{}
	for _, r := range out.Results {
		if n := m.NewsletterDate(r.ID); n == nil || minted[r.ID] {
			t.Fatalf("minted newsletter date id %q", r.ID)
		}
		minted[r.ID] = true
	}
	for _, want := range []string{"2027-08-19", "2027-08-26", "2027-09-30"} {
		if m.newsletterOn(want) == nil {
			t.Fatalf("missing %s in %v", want, m.NewsletterDates)
		}
	}
	queue.Flush()
	_, log, err := sheet.Table(birthdaysAppName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 7 {
		t.Fatalf("logged %d rows", len(log))
	}
	birthdaysCall(t, mux, jordan, "POST", "/api/act", run, http.StatusBadRequest)
}

func TestResendingIsAddingAnInvite(t *testing.T) {
	_, mux := birthdaysServer(t)
	if got := get(t, mux, jordan, "/api/birthday-invites").ids(t); len(got) != 3 {
		t.Fatalf("invites %v", got)
	}
	bill, _ := staff(t, mux, jordan, "bill.ryder@heliosschool.org")
	birthdaysCall(t, mux, parent, "POST", "/api/birthday-invites", map[string]any{"birthday": bill["id"]}, http.StatusForbidden)
	resend := []birthdaysWrite{}
	for _, email := range []string{"bill.ryder", "ruth.amari", "alice.fontaine"} {
		b, _ := staff(t, mux, jordan, email)
		resend = append(resend, birthdaysWrite{Method: "POST", Path: "/api/birthday-invites", Body: map[string]any{"birthday": b["id"]}})
	}
	birthdaysCall(t, mux, jordan, "POST", "/api/act", resend, http.StatusOK)
	msgs := birthdaysSent.Wait(t, 3)
	for _, m := range msgs {
		if len(m.Attachments) != 1 || m.Headers["Message-ID"] == "" || strings.Contains(m.Text, "moved from") {
			t.Fatalf("resent invite: %+v", m)
		}
	}
	queue.Flush()
	invites := get(t, mux, jordan, "/api/birthday-invites")
	if got := invites.ids(t); len(got) != 6 {
		t.Fatalf("invites after resending: %v", got)
	}
	for key, inv := range invites.Resources["birthday-invites"] {
		if inv["sentOn"] == nil {
			t.Errorf("invite %s was not sent: %+v", key, inv)
		}
	}
	if _, out := staff(t, mux, jordan, "bill.ryder@heliosschool.org"); len(out.Resources["birthday-invites"]) != 2 {
		t.Errorf("bill's invites: %+v", out.Resources["birthday-invites"])
	}
}

func TestClearingTheFutureNewsletterDates(t *testing.T) {
	st, mux := birthdaysServer(t)
	dates := get(t, mux, jordan, "/api/newsletter-dates")
	clear := []birthdaysWrite{}
	for _, key := range dates.ids(t) {
		if dates.Resources["newsletter-dates"][key]["date"].(string) >= "2026-09-09" {
			clear = append(clear, birthdaysWrite{Method: "DELETE", Path: "/api/newsletter-dates/" + key})
		}
	}
	birthdaysCall(t, mux, parent, "POST", "/api/act", clear, http.StatusForbidden)
	birthdaysCall(t, mux, jordan, "POST", "/api/act", clear, http.StatusOK)
	left := st.Model().Birthdays.NewsletterDates
	if len(left) != 57-len(clear) || len(left) == 0 || left[len(left)-1].Date >= "2026-09-09" {
		t.Fatalf("after clearing: %v", left)
	}
	if kate := st.Model().Birthdays.Birthday("kate.doyle@heliosschool.org"); kate.Override != "" {
		t.Fatalf("an override outlived its cleared date: %+v", kate)
	}
}

func TestJoinTeam(t *testing.T) {
	birthdaysSheet(t)
	if err := sheet.Update(homeAppName, homeVisibilityTab, map[string]string{"App": "birthday"}, map[string]string{"Emails": ""}); err != nil {
		t.Fatal(err)
	}
	st, mux := birthdaysOver(t)
	joined := func() []string {
		return slices.Sorted(slices.Values(st.Model().Home.Visibility["birthday"].Emails))
	}
	birthdaysCall(t, mux, parent, "POST", "/api/birthday-team", nil, http.StatusOK)
	n := 0
	for _, m := range st.Model().Birthdays.Team {
		if m.Email == parent && m.Role == RoleVolunteer {
			n++
		}
	}
	if n != 1 || !slices.Equal(joined(), []string{parent}) {
		t.Fatalf("after robin joined: %d rows, home %v", n, joined())
	}
	birthdaysCall(t, mux, jordan, "POST", "/api/birthday-team", nil, http.StatusOK)
	if !slices.Contains(st.Model().Birthdays.Team, TeamMember{Email: jordan, Role: RoleVolunteer}) || !slices.Equal(joined(), []string{jordan, parent}) {
		t.Fatalf("after jordan joined: %v, home %v", st.Model().Birthdays.Team, joined())
	}
}

func TestStrangerSeesOnlyTheJoinQuestion(t *testing.T) {
	_, mux := birthdaysServer(t)
	stranger := "sam.whitfield@heliosschool.org"
	settings := get(t, mux, stranger, "/api/birthday-settings")
	s := settings.Resources["birthday-settings"][settings.ids(t)[0]]
	if s["me"].(map[string]any)["team"] != false || s["settings"] != nil {
		t.Fatalf("a stranger's settings: %+v", s)
	}
	for _, path := range []string{"/api/birthdays", "/api/charities", "/api/newsletter-dates", "/api/birthday-team", "/api/donations", "/api/birthday-notes", "/api/birthday-invites"} {
		if got := get(t, mux, stranger, path).ids(t); len(got) != 0 {
			t.Errorf("%s for a stranger: %v", path, got)
		}
	}
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/birthdays/dana.hawkins@heliosschool.org/assign"}, {"POST", "/api/birthdays/dana.hawkins@heliosschool.org/note"},
		{"POST", "/api/charities/" + rocketDog + "/edit"}, {"POST", "/api/newsletter-dates/" + issueSep11 + "/share"},
	} {
		birthdaysCall(t, mux, stranger, c.method, c.path, map[string]any{"note": "x", "name": "X"}, http.StatusNotFound)
	}
	birthdaysCall(t, mux, stranger, "POST", "/api/charities", map[string]any{"name": "X"}, http.StatusForbidden)
	birthdaysCall(t, mux, stranger, "POST", "/api/birthday/charity/describe", map[string]any{"name": "X"}, http.StatusForbidden)
	birthdaysCall(t, mux, stranger, "POST", "/api/birthday-team", nil, http.StatusOK)
	if got := get(t, mux, stranger, "/api/birthdays").ids(t); len(got) == 0 {
		t.Fatal("after joining, still nobody to see")
	}
}

func TestNewsletterDates(t *testing.T) {
	st, mux := birthdaysServer(t)
	birthdaysCall(t, mux, parent, "POST", "/api/newsletter-dates", map[string]any{"date": "2027-06-11"}, http.StatusForbidden)
	birthdaysCall(t, mux, parent, "POST", "/api/newsletter-dates/nwd0000000057/move", map[string]any{"date": "2027-06-05"}, http.StatusForbidden)
	birthdaysCall(t, mux, jordan, "POST", "/api/newsletter-dates", map[string]any{"date": "2027-06-04"}, http.StatusBadRequest)
	key := birthdaysCreated(t, birthdaysCall(t, mux, jordan, "POST", "/api/newsletter-dates", map[string]any{"date": "2027-06-11"}, http.StatusOK))
	if added := st.Model().Birthdays.newsletterOn("2027-06-11"); added == nil || added.ID != key {
		t.Fatalf("the added date: %+v", added)
	}
	if got := get(t, mux, parent, "/api/newsletter-dates/2027-06-11").id(t); got != key {
		t.Fatalf("by date: %s", got)
	}
	if tom, _ := staff(t, mux, parent, "tom.grady@heliosschool.org"); tom["newsletterDate"] != "2027-06-11" {
		t.Fatalf("the summer newsletter did not move: %+v", tom)
	}
	birthdaysCall(t, mux, jordan, "POST", "/api/newsletter-dates/"+key+"/move", map[string]any{"date": "2027-06-04"}, http.StatusBadRequest)
	birthdaysCall(t, mux, jordan, "POST", "/api/newsletter-dates/"+unknownID+"/move", map[string]any{"date": "2027-07-02"}, http.StatusNotFound)
	birthdaysCall(t, mux, jordan, "POST", "/api/newsletter-dates/"+key+"/move", map[string]any{"date": "2027-06-18"}, http.StatusNoContent)
	if tom, _ := staff(t, mux, parent, "tom.grady@heliosschool.org"); tom["newsletterDate"] != "2027-06-18" {
		t.Fatalf("the summer newsletter did not follow the move: %+v", tom)
	}
	birthdaysCall(t, mux, jordan, "DELETE", "/api/newsletter-dates/"+key, nil, http.StatusNoContent)
	if len(st.Model().Birthdays.NewsletterDates) != 57 {
		t.Fatal("the date survived removal")
	}
	birthdaysCall(t, mux, jordan, "DELETE", "/api/newsletter-dates/"+key, nil, http.StatusNotFound)
}

func TestMovingANewsletterDateKeepsItsPinnedBirthdays(t *testing.T) {
	_, mux := birthdaysServer(t)
	birthdaysCall(t, mux, jordan, "POST", "/api/newsletter-dates/"+issueOct23+"/move", map[string]any{"date": "2026-10-22"}, http.StatusNoContent)
	if kate, _ := staff(t, mux, parent, "kate.doyle@heliosschool.org"); kate["override"] != issueOct23 || kate["newsletterDate"] != "2026-10-22" || kate["requestBy"] != "2026-10-14" {
		t.Fatalf("the pinned birthday did not keep its issue: %+v", kate)
	}
	queue.Flush()
	_, log, err := sheet.Table(birthdaysAppName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0]["Tab"] != newsletterDatesTab || log[0]["Column"] != "Date" || log[0]["Previous"] != "2026-10-23" {
		t.Fatalf("logged %v", log)
	}
}

func TestPinningABirthday(t *testing.T) {
	_, mux := birthdaysServer(t)
	const tomPath = "/api/birthdays/tom.grady@heliosschool.org/set"
	birthdaysCall(t, mux, parent, "POST", tomPath, map[string]any{"birthday": "06-20", "override": issueSep11}, http.StatusNoContent)
	if tom, _ := staff(t, mux, parent, "tom.grady@heliosschool.org"); tom["override"] != issueSep11 || tom["newsletterDate"] != "2026-09-11" {
		t.Fatalf("after pinning: %+v", tom)
	}
	for _, bad := range []string{"2026-09-11", unknownID} {
		birthdaysCall(t, mux, parent, "POST", tomPath, map[string]any{"birthday": "06-20", "override": bad}, http.StatusBadRequest)
	}
}

func TestDeletingANewsletterDateClearsItsPins(t *testing.T) {
	st, mux := birthdaysServer(t)
	birthdaysCall(t, mux, jordan, "DELETE", "/api/newsletter-dates/"+issueOct23, nil, http.StatusNoContent)
	m := st.Model().Birthdays
	if m.NewsletterDate(issueOct23) != nil || m.newsletterOn("2026-10-23") != nil {
		t.Fatal("the date survived removal")
	}
	if kate, _ := staff(t, mux, parent, "kate.doyle@heliosschool.org"); kate["override"] != nil || kate["newsletterDate"] != "2026-10-09" {
		t.Fatalf("the pin outlived its date: %+v", kate)
	}
	queue.Flush()
	_, rows, err := sheet.Table(birthdaysAppName, birthdaysTab)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row["Newsletter Override"] == issueOct23 {
			t.Fatalf("the sheet kept %v", row)
		}
	}
}

func TestShareIssue(t *testing.T) {
	st, mux := birthdaysServer(t)
	if at := nextExport(testkit.MustTime("2026-09-09")); at.Format("2006-01-02 15:04 Mon") != "2026-09-10 23:59 Thu" {
		t.Fatalf("next export = %v", at)
	}
	if at := nextExport(time.Date(2026, 9, 10, 23, 59, 30, 0, Location)); at.Format(DateFormat) != "2026-09-17" {
		t.Fatalf("the export after one just run = %v", at)
	}
	if issue := weekIssue(st.Model().Birthdays, time.Date(2026, 9, 10, 23, 59, 0, 0, Location)); issue != "2026-09-11" {
		t.Fatalf("the week's issue = %q", issue)
	}
	birthdaysCall(t, mux, jordan, "POST", "/api/newsletter-dates/"+unknownID+"/share", nil, http.StatusNotFound)
	issue := get(t, mux, parent, "/api/newsletter-dates/"+issueSep11)
	if issue.Resources["newsletter-dates"][issueSep11]["can"].(map[string]any)["share"] != true {
		t.Fatalf("an issue with birthdays to copy: %+v", issue)
	}
	a := birthdaysApp{store: st, queue: queue}
	n, err := a.weeklyExport(context.Background(), "2026-09-11")
	if err != nil {
		t.Fatal(err)
	}
	queue.Flush()
	tabs, err := sheet.Tabs(context.Background(), sharedSheet, []string{sharedNewsletterTab}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := tabs[sharedNewsletterTab].Rows
	if n != len(rows) || n == 0 {
		t.Fatalf("copied %d, rows %d", n, len(rows))
	}
	b := st.Model().Birthdays
	for _, row := range rows {
		email := row["Staff Email"]
		d, ok := b.Donation(email, "2026 - 2027")
		if !ok || d.UsedOn != "2026-09-09" || d.UsedBy != exportActor || row["Target Newsletter Date"] != "2026-09-11" || row["Charity Name"] != b.charityName(d.Charity) || row["Charity Link"] == "" || row["Charity Blurb"] == "" {
			t.Errorf("%s: row %v, donation %+v", email, row, d)
		}
	}
	bill := rows[slices.IndexFunc(rows, func(r map[string]string) bool { return r["Staff Email"] == "bill.ryder@heliosschool.org" })]
	if d, _ := b.Donation("bill.ryder@heliosschool.org", "2026 - 2027"); bill["Staff Name"] != "Bill Ryder" || bill["Staff Birthday"] != "2026-09-15" || bill["Contacted On"] != "2026-09-08" || bill["Charity Selected On"] != "" || bill["Charity Name"] != "Second Harvest of Silicon Valley" || d.Charity != secondHarvest || d.RecordedBy != "" {
		t.Errorf("Bill, defaulted: row %v, donation %+v", bill, d)
	}
	if n, err := a.weeklyExport(context.Background(), "2026-09-11"); n != 0 || err != nil {
		t.Fatalf("a second run copied %d: %v", n, err)
	}
	issue = get(t, mux, parent, "/api/newsletter-dates/"+issueSep11)
	if issue.Resources["newsletter-dates"][issueSep11]["can"].(map[string]any)["share"] != false {
		t.Fatalf("an issue already copied: %+v", issue)
	}
	omar, _ := staff(t, mux, jordan, "omar.farouk@heliosschool.org")
	omarIssue := get(t, mux, jordan, "/api/newsletter-dates/"+omar["newsletterDate"].(string)).id(t)
	birthdaysCall(t, mux, parent, "POST", "/api/newsletter-dates/"+omarIssue+"/share", nil, http.StatusNoContent)
	queue.Flush()
	tabs, _ = sheet.Tabs(context.Background(), sharedSheet, []string{sharedNewsletterTab}, nil)
	rows = tabs[sharedNewsletterTab].Rows
	i := slices.IndexFunc(rows, func(r map[string]string) bool { return r["Staff Email"] == "omar.farouk@heliosschool.org" })
	if i < 0 || rows[i]["Preference"] != LevelNoNewsletter || rows[i]["Charity Name"] != "Wikipedia" || rows[i]["Note"] != "Free knowledge for everyone." || rows[i]["Charity Selected On"] != "2026-09-03" || rows[i]["Contacted On"] != "2026-09-02" {
		t.Errorf("Omar's row: %v", rows)
	}
	if d, _ := st.Model().Birthdays.Donation("omar.farouk@heliosschool.org", "2026 - 2027"); d.UsedOn == "" || d.UsedBy != parent || d.Charity != wikipedia || d.RecordedBy != jordan {
		t.Errorf("Omar's donation after: %+v", d)
	}
}
