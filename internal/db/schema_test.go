package db

import (
	"slices"
	"testing"
)

func TestSchemaHangsTogether(t *testing.T) {
	declared := map[string]bool{}
	for _, table := range Tables {
		if declared[table.Name] {
			t.Fatalf("%s declared twice", table.Name)
		}
		declared[table.Name] = true
	}
	owners := map[string]string{}
	for _, table := range Tables {
		if table.Generated != (table.Sheet == "") {
			t.Errorf("%s: a table is either generated or on a sheet", table.Name)
		}
		if table.Sheet != "" && !slices.Contains(Sheets, table.Sheet) {
			t.Errorf("%s: no sheet %s", table.Name, table.Sheet)
		}
		names := map[string]bool{}
		for _, c := range table.Columns {
			if names[c.Name] {
				t.Errorf("%s.%s declared twice", table.Name, c.Name)
			}
			names[c.Name] = true
			switch c.Kind {
			case Enum:
				if len(c.Values) == 0 {
					t.Errorf("%s.%s is an enum with no values", table.Name, c.Name)
				}
			case ID:
				if owner, dup := owners[c.Prefix]; dup {
					t.Errorf("%s.%s and %s share prefix %s", table.Name, c.Name, owner, c.Prefix)
				}
				owners[c.Prefix] = table.Name + "." + c.Name
			case Ref, Refs:
				if c.Target == "" {
					continue
				}
				target, ok := Lookup(c.Target)
				if !ok {
					t.Errorf("%s.%s refers to no table %s", table.Name, c.Name, c.Target)
					continue
				}
				if len(target.Key) != 1 {
					t.Errorf("%s.%s refers to %s, which has no single key", table.Name, c.Name, c.Target)
					continue
				}
				if k, _ := target.Column(target.Key[0]); k.Kind != ID {
					t.Errorf("%s.%s refers to %s, whose key is not a minted id", table.Name, c.Name, c.Target)
				}
			}
		}
		if table.Generated {
			continue
		}
		if len(table.Key) == 0 {
			t.Errorf("%s has no key", table.Name)
		}
		for _, k := range table.Key {
			c, ok := table.Column(k)
			if !ok || c.Generated {
				t.Errorf("%s: key column %s is not a stored column", table.Name, k)
			}
		}
	}
	for prefix := range prefixes {
		if _, ok := owners[prefix]; !ok {
			t.Errorf("prefix %s names no id column", prefix)
		}
	}
}

func TestStoredLeavesOutGenerated(t *testing.T) {
	person, _ := Lookup("PERSON")
	stored := person.Stored()
	if slices.Contains(stored, "name_show") || !slices.Contains(stored, "name_long_override") {
		t.Fatalf("PERSON stores %v", stored)
	}
}

func TestCheck(t *testing.T) {
	member, _ := Lookup("MEMBER")
	good := map[string]string{
		"group":       "grpX7pQ2m9KdLr",
		"person":      "perX7pQ2m9KdLr",
		"role":        "Member",
		"status":      "yes",
		"price":       "12.50",
		"purchase_id": "purX7pQ2m9KdLr",
		"archived":    "No",
		"answered":    "2026-09-24 16:00",
		"unknown":     "a column added by hand",
	}
	if err := member.Check(good); err != nil {
		t.Fatalf("Check(%v) = %v", good, err)
	}
	for column, bad := range map[string]string{
		"group":       "perX7pQ2m9KdLr",
		"person":      "",
		"role":        "host",
		"price":       "12.5.0",
		"purchase_id": "grpX7pQ2m9KdLr",
		"archived":    "maybe",
		"answered":    "yesterday",
		"quantity":    "-1",
		"guest_of":    " perX7pQ2m9KdLr",
	} {
		row := map[string]string{}
		for k, v := range good {
			row[k] = v
		}
		row[column] = bad
		if err := member.Check(row); err == nil {
			t.Errorf("Check accepted %s=%q", column, bad)
		}
	}
}

func TestCheckRefs(t *testing.T) {
	view, _ := Lookup("SAVED_VIEW")
	row := map[string]string{"token": "fedX7pQ2m9KdLr", "person": "perX7pQ2m9KdLr", "groups": "grpX7pQ2m9KdLr, grpY7pQ2m9KdLr"}
	if err := view.Check(row); err != nil {
		t.Fatal(err)
	}
	row["groups"] = "grpX7pQ2m9KdLr, catY7pQ2m9KdLr"
	if err := view.Check(row); err == nil {
		t.Fatal("Check accepted a category among groups")
	}
}

func TestAnyTableReference(t *testing.T) {
	alias, _ := Lookup("ALIAS")
	if err := alias.Check(map[string]string{"alias": "k7m2q9x4v1bnc", "id": "bdyX7pQ2m9KdLr"}); err != nil {
		t.Fatal(err)
	}
	if err := alias.Check(map[string]string{"alias": "k7m2q9x4v1bnc", "id": "purX7pQ2m9KdLr"}); err == nil {
		t.Fatal("ALIAS accepted a purchase, which no table is keyed by")
	}
}
