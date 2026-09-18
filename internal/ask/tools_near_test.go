package ask

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"heliosian/internal/geocode"
)

func TestNearbyFamiliesReadWhatTheMapShows(t *testing.T) {
	sources := sampleSources(t)
	directory := sources.Directory()
	masked := ""
	for key, family := range directory.Families {
		if family.Address == "" {
			if family.AddressMasked {
				masked = family.Name
			}
			continue
		}
		point, err := geocode.Fake{}.Lookup(family.Address)
		if err != nil {
			t.Fatal(err)
		}
		family.Lat, family.Lng = point.Lat, point.Lng
		directory.Families[key] = family
	}
	if masked == "" {
		t.Fatal("the sample community has no family that keeps its address to itself")
	}
	v := app{sources: sources}.viewer(jordan)
	own := map[string]bool{}
	for _, key := range directory.FamilyKeysOf(jordan) {
		own[whoBase+"/families/"+key] = true
	}
	byLink := map[string]string{}
	for key, family := range directory.Families {
		byLink[whoBase+"/families/"+key] = family.Address
	}
	var result struct {
		Families []nearbyCard `json:"families"`
		SameCity []nearbyCard `json:"sameCityNoDistance"`
	}
	out, err := v.run(context.Background(), "nearby_families", json.RawMessage(`{"limit":30}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Families) < 2 {
		t.Fatalf("found %d families", len(result.Families))
	}
	for i, c := range result.Families {
		if own[c.Link] || c.Name == masked || !streetAddress.MatchString(byLink[c.Link]) || c.Miles <= 0 {
			t.Errorf("listed %+v", c)
		}
		if i > 0 && c.Miles < result.Families[i-1].Miles {
			t.Errorf("%s at %.1f miles comes after %.1f", c.Name, c.Miles, result.Families[i-1].Miles)
		}
	}
	for _, c := range result.SameCity {
		if own[c.Link] || c.Name == masked || streetAddress.MatchString(byLink[c.Link]) || c.Miles != 0 {
			t.Errorf("listed by city %+v", c)
		}
	}
	if strings.Contains(out, masked) {
		t.Errorf("the family that keeps its address to itself is named: %s", out)
	}
	out, err = v.run(context.Background(), "nearby_families", json.RawMessage(`{"classroom":"Jays"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	for _, c := range append(result.Families, result.SameCity...) {
		if !strings.Contains(strings.Join(c.Students, ";"), "Jays") {
			t.Errorf("no student in Jays: %+v", c)
		}
	}
	if _, err := v.run(context.Background(), "nearby_families", json.RawMessage(`{"name":"`+masked+`"}`)); err == nil {
		t.Errorf("measured from %s, who share no address", masked)
	}
}
