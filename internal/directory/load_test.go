package directory

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
	// Her family opted out on the consent form, which masks the household phone. The
	// staff merge must not resurrect it by filling from the export.
	if p.Phone != "" || !p.PhoneMasked {
		t.Errorf("phone = %q masked = %t, want it cleared by preference", p.Phone, p.PhoneMasked)
	}
}

// Both of Mia's parents carry family cells on their own Overrides rows: Marco's has a
// stale Family Photo Updated left over from a prior year, Elena's reflects her actual
// recent upload. The merge must keep the later date regardless of which row it visits
// first, rather than whichever row happens to be processed last.
func TestFamilyPhotoUpdatedKeepsTheLatestParentDate(t *testing.T) {
	m := sampleModel(t)
	p := m.Person("elena.torres@heliosschool.org")
	if p == nil || p.FamilyKey == "" {
		t.Fatalf("elena.torres has no family: %+v", p)
	}
	family, ok := m.Families[p.FamilyKey]
	if !ok {
		t.Fatalf("family %s not found", p.FamilyKey)
	}
	if family.PhotoUpdated != "2026-08-20" {
		t.Errorf("family photo updated = %q, want Elena's newer date to win over Marco's stale 2023-01-01", family.PhotoUpdated)
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

// Someone with no photos of their own shows their family's photo instead of blank
// initials, and PhotoUpdated stays unset so the "add your own" nag isn't silenced.
func TestPersonWithNoPhotosFallsBackToFamilyPhoto(t *testing.T) {
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample tables: %v", err)
	}
	email := "elena.torres@heliosschool.org"
	withFamilyPhoto := tables.withOverride(email, map[string]string{"Family Photo": "family.jpg"})
	blobs := fakeBlobs{"photos/family.jpg": true}
	m, err := BuildModel(withFamilyPhoto, blobs, noBlobs{})
	if err != nil {
		t.Fatalf("build model with a family photo: %v", err)
	}
	p := m.Person("mia.torres@heliosschool.org")
	if p == nil || p.FamilyKey == "" {
		t.Fatalf("mia.torres has no family: %+v", p)
	}
	if len(p.Photos) != 0 {
		t.Fatalf("test assumes mia.torres has no photos of her own, got %+v", p.Photos)
	}
	family := m.Families[p.FamilyKey]
	if p.PhotoURL != family.PhotoURL || p.PhotoURL != "/photos/family.jpg" {
		t.Errorf("PhotoURL = %q, want it to fall back to the family photo %q", p.PhotoURL, family.PhotoURL)
	}
	if p.PhotoUpdated != "" {
		t.Errorf("PhotoUpdated = %q, want it to stay unset so photoNeedsUpdate still nags for a real photo", p.PhotoUpdated)
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
