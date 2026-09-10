package who

import (
	"testing"

	"heliosian/internal/data"
)

type noBlobs struct{}

func (noBlobs) Has(string) bool { return false }

func sampleModel(t *testing.T) *Model {
	t.Helper()
	model, err := LoadModel(&data.Dir{Root: "../../sampledata"}, noBlobs{}, noBlobs{})
	if err != nil {
		t.Fatalf("load sample model: %v", err)
	}
	return model
}

func TestStaffImportExcludesVendors(t *testing.T) {
	if p := sampleModel(t).Person("sasha.pike@heliosschool.org"); p != nil {
		t.Errorf("vendor %s reached the directory", p.Email)
	}
}

// Veracross has no address for several real staff. The Name to Email tab covers them
// the same way it covers students, so they are in the directory rather than dropped.
func TestStaffWithNoVeracrossEmailComeFromTheMapping(t *testing.T) {
	p := model(t, "luis.ortega@heliosschool.org")
	if !p.IsStaff || p.FullName != "Luis Ortega" || p.JobTitle != "Janitorial" {
		t.Errorf("mapped staff member did not load: %+v", p)
	}
}

// A name with no address is how somebody records that a person Veracross carries is
// deliberately not in the directory, as opposed to nobody having decided yet.
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
	// Overrides keeps what the export cannot know.
	if p.Department != "Classroom Teachers" || p.Classroom != "Hummingbirds" {
		t.Errorf("department %q classroom %q, want the override values", p.Department, p.Classroom)
	}
}

func TestStaffImportTakesBusinessPhone(t *testing.T) {
	if p := model(t, "hank.morrow@heliosschool.org"); p.Phone != "650-555-0142" {
		t.Errorf("phone = %q, want the imported business phone", p.Phone)
	}
}

// A staff member who is also a parent arrives from both imports. The household copy of
// the name carries a redundant parenthetical the staff export omits, which must merge
// rather than read as a conflict.
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
	// Her family shared neither field on the consent form, which masks the household
	// phone. The staff merge must not resurrect it by filling from the export.
	if p.Phone != "" || !p.PhoneMasked {
		t.Errorf("phone = %q masked = %t, want it cleared by preference", p.Phone, p.PhoneMasked)
	}
}

// The consent form is what puts a family in the directory at all: a household that
// never answered it is gone, not listed with its fields masked.
func TestFamilyThatNeverSubmittedIsAbsent(t *testing.T) {
	m := sampleModel(t)
	for _, email := range []string{"april.baxter@heliosschool.org", "leo.baxter@heliosschool.org"} {
		if p := m.Person(email); p != nil {
			t.Errorf("%s reached the directory with no consent form submission", p.Email)
		}
	}
	if _, ok := m.Families["april.baxter@heliosschool.org"]; ok {
		t.Error("a family with no submission still has a family record")
	}
}

// Staff reach the family consent form only by being a parent too, so silence leaves
// them listed - the exemption covers non-submission and nothing else.
func TestStaffWhoNeverSubmittedStayListed(t *testing.T) {
	if p := model(t, "ruth.amari@heliosschool.org"); !p.IsStaff {
		t.Errorf("staff %t, want a staff member with no submission to stay listed", p.IsStaff)
	}
}

// An opt-out is an answer rather than silence, so it removes a staff member too.
func TestConsentOptOutRemovesStaff(t *testing.T) {
	if p := sampleModel(t).Person("grace.kim@heliosschool.org"); p != nil {
		t.Errorf("%s opted out on the form and is still in the directory", p.Email)
	}
}

// Where a two-household kid's parents disagree the stricter answer holds, and a
// household that never answered is one of those answers.
func TestKidLosesAHouseholdThatNeverSubmitted(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	kept := make([]map[string]string, 0, len(tables.Preferences))
	for _, row := range tables.Preferences {
		if row[preferenceEmail] != "rohan.chandra@heliosschool.org" {
			kept = append(kept, row)
		}
	}
	next := *tables
	next.Preferences = kept
	// Dev is the only student in his crew, which the sample cannot lose while a teacher
	// is still assigned to it.
	m, err := BuildModel(next.withOverride("tom.grady@heliosschool.org", map[string]string{"Crew": ""}), noBlobs{}, noBlobs{})
	if err != nil {
		t.Fatalf("build model with one household's submission dropped: %v", err)
	}
	if p := m.Person("dev.chandra@heliosschool.org"); p != nil {
		t.Error("a kid whose other household never submitted is still in the directory")
	}
	if p := m.Person("rohan.chandra@heliosschool.org"); p != nil {
		t.Error("the parent who never submitted is still in the directory")
	}
	if p := m.Person("asha.chandra@heliosschool.org"); p == nil {
		t.Error("the household that did submit lost its own adult")
	}
}

// The school's staff page is the only source of a bio, and it fills a title only
// where Veracross has none.
func TestWebsiteImportSuppliesTheBioAndNotTheTitle(t *testing.T) {
	p := model(t, "bill.ryder@heliosschool.org")
	if p.Facts != "Bill runs the front office and knows where everything in the building is." {
		t.Errorf("facts = %q, want the website bio", p.Facts)
	}
	if p.JobTitle != "Office Manager" {
		t.Errorf("job title = %q, want the imported Veracross title", p.JobTitle)
	}
}

// Overrides outrank the website like they outrank Veracross, which is why the layer
// runs off the override row rather than the value it resolved to.
func TestOverriddenFactsBeatTheWebsiteBio(t *testing.T) {
	p := model(t, "ruth.amari@heliosschool.org")
	if p.Facts != "Twelve years teaching kindergarten, keeper of the class worm farm." {
		t.Errorf("facts = %q, want the Overrides value", p.Facts)
	}
}

// An override that clears a field with "-" has to stay cleared: the website filling
// it back in would quietly undo a deliberate removal.
func TestClearedFactsAreNotRefilledFromTheWebsite(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	email := "bill.ryder@heliosschool.org"
	m, err := BuildModel(tables.withOverride(email, map[string]string{"Facts": "-"}), noBlobs{}, noBlobs{})
	if err != nil {
		t.Fatalf("build model with the facts cleared: %v", err)
	}
	if got := m.Person(email).Facts; got != "" {
		t.Errorf("facts = %q, want the cleared value to hold", got)
	}
}

// Staff added by hand don't exist until applyOverrides, which runs after the website
// layer, so the page reaches nobody Veracross doesn't carry. Reaching them means
// creating added people before the layer rather than moving the layer.
func TestAddedStaffAreOutOfTheWebsiteLayersReach(t *testing.T) {
	p := model(t, "noa.adler@heliosschool.org")
	if p.Facts != "" {
		t.Errorf("facts = %q, want the website layer to have missed an added-only person", p.Facts)
	}
}

// The website lists people the directory drops - vendors, and anyone who has left.
func TestWebsiteEntryMatchingNobodyIsSkipped(t *testing.T) {
	if p := sampleModel(t).Person("sasha.pike@heliosschool.org"); p != nil {
		t.Errorf("%s is on the staff page but not in the directory, and should stay out", p.Email)
	}
}

// A staff member the site publishes no address for is matched by name, the same way
// the Veracross imports reach the people it has no address for.
func TestWebsiteEntryWithNoEmailMatchesByName(t *testing.T) {
	p := model(t, "luis.ortega@heliosschool.org")
	if p.Facts == "" {
		t.Error("a website entry with no address did not reach its person by name")
	}
}

// A staff member the site publishes no address for, but Veracross has one for, is
// reached by name through the staff import itself: Name to Email cannot carry them,
// since it refuses to restate an address Veracross exports.
func TestWebsiteEntryWithNoEmailMatchesVeracrossStaffByName(t *testing.T) {
	p := model(t, "hana.ito@heliosschool.org")
	if p.Facts != "Hana came to teaching from marine research and still takes her class tide-pooling every spring." {
		t.Errorf("facts = %q, want the bio matched by name against the staff import", p.Facts)
	}
}

// A name two staff share is refused rather than guessed at.
func TestWebsiteEntryMatchingTwoStaffByNameIsFatal(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	next := *tables
	next.Staff = append(append([]map[string]string{}, tables.Staff...), map[string]string{
		"person_full_name": "Hana Ito", "person_email": "hana.ito2@heliosschool.org",
		"person_classifications": `{"faculty_type":"Specialist"}`,
	})
	if _, err := BuildModel(&next, noBlobs{}, noBlobs{}); err == nil {
		t.Error("a website entry with no address whose name matches two staff should fail the load")
	}
}

// A staff member the site publishes under a second address is reached through the
// Email Aliases tab, which resolves it to the address Veracross exports before the
// layer matches on it.
func TestWebsiteEntryUnderAnAliasMatchesByEmail(t *testing.T) {
	p := model(t, "hank.morrow@heliosschool.org")
	if p.Facts != "Hank has kept the boilers running through nine winters and knows every valve by name." {
		t.Errorf("facts = %q, want the bio published under the alias", p.Facts)
	}
	if p := sampleModel(t).Person("facilities@heliosschool.org"); p != nil {
		t.Errorf("the alias %s reached the directory as a person", p.Email)
	}
}

// Which of a person's addresses their Workspace account calls primary is nobody's
// deliberate choice, so signing in under an alias has to reach the same record.
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

// An alias nothing publishes any more is stale, and stale entries are refused rather
// than carried: a match key that quietly stops working looks exactly like a page
// nobody has updated.
func TestAliasMatchingNothingIsFatal(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	next := *tables
	next.Aliases = append(append([]map[string]string{}, tables.Aliases...), map[string]string{
		AliasColumn: "nobody@heliosschool.org", AliasEmailColumn: "ruth.amari@heliosschool.org",
	})
	if _, err := BuildModel(&next, noBlobs{}, noBlobs{}); err == nil {
		t.Error("an alias matching no import row should fail the load")
	}
}

// A family is keyed by its alphabetically first adult email, and the Families tab -
// keyed the same way - is the one home of its fields.
func TestFamilyFieldsComeFromTheFamiliesTab(t *testing.T) {
	m := sampleModel(t)
	keys := m.FamilyKeysOf("marco.torres@heliosschool.org")
	if len(keys) != 1 || keys[0] != "elena.torres@heliosschool.org" {
		t.Fatalf("family keys = %v, want the alphabetically first adult email", keys)
	}
	family := m.Families[keys[0]]
	if family.PhotoUpdated != "2026-08-20" {
		t.Errorf("family photo updated = %q, want the Families tab value", family.PhotoUpdated)
	}
	if family.Phone != "650-555-0141" {
		t.Errorf("family phone = %q, want the imported household phone", family.Phone)
	}
}

// A kid in two households belongs to two families, and both list them.
func TestTwoHouseholdKidBelongsToBothFamilies(t *testing.T) {
	m := sampleModel(t)
	keys := m.FamilyKeysOf("dev.chandra@heliosschool.org")
	want := []string{"asha.chandra@heliosschool.org", "rohan.chandra@heliosschool.org"}
	if len(keys) != 2 || keys[0] != want[0] || keys[1] != want[1] {
		t.Fatalf("family keys = %v, want both households in sorted order %v", keys, want)
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

// The Families tab is keyed by the family key itself, so a row keyed by any other
// parent of the household refuses to load rather than silently starting a second
// place for the same family's fields to live.
func TestFamiliesRowKeyedByTheWrongParentIsFatal(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	misKeyed := tables.withFamily("marco.torres@heliosschool.org", map[string]string{"Family Phone": "650-555-0000"})
	if _, err := BuildModel(misKeyed, noBlobs{}, noBlobs{}); err == nil {
		t.Error("a Families row keyed by the non-first parent should fail the load")
	}
}

// Staff Veracross does not carry still enter through a flagged Overrides row.
func TestStaffNotInVeracrossStillLoad(t *testing.T) {
	p := model(t, "noa.adler@heliosschool.org")
	if !p.IsStaff || p.FullName != "Noa Adler" || p.JobTitle != "Music Teacher" {
		t.Errorf("added staff row did not load: %+v", p)
	}
}

// Regression for the reorder-photos handler failing to retire a legacy Primary
// Photo override: unlike Phone or Address, Primary Photo has no import-provided
// baseline, so applyOverrides always starts it at "" and writing "-" to clear it
// (the convention for fields that do have a baseline) is always flagged as
// clearing an already-empty value, failing the whole model load. Clearing with an
// empty string instead - which deletes the cell rather than marking it explicitly
// blank - is the fix, and this test would have caught the bug reintroduced.
func TestClearingLegacyPrimaryPhotoOverrideSucceeds(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	email := "ruth.amari@heliosschool.org"
	withPrimary := tables.withOverride(email, map[string]string{"Primary Photo": "somephoto.jpg"})
	if _, err := BuildModel(withPrimary, noBlobs{}, noBlobs{}); err != nil {
		t.Fatalf("seed a legacy Primary Photo override: %v", err)
	}

	if _, err := BuildModel(withPrimary.withOverride(email, map[string]string{"Primary Photo": ""}), noBlobs{}, noBlobs{}); err != nil {
		t.Errorf("clearing Primary Photo with an empty string should succeed, got: %v", err)
	}

	if _, err := BuildModel(withPrimary.withOverride(email, map[string]string{"Primary Photo": "-"}), noBlobs{}, noBlobs{}); err == nil {
		t.Errorf("clearing Primary Photo with \"-\" should still fail the useless-override check, documenting why \"\" is required")
	}
}

// Same class of bug as the Primary Photo regression above: Pronouns has no
// import baseline either (see edit's "pronouns" case, upload.go), so clearing
// it must also write "" rather than clearable's "-".
func TestClearingPronounsSucceeds(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	email := "ruth.amari@heliosschool.org"
	withPronouns := tables.withOverride(email, map[string]string{"Pronouns": "she/her"})
	if _, err := BuildModel(withPronouns, noBlobs{}, noBlobs{}); err != nil {
		t.Fatalf("seed pronouns: %v", err)
	}

	if _, err := BuildModel(withPronouns.withOverride(email, map[string]string{"Pronouns": ""}), noBlobs{}, noBlobs{}); err != nil {
		t.Errorf("clearing Pronouns with an empty string should succeed, got: %v", err)
	}

	if _, err := BuildModel(withPronouns.withOverride(email, map[string]string{"Pronouns": "-"}), noBlobs{}, noBlobs{}); err == nil {
		t.Errorf("clearing Pronouns with \"-\" should still fail the useless-override check, documenting why \"\" is required")
	}
}

// Family Photo Caption is a Families-tab column, cleared with clearable's "-"
// convention like every other column there: applyFamilies reads "-" as an explicit
// clear and "" as no cell at all. This is a sanity check that a real caption clears
// successfully with "-", the opposite regression from the two tests above.
func TestClearingFamilyPhotoCaptionSucceeds(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	key := "carmen.alvarez@heliosschool.org"
	withCaption := tables.withFamily(key, map[string]string{"Family Photo Caption": "Carmen at the beach."})
	m, err := BuildModel(withCaption, noBlobs{}, noBlobs{})
	if err != nil {
		t.Fatalf("seed a family photo caption: %v", err)
	}
	if m.Families[key].PhotoCaption != "Carmen at the beach." {
		t.Fatalf("caption did not seed correctly: %+v", m.Families[key])
	}

	cleared, err := BuildModel(withCaption.withFamily(key, map[string]string{"Family Photo Caption": "-"}), noBlobs{}, noBlobs{})
	if err != nil {
		t.Fatalf("clearing Family Photo Caption with \"-\" should succeed, got: %v", err)
	}
	if got := cleared.Families[key].PhotoCaption; got != "" {
		t.Errorf("caption = %q after clearing with \"-\", want empty", got)
	}
}

// Pronunciation is only ever set via the upload endpoint (edit's "pronunciation"
// case rejects a non-empty value) - it has no import baseline, same class as
// Pronouns/Primary Photo above, so clearing it must write "" rather than
// clearable's "-".
func TestClearingPronunciationSucceeds(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	email := "ruth.amari@heliosschool.org"
	blobs := fakeBlobs{"pronunciation/somefile.webm": true}
	withPronunciation := tables.withOverride(email, map[string]string{"Pronunciation": "somefile.webm"})
	seeded, err := BuildModel(withPronunciation, blobs, noBlobs{})
	if err != nil {
		t.Fatalf("seed a pronunciation: %v", err)
	}
	if got := seeded.Person(email).pronunciation; got != "somefile.webm" {
		t.Fatalf("pronunciation did not seed correctly: %q", got)
	}

	cleared, err := BuildModel(withPronunciation.withOverride(email, map[string]string{"Pronunciation": ""}), blobs, noBlobs{})
	if err != nil {
		t.Errorf("clearing Pronunciation with an empty string should succeed, got: %v", err)
	} else if got := cleared.Person(email).pronunciation; got != "" {
		t.Errorf("pronunciation = %q after clearing with \"\", want empty", got)
	}

	if _, err := BuildModel(withPronunciation.withOverride(email, map[string]string{"Pronunciation": "-"}), noBlobs{}, noBlobs{}); err == nil {
		t.Errorf("clearing Pronunciation with \"-\" should still fail the useless-override check, documenting why \"\" is required")
	}
}

// Family Pronunciation is a raw Families-tab read (applyFamilies), not routed
// through apply()'s useless-override check - so unlike person Pronunciation
// above, clearing it with "" works with no "-" fallback to document, since
// there's no useless-override check to trip either way.
func TestClearingFamilyPronunciationSucceeds(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	key := "carmen.alvarez@heliosschool.org"
	blobs := fakeBlobs{"pronunciation/somefile.webm": true}
	withPronunciation := tables.withFamily(key, map[string]string{"Family Pronunciation": "somefile.webm"})
	seeded, err := BuildModel(withPronunciation, blobs, noBlobs{})
	if err != nil {
		t.Fatalf("seed a family pronunciation: %v", err)
	}
	if got := seeded.Families[key].pronunciation; got != "somefile.webm" {
		t.Fatalf("family pronunciation did not seed correctly: %q", got)
	}

	cleared, err := BuildModel(withPronunciation.withFamily(key, map[string]string{"Family Pronunciation": ""}), blobs, noBlobs{})
	if err != nil {
		t.Fatalf("clearing Family Pronunciation with an empty string should succeed, got: %v", err)
	}
	if got := cleared.Families[key].pronunciation; got != "" {
		t.Errorf("family pronunciation = %q after clearing with \"\", want empty", got)
	}
}

// fakeBlobs simulates specific objects existing in the bucket, unlike noBlobs
// (which simulates none existing) - needed to exercise real photo/crop URL
// resolution rather than the empty-name early return every other test relies on.
type fakeBlobs map[string]bool

func (f fakeBlobs) Has(key string) bool { return f[key] }

// A photo with a linked crop shows the crop wherever it's the effective, square
// display URL, while the original stays available separately for "View photo".
func TestPhotoCropResolvesOverOriginal(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	email := "elena.torres@heliosschool.org"
	withCrop := tables.withPhotos(email, []photoRef{{Name: "orig.jpg", CropName: "crop.jpg"}})
	blobs := fakeBlobs{"photos/orig.jpg": true, "photos/crop.jpg": true}
	m, err := BuildModel(withCrop, blobs, noBlobs{})
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

// A crop naming an object that isn't actually in the bucket - the object went
// missing, or the sheet row is stale - falls back to the original rather than
// failing the whole model load, unlike an unresolvable original name.
func TestMissingCropFallsBackToOriginal(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	email := "elena.torres@heliosschool.org"
	withCrop := tables.withPhotos(email, []photoRef{{Name: "orig.jpg", CropName: "missing-crop.jpg"}})
	blobs := fakeBlobs{"photos/orig.jpg": true}
	m, err := BuildModel(withCrop, blobs, noBlobs{})
	if err != nil {
		t.Fatalf("a missing crop should not fail the model load: %v", err)
	}
	p := m.Person(email)
	if len(p.Photos) != 1 || p.Photos[0].URL != "/photos/orig.jpg" {
		t.Errorf("photos = %+v, want URL to fall back to the original", p.Photos)
	}
}

// Someone with no photos of their own shows the initials placeholder - never the
// family photo standing in for them, however present it is.
func TestPersonWithNoPhotosShowsNoFamilyPhoto(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	key := "elena.torres@heliosschool.org"
	withFamilyPhoto := tables.withFamily(key, map[string]string{"Family Photo": "family.jpg"})
	blobs := fakeBlobs{"photos/family.jpg": true}
	m, err := BuildModel(withFamilyPhoto, blobs, noBlobs{})
	if err != nil {
		t.Fatalf("build model with a family photo: %v", err)
	}
	if got := m.Families[key].PhotoURL; got != "/photos/family.jpg" {
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

// Unlike the photo case above, pronunciation does fall back: a family member with
// no recording of their own hears the family's instead of nothing.
func TestPersonWithNoPronunciationFallsBackToFamilyPronunciation(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	key := "elena.torres@heliosschool.org"
	withFamilyPronunciation := tables.withFamily(key, map[string]string{"Family Pronunciation": "family.webm"})
	blobs := fakeBlobs{"pronunciation/family.webm": true}
	m, err := BuildModel(withFamilyPronunciation, blobs, noBlobs{})
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

// A personal recording overrides the family's fallback rather than being masked
// by it - HasOwnPronunciation is what the frontend uses to decide whether the
// delete button (which clears only the personal recording) should show.
func TestOwnPronunciationOverridesFamilyFallback(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	withBoth := tables.
		withFamily("elena.torres@heliosschool.org", map[string]string{"Family Pronunciation": "family.webm"}).
		withOverride("mia.torres@heliosschool.org", map[string]string{"Pronunciation": "mia.webm"})
	blobs := fakeBlobs{"pronunciation/family.webm": true, "pronunciation/mia.webm": true}
	m, err := BuildModel(withBoth, blobs, noBlobs{})
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

// blobsWith reports every named object as present, so a photo name in the
// sheet resolves to a URL the way it does against the real bucket.
type blobsWith map[string]bool

func (b blobsWith) Has(name string) bool { return b[name] }

// The topbar avatar on both the directory and the link portal leads with this,
// so the own-photo-then-family fallback is worth pinning down. The sample
// carries no photos at all, which is why these are built by hand.
func TestHeroPhotoPrefersOwnPhotoThenFallsBackToTheFamily(t *testing.T) {
	const email = "jordan.whitfield@heliosschool.org"
	key := sampleModel(t).FamilyKeysOf(email)
	if len(key) == 0 {
		t.Fatalf("%s has no family in the sample", email)
	}
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	blobs := blobsWith{"photos/own.jpg": true, "photos/fam.jpg": true}

	family := tables.withFamily(key[0], map[string]string{"Family Photo": "fam.jpg"})
	m, err := BuildModel(family, blobs, noBlobs{})
	if err != nil {
		t.Fatalf("build model with a family photo: %v", err)
	}
	if got := m.HeroPhoto(email); got != "/photos/fam.jpg" {
		t.Errorf("with only a family photo: got %q, want the family's", got)
	}

	own := family.withPhotos(email, []photoRef{{Name: "own.jpg"}})
	m, err = BuildModel(own, blobs, noBlobs{})
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
