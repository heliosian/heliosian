package who

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"strings"
	"testing"
	"time"

	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

type noBlobs struct{}

func (noBlobs) Has(string) (bool, error) { return false, nil }

func (noBlobs) Prefetch(context.Context, []string) error { return nil }

var testKey = []byte("test")

func sampleModel(t *testing.T) *Model {
	t.Helper()
	model, err := LoadModel(&data.Dir{Root: "../../sampledata"}, noBlobs{}, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("load sample model: %v", err)
	}
	return model
}

func sampleTables(t *testing.T) store.Tables {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	tables := store.Tables{}
	for _, tab := range spec(nil, nil, nil, func() {}).Tabs {
		app := tab.App
		if app == "" {
			app = appName
		}
		_, rows, err := dir.Table(app, tab.Name)
		if err != nil {
			t.Fatal(err)
		}
		tables[tab.Name] = rows
	}
	return tables
}

func fill(row, cells store.Row) {
	for column, value := range cells {
		if value == "" {
			delete(row, column)
			continue
		}
		row[column] = value
	}
}

func with(tables store.Tables, tab string, match, cells store.Row) store.Tables {
	out := maps.Clone(tables)
	rows := []store.Row{}
	found := false
	for _, row := range tables[tab] {
		matched := true
		for column, value := range match {
			if !strings.EqualFold(row[column], value) {
				matched = false
			}
		}
		if !matched {
			rows = append(rows, row)
			continue
		}
		next := maps.Clone(row)
		fill(next, cells)
		rows = append(rows, next)
		found = true
	}
	if !found {
		row := maps.Clone(match)
		fill(row, cells)
		rows = append(rows, row)
	}
	out[tab] = rows
	return out
}

func withOverride(tables store.Tables, email string, cells store.Row) store.Tables {
	return with(tables, overridesTab, store.Row{"Email": email}, cells)
}

func withFamily(tables store.Tables, key string, cells store.Row) store.Tables {
	return with(tables, familiesTab, store.Row{"Email": key}, cells)
}

func withPhotos(tables store.Tables, email string, refs []photoRef) store.Tables {
	out := maps.Clone(tables)
	rows := []store.Row{}
	for _, row := range tables[photosTab] {
		if !strings.EqualFold(row["Email"], email) {
			rows = append(rows, row)
		}
	}
	for _, ref := range refs {
		row := store.Row{"Email": email, "Photo Name": ref.Name}
		if ref.CropName != "" {
			row["Crop Name"] = ref.CropName
		}
		rows = append(rows, row)
	}
	out[photosTab] = rows
	return out
}

func TestStaffImportExcludesVendors(t *testing.T) {
	if p := sampleModel(t).Person("sasha.pike@heliosschool.org"); p != nil {
		t.Errorf("vendor %s reached the directory", p.Email)
	}
}

func TestStaffWithNoVeracrossEmailComeFromTheMapping(t *testing.T) {
	p := model(t, "luis.ortega@heliosschool.org")
	if !p.IsStaff || p.FullName != "Luis Ortega" || p.JobTitle != "Janitorial" {
		t.Errorf("mapped staff member did not load: %+v", p)
	}
}

func TestNameWithNoEmailExcludesThePerson(t *testing.T) {
	for _, p := range sampleModel(t).People {
		if p.FullName == "Rosa Delgado" {
			t.Errorf("excluded person reached the directory as %s", p.Email)
		}
	}
}

func TestStaffImportSuppliesNameAndJobTitle(t *testing.T) {
	p := model(t, "ruth.amari@heliosschool.org")
	if !p.IsStaff {
		t.Error("imported staff is not marked staff")
	}
	if p.FullName != "Ruth Amari" {
		t.Errorf("full name = %q, want the imported name", p.FullName)
	}
	if p.JobTitle != "Kindergarten Teacher" {
		t.Errorf("job title = %q, want the imported title", p.JobTitle)
	}
	if p.Department != "Classroom Teachers" || p.Classroom != "Hummingbirds" {
		t.Errorf("department %q classroom %q, want the override values", p.Department, p.Classroom)
	}
}

func TestStaffImportTakesBusinessPhone(t *testing.T) {
	if p := model(t, "hank.morrow@heliosschool.org"); p.Phone != "650-555-0142" {
		t.Errorf("phone = %q, want the imported business phone", p.Phone)
	}
}

func TestStaffWhoIsAlsoAParentMerges(t *testing.T) {
	p := model(t, "dana.hawkins@heliosschool.org")
	if !p.IsStaff || !p.IsParent {
		t.Errorf("staff %t parent %t, want both", p.IsStaff, p.IsParent)
	}
	if p.JobTitle != "Art Teacher" {
		t.Errorf("job title = %q, want the imported title", p.JobTitle)
	}
	if p.PreferredName != "Dana" {
		t.Errorf("preferred name = %q, want the household form to survive", p.PreferredName)
	}
	if p.Phone != "" || !p.PhoneMasked {
		t.Errorf("phone = %q masked = %t, want it cleared by preference", p.Phone, p.PhoneMasked)
	}
}

func TestFamilyThatNeverSubmittedIsAbsent(t *testing.T) {
	m := sampleModel(t)
	for _, email := range []string{"april.baxter@heliosschool.org", "leo.baxter@heliosschool.org"} {
		if p := m.Person(email); p != nil {
			t.Errorf("%s reached the directory with no consent form submission", p.Email)
		}
	}
	if _, ok := m.Families[familyID(testKey, "april.baxter@heliosschool.org")]; ok {
		t.Error("a family with no submission still has a family record")
	}
}

func TestStaffWhoNeverSubmittedStayListed(t *testing.T) {
	if p := model(t, "ruth.amari@heliosschool.org"); !p.IsStaff {
		t.Errorf("staff %t, want a staff member with no submission to stay listed", p.IsStaff)
	}
}

func TestConsentOptOutRemovesStaff(t *testing.T) {
	if p := sampleModel(t).Person("grace.kim@heliosschool.org"); p != nil {
		t.Errorf("%s opted out on the form and is still in the directory", p.Email)
	}
}

func TestOneHouseholdsAnswerCoversTheOther(t *testing.T) {
	tables := sampleTables(t)
	kept := []store.Row{}
	for _, row := range tables[preferencesTab] {
		if row[preferenceEmail] != "rohan.chandra@heliosschool.org" {
			kept = append(kept, row)
		}
	}
	tables[preferencesTab] = kept
	m, err := BuildModel(context.Background(), tables, noBlobs{}, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("build model with one household's submission dropped: %v", err)
	}
	for _, email := range []string{"dev.chandra@heliosschool.org", "rohan.chandra@heliosschool.org", "asha.chandra@heliosschool.org"} {
		if p := m.Person(email); p == nil {
			t.Errorf("%s is not in the directory, though one of the kid's households opted in", email)
		}
	}
	family := m.Families[familyID(testKey, "rohan.chandra@heliosschool.org")]
	if family.AddressMasked || family.Address == "" || family.PhoneMasked {
		t.Errorf("the household that never submitted does not carry the other's permissions: %+v", family)
	}
}

func TestLatestAnswerInAFamilySpeaksForEveryHousehold(t *testing.T) {
	m := sampleModel(t)
	for _, key := range []string{"asha.chandra@heliosschool.org", "rohan.chandra@heliosschool.org"} {
		family := m.Families[familyID(testKey, key)]
		if !family.AddressMasked || family.Address != "" || family.PhoneMasked {
			t.Errorf("family %s = %+v, want the later answer's grants, phone only, on both households", key, family)
		}
	}
}

func TestWebsiteImportSuppliesTheBioAndNotTheTitle(t *testing.T) {
	p := model(t, "bill.ryder@heliosschool.org")
	if p.Facts != "Bill runs the front office and knows where everything in the building is." {
		t.Errorf("facts = %q, want the website bio", p.Facts)
	}
	if p.JobTitle != "Office Manager" {
		t.Errorf("job title = %q, want the imported Veracross title", p.JobTitle)
	}
}

func TestOverriddenFactsBeatTheWebsiteBio(t *testing.T) {
	p := model(t, "ruth.amari@heliosschool.org")
	if p.Facts != "Twelve years teaching kindergarten, keeper of the class worm farm." {
		t.Errorf("facts = %q, want the Overrides value", p.Facts)
	}
}

func TestClearedFactsAreNotRefilledFromTheWebsite(t *testing.T) {
	email := "bill.ryder@heliosschool.org"
	m, err := BuildModel(context.Background(), withOverride(sampleTables(t), email, store.Row{"Facts": "-"}), noBlobs{}, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("build model with the facts cleared: %v", err)
	}
	if got := m.Person(email).Facts; got != "" {
		t.Errorf("facts = %q, want the cleared value to hold", got)
	}
}

func TestAddedStaffAreOutOfTheWebsiteLayersReach(t *testing.T) {
	p := model(t, "noa.adler@heliosschool.org")
	if p.Facts != "" {
		t.Errorf("facts = %q, want the website layer to have missed an added-only person", p.Facts)
	}
}

func TestWebsiteEntryMatchingNobodyIsSkipped(t *testing.T) {
	if p := sampleModel(t).Person("sasha.pike@heliosschool.org"); p != nil {
		t.Errorf("%s is on the staff page but not in the directory, and should stay out", p.Email)
	}
}

func TestWebsiteEntryWithNoEmailMatchesByName(t *testing.T) {
	p := model(t, "luis.ortega@heliosschool.org")
	if p.Facts == "" {
		t.Error("a website entry with no address did not reach its person by name")
	}
}

func TestWebsiteEntryWithNoEmailMatchesVeracrossStaffByName(t *testing.T) {
	p := model(t, "hana.ito@heliosschool.org")
	if p.Facts != "Hana came to teaching from marine research and still takes her class tide-pooling every spring." {
		t.Errorf("facts = %q, want the bio matched by name against the staff import", p.Facts)
	}
}

func TestWebsiteEntryMatchingTwoStaffByNameIsFatal(t *testing.T) {
	tables := sampleTables(t)
	tables[staffTab] = append(append([]store.Row{}, tables[staffTab]...), store.Row{
		"person_full_name": "Hana Ito", "person_email": "hana.ito2@heliosschool.org",
		"person_classifications": `{"faculty_type":"Specialist"}`,
	})
	if _, err := BuildModel(context.Background(), tables, noBlobs{}, noBlobs{}, testKey); err == nil {
		t.Error("a website entry with no address whose name matches two staff should fail the load")
	}
}

func TestWebsiteEntryUnderAnAliasMatchesByEmail(t *testing.T) {
	p := model(t, "hank.morrow@heliosschool.org")
	if p.Facts != "Hank has kept the boilers running through nine winters and knows every valve by name." {
		t.Errorf("facts = %q, want the bio published under the alias", p.Facts)
	}
	if p := sampleModel(t).Person("facilities@heliosschool.org"); p != nil {
		t.Errorf("the alias %s reached the directory as a person", p.Email)
	}
}

func TestSignInUnderAnAliasResolvesToThePerson(t *testing.T) {
	m := sampleModel(t)
	if got := m.Resolve("facilities@heliosschool.org"); got != "hank.morrow@heliosschool.org" {
		t.Errorf("resolved %q, want the address the directory keys the person by", got)
	}
	if got := m.Resolve("ruth.amari@heliosschool.org"); got != "ruth.amari@heliosschool.org" {
		t.Errorf("resolved %q, want an address that is nobody's alias left alone", got)
	}
	if !m.Member(m.Resolve("facilities@heliosschool.org")) {
		t.Error("an alias sign-in resolved to somebody the directory does not list")
	}
}

func TestAliasMatchingNothingIsFatal(t *testing.T) {
	tables := sampleTables(t)
	tables[AliasesTable] = append(append([]store.Row{}, tables[AliasesTable]...), store.Row{
		AliasColumn: "nobody@heliosschool.org", AliasEmailColumn: "ruth.amari@heliosschool.org",
	})
	if _, err := BuildModel(context.Background(), tables, noBlobs{}, noBlobs{}, testKey); err == nil {
		t.Error("an alias matching no import row should fail the load")
	}
}

func TestFamilyFieldsComeFromTheFamiliesTab(t *testing.T) {
	m := sampleModel(t)
	keys := m.FamilyKeysOf("marco.torres@heliosschool.org")
	if len(keys) != 1 || keys[0] != familyID(testKey, "elena.torres@heliosschool.org") {
		t.Fatalf("family keys = %v, want the masked alphabetically first adult email", keys)
	}
	family := m.Families[keys[0]]
	if family.Key != keys[0] || family.email != "elena.torres@heliosschool.org" {
		t.Fatalf("family key %q, email %q, want the masked key and the first adult's email", family.Key, family.email)
	}
	if family.PhotoUpdated != "2026-08-20" {
		t.Errorf("family photo updated = %q, want the Families tab value", family.PhotoUpdated)
	}
	if family.Phone != "650-555-0141" {
		t.Errorf("family phone = %q, want the imported household phone", family.Phone)
	}
}

func TestClassroomPathIsThePagesSlug(t *testing.T) {
	for name, want := range map[string]string{"Condors": "/classrooms/condors", "Blue Jays": "/classrooms/blue-jays"} {
		if got := ClassroomPath(name); got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
}

func TestFamilyNameFoldsSurnamesIntoAHyphenatedOne(t *testing.T) {
	people := map[string]*Person{
		"ada@x.org":   {FullName: "Ada Mager-Ridgeway"},
		"mira@x.org":  {FullName: "Mira Mager-Ridgeway"},
		"lena@x.org":  {FullName: "Lena Mager"},
		"tom@x.org":   {FullName: "Tom Ridgeway"},
		"avni@x.org":  {FullName: "Avni Bhat"},
		"ravi@x.org":  {FullName: "Ravi Bhat"},
		"asha@x.org":  {FullName: "Asha Shenoy"},
		"kim@x.org":   {FullName: "Kim Lee"},
		"sam@x.org":   {FullName: "Sam LEE"},
		"noah@x.org":  {FullName: "Noah Park"},
		"parks@x.org": {FullName: "Jo Park-Hill"},
		"hill@x.org":  {FullName: "Al Hillman"},
	}
	for _, c := range []struct {
		kids, adults []string
		short, full  string
	}{
		{[]string{"ada@x.org", "mira@x.org"}, []string{"lena@x.org", "tom@x.org"}, "Mager-Ridgeway", "Mager-Ridgeway Family"},
		{[]string{"avni@x.org"}, []string{"ravi@x.org", "asha@x.org"}, "Bhat & Shenoy", "Bhat & Shenoy Family"},
		{[]string{"kim@x.org"}, []string{"sam@x.org"}, "Lee", "Lee Family"},
		{[]string{"noah@x.org"}, []string{"parks@x.org", "hill@x.org"}, "Park-Hill & Hillman", "Park-Hill & Hillman Family"},
		{nil, []string{"nobody@x.org"}, "", ""},
	} {
		short, full := familyNameFor(Family{KidEmails: c.kids, AdultEmails: c.adults}, people)
		if short != c.short || full != c.full {
			t.Errorf("%v %v: %q %q, want %q %q", c.kids, c.adults, short, full, c.short, c.full)
		}
	}
}

func TestTwoHouseholdKidBelongsToBothFamilies(t *testing.T) {
	m := sampleModel(t)
	keys := m.FamilyKeysOf("dev.chandra@heliosschool.org")
	want := []string{familyID(testKey, "asha.chandra@heliosschool.org"), familyID(testKey, "rohan.chandra@heliosschool.org")}
	if len(keys) != 2 || keys[0] != want[0] || keys[1] != want[1] {
		t.Fatalf("family keys = %v, want both households in key email order %v", keys, want)
	}
	for _, key := range keys {
		found := false
		for _, kid := range m.Families[key].KidEmails {
			if kid == "dev.chandra@heliosschool.org" {
				found = true
			}
		}
		if !found {
			t.Errorf("family %s does not list the kid", key)
		}
	}
}

func TestFamiliesRowKeyedByTheWrongParentIsFatal(t *testing.T) {
	misKeyed := withFamily(sampleTables(t), "marco.torres@heliosschool.org", store.Row{"Family Phone": "650-555-0000"})
	if _, err := BuildModel(context.Background(), misKeyed, noBlobs{}, noBlobs{}, testKey); err == nil {
		t.Error("a Families row keyed by the non-first parent should fail the load")
	}
}

func TestWithheldFirstAdultIsNowhereInTheModel(t *testing.T) {
	tables := sampleTables(t)
	silent := maps.Clone(tables)
	silent[preferencesTab] = nil
	for _, row := range tables[preferencesTab] {
		if row[preferenceEmail] != "dana.hawkins@heliosschool.org" {
			silent[preferencesTab] = append(silent[preferencesTab], row)
		}
	}
	for _, c := range []struct {
		name, first, partner string
		tables               store.Tables
	}{
		{"opted out", "elena.torres@heliosschool.org", "marco.torres@heliosschool.org",
			withOverride(tables, "elena.torres@heliosschool.org", store.Row{"Opted Out": "TRUE"})},
		{"silent staff partner", "colin.quinn@heliosschool.org", "dana.hawkins@heliosschool.org", silent},
	} {
		m, err := BuildModel(context.Background(), c.tables, noBlobs{}, noBlobs{}, testKey)
		if err != nil {
			t.Fatalf("%s: build model: %v", c.name, err)
		}
		if m.Person(c.first) != nil {
			t.Fatalf("%s: %s is still in the directory", c.name, c.first)
		}
		family, ok := m.Families[familyID(testKey, c.first)]
		if !ok || len(family.AdultEmails) != 1 || family.AdultEmails[0] != c.partner {
			t.Fatalf("%s: family = %+v, want it kept under the first adult's masked key with only the partner listed", c.name, family)
		}
		encoded, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("%s: encode model: %v", c.name, err)
		}
		if bytes.Contains(encoded, []byte(c.first)) {
			t.Errorf("%s: the model every member fetches names the withheld adult %s", c.name, c.first)
		}
	}
}

func TestStaffNotInVeracrossStillLoad(t *testing.T) {
	p := model(t, "noa.adler@heliosschool.org")
	if !p.IsStaff || p.FullName != "Noa Adler" || p.JobTitle != "Music Teacher" {
		t.Errorf("added staff row did not load: %+v", p)
	}
}

func TestClearingLegacyPrimaryPhotoOverrideSucceeds(t *testing.T) {
	email := "ruth.amari@heliosschool.org"
	withPrimary := withOverride(sampleTables(t), email, store.Row{"Primary Photo": "somephoto.jpg"})
	if _, err := BuildModel(context.Background(), withPrimary, noBlobs{}, noBlobs{}, testKey); err != nil {
		t.Fatalf("seed a legacy Primary Photo override: %v", err)
	}
	if _, err := BuildModel(context.Background(), withOverride(withPrimary, email, store.Row{"Primary Photo": ""}), noBlobs{}, noBlobs{}, testKey); err != nil {
		t.Errorf("clearing Primary Photo with an empty string should succeed, got: %v", err)
	}
	if _, err := BuildModel(context.Background(), withOverride(withPrimary, email, store.Row{"Primary Photo": "-"}), noBlobs{}, noBlobs{}, testKey); err == nil {
		t.Errorf("clearing Primary Photo with \"-\" should still fail the useless-override check, documenting why \"\" is required")
	}
}

func TestClearingPronounsSucceeds(t *testing.T) {
	email := "ruth.amari@heliosschool.org"
	withPronouns := withOverride(sampleTables(t), email, store.Row{"Pronouns": "she/her"})
	if _, err := BuildModel(context.Background(), withPronouns, noBlobs{}, noBlobs{}, testKey); err != nil {
		t.Fatalf("seed pronouns: %v", err)
	}
	if _, err := BuildModel(context.Background(), withOverride(withPronouns, email, store.Row{"Pronouns": ""}), noBlobs{}, noBlobs{}, testKey); err != nil {
		t.Errorf("clearing Pronouns with an empty string should succeed, got: %v", err)
	}
	if _, err := BuildModel(context.Background(), withOverride(withPronouns, email, store.Row{"Pronouns": "-"}), noBlobs{}, noBlobs{}, testKey); err == nil {
		t.Errorf("clearing Pronouns with \"-\" should still fail the useless-override check, documenting why \"\" is required")
	}
}

func TestClearingFamilyPhotoCaptionSucceeds(t *testing.T) {
	key := "carmen.alvarez@heliosschool.org"
	withCaption := withFamily(sampleTables(t), key, store.Row{"Family Photo Caption": "Carmen at the beach."})
	m, err := BuildModel(context.Background(), withCaption, noBlobs{}, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("seed a family photo caption: %v", err)
	}
	if m.Families[familyID(testKey, key)].PhotoCaption != "Carmen at the beach." {
		t.Fatalf("caption did not seed correctly: %+v", m.Families[familyID(testKey, key)])
	}
	cleared, err := BuildModel(context.Background(), withFamily(withCaption, key, store.Row{"Family Photo Caption": "-"}), noBlobs{}, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("clearing Family Photo Caption with \"-\" should succeed, got: %v", err)
	}
	if got := cleared.Families[familyID(testKey, key)].PhotoCaption; got != "" {
		t.Errorf("caption = %q after clearing with \"-\", want empty", got)
	}
}

func TestClearingPronunciationSucceeds(t *testing.T) {
	email := "ruth.amari@heliosschool.org"
	blobs := fakeBlobs{"pronunciation/somefile.webm": true}
	withPronunciation := withOverride(sampleTables(t), email, store.Row{"Pronunciation": "somefile.webm"})
	seeded, err := BuildModel(context.Background(), withPronunciation, blobs, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("seed a pronunciation: %v", err)
	}
	if got := seeded.Person(email).pronunciation; got != "somefile.webm" {
		t.Fatalf("pronunciation did not seed correctly: %q", got)
	}
	cleared, err := BuildModel(context.Background(), withOverride(withPronunciation, email, store.Row{"Pronunciation": ""}), blobs, noBlobs{}, testKey)
	if err != nil {
		t.Errorf("clearing Pronunciation with an empty string should succeed, got: %v", err)
	} else if got := cleared.Person(email).pronunciation; got != "" {
		t.Errorf("pronunciation = %q after clearing with \"\", want empty", got)
	}
	if _, err := BuildModel(context.Background(), withOverride(withPronunciation, email, store.Row{"Pronunciation": "-"}), noBlobs{}, noBlobs{}, testKey); err == nil {
		t.Errorf("clearing Pronunciation with \"-\" should still fail the useless-override check, documenting why \"\" is required")
	}
}

func TestClearingFamilyPronunciationSucceeds(t *testing.T) {
	key := "carmen.alvarez@heliosschool.org"
	blobs := fakeBlobs{"pronunciation/somefile.webm": true}
	withPronunciation := withFamily(sampleTables(t), key, store.Row{"Family Pronunciation": "somefile.webm"})
	seeded, err := BuildModel(context.Background(), withPronunciation, blobs, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("seed a family pronunciation: %v", err)
	}
	if got := seeded.Families[familyID(testKey, key)].pronunciation; got != "somefile.webm" {
		t.Fatalf("family pronunciation did not seed correctly: %q", got)
	}
	cleared, err := BuildModel(context.Background(), withFamily(withPronunciation, key, store.Row{"Family Pronunciation": ""}), blobs, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("clearing Family Pronunciation with an empty string should succeed, got: %v", err)
	}
	if got := cleared.Families[familyID(testKey, key)].pronunciation; got != "" {
		t.Errorf("family pronunciation = %q after clearing with \"\", want empty", got)
	}
}

type fakeBlobs map[string]bool

func (f fakeBlobs) Has(key string) (bool, error) { return f[key], nil }

func (fakeBlobs) Prefetch(context.Context, []string) error { return nil }

func TestPhotoCropResolvesOverOriginal(t *testing.T) {
	email := "elena.torres@heliosschool.org"
	withCrop := withPhotos(sampleTables(t), email, []photoRef{{Name: "orig.jpg", CropName: "crop.jpg"}})
	blobs := fakeBlobs{"photos/orig.jpg": true, "photos/crop.jpg": true}
	m, err := BuildModel(context.Background(), withCrop, blobs, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("build model with a crop: %v", err)
	}
	p := m.Person(email)
	if len(p.Photos) != 1 {
		t.Fatalf("photos = %+v, want exactly one", p.Photos)
	}
	photo := p.Photos[0]
	if photo.URL != "/photos/crop.jpg" {
		t.Errorf("URL = %q, want the crop", photo.URL)
	}
	if photo.OriginalURL != "/photos/orig.jpg" {
		t.Errorf("OriginalURL = %q, want the original regardless of the crop", photo.OriginalURL)
	}
}

func TestMissingCropFallsBackToOriginal(t *testing.T) {
	email := "elena.torres@heliosschool.org"
	withCrop := withPhotos(sampleTables(t), email, []photoRef{{Name: "orig.jpg", CropName: "missing-crop.jpg"}})
	blobs := fakeBlobs{"photos/orig.jpg": true}
	m, err := BuildModel(context.Background(), withCrop, blobs, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("a missing crop should not fail the model load: %v", err)
	}
	p := m.Person(email)
	if len(p.Photos) != 1 || p.Photos[0].URL != "/photos/orig.jpg" {
		t.Errorf("photos = %+v, want URL to fall back to the original", p.Photos)
	}
}

func TestPhotoOrderIsNotRowOrder(t *testing.T) {
	email := "elena.torres@heliosschool.org"
	tables := sampleTables(t)
	tables[photosTab] = []store.Row{
		{"Email": email, "Photo Name": "late.jpg"},
		{"Email": email, "Photo Name": "second.jpg", store.OrderColumn: "m"},
		{"Email": email, "Photo Name": "first.jpg", store.OrderColumn: "b"},
	}
	m, err := BuildModel(context.Background(), tables, allBlobs{}, noBlobs{}, testKey)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, photo := range m.Person(email).Photos {
		got = append(got, photo.Name)
	}
	if strings.Join(got, ",") != "first.jpg,second.jpg,late.jpg" {
		t.Errorf("photos %v, want keyed ones in key order and the blank one last", got)
	}
	tables[photosTab][0][store.OrderColumn] = "B"
	if _, err := BuildModel(context.Background(), tables, allBlobs{}, noBlobs{}, testKey); err == nil {
		t.Error("a photo order that is not a sort key loaded")
	}
}

func TestPersonWithNoPhotosShowsNoFamilyPhoto(t *testing.T) {
	key := "elena.torres@heliosschool.org"
	withFamilyPhoto := withFamily(sampleTables(t), key, store.Row{"Family Photo": "family.jpg"})
	blobs := fakeBlobs{"photos/family.jpg": true}
	m, err := BuildModel(context.Background(), withFamilyPhoto, blobs, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("build model with a family photo: %v", err)
	}
	if got := m.Families[familyID(testKey, key)].PhotoURL; got != "/photos/family.jpg" {
		t.Fatalf("family photo did not seed correctly: %q", got)
	}
	p := m.Person("mia.torres@heliosschool.org")
	if p == nil || len(p.Photos) != 0 {
		t.Fatalf("test assumes mia.torres exists with no photos of her own: %+v", p)
	}
	if p.PhotoURL != "" {
		t.Errorf("PhotoURL = %q, want empty so the client shows the initials placeholder", p.PhotoURL)
	}
}

func TestPersonWithNoPronunciationFallsBackToFamilyPronunciation(t *testing.T) {
	key := "elena.torres@heliosschool.org"
	withFamilyPronunciation := withFamily(sampleTables(t), key, store.Row{"Family Pronunciation": "family.webm"})
	blobs := fakeBlobs{"pronunciation/family.webm": true}
	m, err := BuildModel(context.Background(), withFamilyPronunciation, blobs, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("build model with a family pronunciation: %v", err)
	}
	p := m.Person("mia.torres@heliosschool.org")
	if p == nil {
		t.Fatal("mia.torres is not in the model")
	}
	if p.HasOwnPronunciation {
		t.Fatalf("test assumes mia.torres has no pronunciation of her own, got HasOwnPronunciation=true")
	}
	if p.PronunciationURL != "/pronunciation/family.webm" {
		t.Errorf("PronunciationURL = %q, want it to fall back to the family pronunciation", p.PronunciationURL)
	}
}

func TestOwnPronunciationOverridesFamilyFallback(t *testing.T) {
	withBoth := withOverride(
		withFamily(sampleTables(t), "elena.torres@heliosschool.org", store.Row{"Family Pronunciation": "family.webm"}),
		"mia.torres@heliosschool.org", store.Row{"Pronunciation": "mia.webm"})
	blobs := fakeBlobs{"pronunciation/family.webm": true, "pronunciation/mia.webm": true}
	m, err := BuildModel(context.Background(), withBoth, blobs, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("build model with both a personal and family pronunciation: %v", err)
	}
	p := m.Person("mia.torres@heliosschool.org")
	if !p.HasOwnPronunciation {
		t.Errorf("HasOwnPronunciation = false, want true since mia.torres has her own recording")
	}
	if p.PronunciationURL != "/pronunciation/mia.webm" {
		t.Errorf("PronunciationURL = %q, want mia's own recording to take priority over the family's", p.PronunciationURL)
	}
}

func model(t *testing.T, email string) *Person {
	t.Helper()
	p := sampleModel(t).Person(email)
	if p == nil {
		t.Fatalf("%s is not in the model", email)
	}
	return p
}

type blobsWith map[string]bool

func (b blobsWith) Has(name string) (bool, error) { return b[name], nil }

func (blobsWith) Prefetch(context.Context, []string) error { return nil }

func TestHeroPhotoPrefersOwnPhotoThenFallsBackToTheFamily(t *testing.T) {
	const email = "jordan.whitfield@heliosschool.org"
	sample := sampleModel(t)
	key := sample.FamilyKeysOf(email)
	if len(key) == 0 {
		t.Fatalf("%s has no family in the sample", email)
	}
	blobs := blobsWith{"photos/own.jpg": true, "photos/fam.jpg": true}

	family := withFamily(sampleTables(t), sample.Families[key[0]].email, store.Row{"Family Photo": "fam.jpg"})
	m, err := BuildModel(context.Background(), family, blobs, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("build model with a family photo: %v", err)
	}
	if got := m.HeroPhoto(email); got != "/photos/fam.jpg" {
		t.Errorf("with only a family photo: got %q, want the family's", got)
	}

	own := withPhotos(family, email, []photoRef{{Name: "own.jpg"}})
	m, err = BuildModel(context.Background(), own, blobs, noBlobs{}, testKey)
	if err != nil {
		t.Fatalf("build model with an own photo: %v", err)
	}
	if got := m.HeroPhoto(email); got != "/photos/own.jpg" {
		t.Errorf("with an own photo: got %q, want their own over the family's", got)
	}
}

func TestHeroPhotoIsEmptyForNonMembersAndForNoPhoto(t *testing.T) {
	m := sampleModel(t)
	if got := m.HeroPhoto("nobody@example.org"); got != "" {
		t.Errorf("non-member: got %q, want empty", got)
	}
	if got := m.HeroPhoto("jordan.whitfield@heliosschool.org"); got != "" {
		t.Errorf("member with no photo anywhere: got %q, want empty", got)
	}
}

func TestAlertsMatchTheDirectoryPage(t *testing.T) {
	m := sampleModel(t)
	years := config.StaleYears{Photo: 0.75, Facts: 0.6, FamilyPhoto: 1.5}
	got := m.Alerts("jordan.whitfield@heliosschool.org", years, time.Now())
	if len(got.Stale) != 4 || len(got.Privacy) != 1 {
		t.Errorf("alerts for the sample parent = %+v, want 4 stale and one detail mismatched", got)
	}
	if got := m.Alerts("nobody@example.org", years, time.Now()); len(got.Stale) != 0 || len(got.Privacy) != 0 {
		t.Errorf("alerts for a stranger = %+v, want none", got)
	}
}
