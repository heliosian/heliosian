package ask

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/who"
)

var streetAddress = regexp.MustCompile(`^\s*\d`)

type nearbyCard struct {
	Name     string   `json:"name"`
	Miles    float64  `json:"milesAway,omitempty"`
	City     string   `json:"city,omitempty"`
	Adults   []string `json:"adults"`
	Students []string `json:"students"`
	Link     string   `json:"link"`
}

func cityOf(address string) string {
	parts := strings.Split(address, ",")
	if !streetAddress.MatchString(address) {
		return strings.TrimSpace(parts[0])
	}
	if len(parts) >= 3 {
		return strings.TrimSpace(parts[len(parts)-2])
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func milesBetween(a, b who.Family) float64 {
	const earth = 3958.8
	lat1, lat2 := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	dLat, dLng := lat2-lat1, (b.Lng-a.Lng)*math.Pi/180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earth * math.Asin(math.Sqrt(h))
}

func (v *viewer) nearbyCard(key string, family who.Family) nearbyCard {
	c := nearbyCard{Name: family.Name, City: cityOf(family.Address), Adults: []string{}, Students: []string{}, Link: whoBase + "/families/" + key}
	for _, email := range family.AdultEmails {
		if p := v.directory.Person(email); p != nil {
			c.Adults = append(c.Adults, p.FullName)
		}
	}
	for _, email := range family.KidEmails {
		if p := v.directory.Person(email); p != nil {
			c.Students = append(c.Students, p.FullName+" ("+placeWords(v, p)+")")
		}
	}
	return c
}

func (v *viewer) familyMatches(family who.Family, classroom, grade string) bool {
	if classroom == "" && grade == "" {
		return true
	}
	for _, email := range family.KidEmails {
		p := v.directory.Person(email)
		if p == nil {
			continue
		}
		if (classroom == "" || strings.EqualFold(p.Classroom, classroom)) && (grade == "" || contains(p.Grade, grade)) {
			return true
		}
	}
	return false
}

var nearbyFamilies = tool{
	name:        "nearby_families",
	description: "Families who live near a family - the viewer's own unless another is named - nearest first, by straight-line distance between the addresses families share in Helios Who?, as its map shows them: for carpools, walking groups and playdates. Each comes with its adults, its students' grades and classrooms, and its city. Narrow them to a classroom or a grade. A family that shares only its city has no distance and is listed apart when it is in the same city; a family that shares no address is never listed.",
	words:       "Looking for families nearby",
	properties: map[string]any{
		"name":         str("Words of a member's name or the family's name, to measure from a family other than the viewer's."),
		"classroom":    str("Only families with a student in this classroom."),
		"grade":        str("Only families with a student in this grade, as Grade 3 or Kindergarten."),
		"within_miles": map[string]any{"type": "number", "description": "Only families within this many miles."},
		"limit":        integer("How many families to return, 10 unless said, 30 at most."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			Name, Classroom, Grade string
			WithinMiles            float64 `json:"within_miles"`
			Limit                  int
		}](input)
		if err != nil {
			return nil, err
		}
		keys := v.directory.FamilyKeysOf(v.email)
		if strings.TrimSpace(in.Name) != "" {
			keys = v.familyKeys("", in.Name)
		}
		if len(keys) == 0 {
			return nil, fmt.Errorf("no family in the directory matches that")
		}
		from, fromKey, city := who.Family{}, "", ""
		for _, key := range keys {
			family := v.directory.Families[key]
			if family.Address == "" {
				continue
			}
			if city == "" {
				city = cityOf(family.Address)
			}
			if streetAddress.MatchString(family.Address) && family.Lat != 0 {
				from, fromKey, city = family, key, cityOf(family.Address)
				break
			}
		}
		if city == "" {
			return nil, fmt.Errorf("that family shares no address in Helios Who?, so there is nothing to measure from")
		}
		classroom, grade := strings.TrimSpace(in.Classroom), strings.TrimSpace(in.Grade)
		near, sameCity := []nearbyCard{}, []nearbyCard{}
		for key, family := range v.directory.Families {
			if slices.Contains(keys, key) || family.Address == "" || !v.familyMatches(family, classroom, grade) {
				continue
			}
			if !streetAddress.MatchString(family.Address) || family.Lat == 0 || fromKey == "" {
				if strings.EqualFold(cityOf(family.Address), city) {
					sameCity = append(sameCity, v.nearbyCard(key, family))
				}
				continue
			}
			c := v.nearbyCard(key, family)
			c.Miles = math.Round(milesBetween(from, family)*10) / 10
			if in.WithinMiles > 0 && c.Miles > in.WithinMiles {
				continue
			}
			near = append(near, c)
		}
		sort.Slice(near, func(i, j int) bool {
			if near[i].Miles != near[j].Miles {
				return near[i].Miles < near[j].Miles
			}
			return near[i].Name < near[j].Name
		})
		sort.Slice(sameCity, func(i, j int) bool { return sameCity[i].Name < sameCity[j].Name })
		limit := limitOf(in.Limit, 10, 30)
		out := map[string]any{
			"from": map[string]string{"city": city}, "families": near[:min(limit, len(near))], "sameCityNoDistance": sameCity[:min(limit, len(sameCity))],
			"note": "Distances are straight lines between the addresses families share, not driving routes.",
		}
		if fromKey != "" {
			out["from"] = map[string]string{"family": from.Name, "city": city}
		}
		return out, nil
	},
}
