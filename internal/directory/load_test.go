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

// Staff Veracross does not carry still enter through a flagged Overrides row.
func TestStaffNotInVeracrossStillLoad(t *testing.T) {
	p := model(t, "noa.adler@heliosschool.org")
	if !p.IsStaff || p.FullName != "Noa Adler" || p.JobTitle != "Music Teacher" {
		t.Errorf("added staff row did not load: %+v", p)
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
