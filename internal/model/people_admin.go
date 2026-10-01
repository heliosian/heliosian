package model

import (
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/store"
)

func overrideStringValue(person *Person, column string) string {
	cell := person.overrideRow[column]
	if cell == "-" {
		return ""
	}
	return cell
}

func overrideBoolValue(person *Person, column string) bool {
	value, _ := cells.YesNo(person.overrideRow[column], false)
	return value
}

func familyStringValue(family Family, column string) string {
	cell := family.sheetRow[column]
	if cell == "-" {
		return ""
	}
	return cell
}

func importedValue(person *Person, column, resolved string) string {
	if person.imported != nil {
		return person.imported[column]
	}
	return resolved
}

func fullNameFallback(person *Person) string {
	return importedValue(person, "Full Name", person.FullName)
}

const (
	maxJobTitleLength = 100
	maxNameLength     = 100
	maxFactsLength    = 4000
	maxPhoneLength    = 40
	maxAddressLength  = 200
)

func diffBoolCell(row store.Row, column string, next, current bool) {
	if next == current {
		return
	}
	row[column] = cells.YesNoCell(next)
}

func diffStringCell(cells store.Row, column string, next, current string) {
	trimmed := strings.TrimSpace(next)
	if next != "" && trimmed == "" {
		cells[column] = "-"
		return
	}
	if trimmed == current {
		return
	}
	cells[column] = trimmed
}

func diffStringCellNoBaseline(cells store.Row, column string, next, current string) {
	next = strings.TrimSpace(next)
	if next == current {
		return
	}
	cells[column] = next
}

func diffFacts(cells store.Row, next string, person *Person) {
	next = strings.TrimSpace(next)
	if next == overrideStringValue(person, "Facts") {
		return
	}
	cells["Facts"] = next
	cells["Facts Updated"] = today()
}

func validClassroom(model *Directory, name string) bool {
	return name == "" || slices.ContainsFunc(model.Classrooms, func(c Classroom) bool { return c.Name == name })
}

func validCrew(model *Directory, classroom, crew string) bool {
	if crew == "" {
		return true
	}
	if classroom == "" {
		return slices.ContainsFunc(model.Crews, func(c Crew) bool { return c.Name == crew })
	}
	return slices.ContainsFunc(model.Crews, func(c Crew) bool { return c.Classroom == classroom && c.Name == crew })
}

func validGrade(grade string) bool {
	return grade == "" || slices.Contains(gradeOrder, grade)
}

func gradeBandSet() map[string]bool {
	set := map[string]bool{}
	for _, band := range gradeBands {
		set[band] = true
	}
	return set
}

func validFullName(fullName string, person *Person) error {
	if strings.TrimSpace(fullName) == "" {
		if fullNameFallback(person) == "" {
			return access.Invalid("full name required: this person has no Veracross record to fall back to")
		}
		return nil
	}
	if len(fullName) > maxNameLength {
		return access.Invalid("full name too long")
	}
	return nil
}
