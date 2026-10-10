package ask

import (
	"time"

	"heliosian/internal/db"
	"heliosian/internal/store"
)

type linkCard struct {
	URL   string `json:"url"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Image string `json:"image,omitempty"`
	Badge string `json:"badge,omitempty"`
	Color string `json:"color,omitempty"`
}

var groupCards = map[string]string{"family": "family", "classroom": "classroom", "grade": "classroom", "event": "event", "activity": "activity", "party": "party"}

func (t *turn) linkCard(address string) (linkCard, bool) {
	id, ok := t.found.of(address)
	if !ok {
		return linkCard{}, false
	}
	table, ok := db.TableOf(id)
	if !ok || (table != "PERSON" && table != "GROUP") {
		return linkCard{}, false
	}
	found, _, err := t.rows(`(from %s (where (= id %q)))`, table, id)
	if err != nil || len(found) == 0 {
		return linkCard{}, false
	}
	row := found[0]
	card := linkCard{URL: address, Kind: "person", Name: row["name_show"]}
	if table == "GROUP" {
		card.Name = row["name"]
		kind, ok := groupCards[row["kind"]]
		if !ok {
			kind = "group"
		}
		card.Kind = kind
		card.Color = row["color"]
		if start := row["start"]; kind == "event" && len(start) >= len(time.DateOnly) {
			day, err := time.Parse(time.DateOnly, start[:len(time.DateOnly)])
			if err != nil {
				panic(err)
			}
			card.Badge = day.Format("Mon Jan 2")
		}
	}
	card.Image = t.picture(table, row)
	return card, true
}

func (t *turn) picture(table string, row store.Row) string {
	column := "person"
	if table == "GROUP" {
		column = "group"
	}
	found, _, err := t.rows(`(from PHOTO (where (= %s %q) (not (blank thumbnail))) (order order asc) (limit 1))`, column, row["id"])
	if err != nil || len(found) == 0 {
		return ""
	}
	return "/api/blob/" + found[0]["id"] + "/thumbnail"
}
