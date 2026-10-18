package db

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/store"
)

const birthdayApp = "birthday"

func (m *Model) birthdayRows() []store.Row {
	start, err := time.Parse("01-02", m.Setting(birthdayApp, "Year Start"))
	if err != nil {
		panic("db: the birthday Year Start setting is not a month and day: " + err.Error())
	}
	askLead := leadDays(m.Setting(birthdayApp, "Request Lead Days"))
	dueLead := leadDays(m.Setting(birthdayApp, "Due By Lead Days"))
	issues := map[int][]store.Row{}
	for _, g := range m.Shown("GROUP").Referencing("parent", m.Setting(birthdayApp, "Newsletter")) {
		day, ok := dayOf(g["start"])
		if !ok {
			continue
		}
		year := birthdaySchoolYear(day, start)
		issues[year] = append(issues[year], store.Row{"id": g["id"], "start": day.Format(time.DateOnly)})
	}
	for _, list := range issues {
		slices.SortFunc(list, func(a, b store.Row) int { return strings.Compare(a["start"], b["start"]) })
	}
	records := map[string]string{}
	for _, y := range m.Shown("BIRTHDAY_YEAR").All() {
		records[y["person"]+"|"+y["year"]] = y["id"]
	}
	out := []store.Row{}
	for _, p := range m.Shown("PERSON").All() {
		born, ok := dayOf(p["birthday"])
		if !ok || p["source"] == "guest" {
			continue
		}
		for year, list := range issues {
			on := occurrence(year, start, born)
			issue := list[0]
			for _, i := range list {
				if i["start"] < on.Format(time.DateOnly) {
					issue = i
				}
			}
			sent, _ := dayOf(issue["start"])
			y := strconv.Itoa(year)
			out = append(out, store.Row{
				"id": Derive(BirthdayPrefix, p["id"], y), "person": p["id"], "year": y, "day": on.Format(time.DateOnly),
				"newsletter": issue["id"], "ask_by": sent.AddDate(0, 0, -askLead).Format(time.DateOnly),
				"due_by": sent.AddDate(0, 0, -dueLead).Format(time.DateOnly), "record": records[p["id"]+"|"+y],
			})
		}
	}
	return out
}

func leadDays(cell string) int {
	n, err := strconv.Atoi(cell)
	if err != nil {
		panic("db: a birthday lead days setting is not a number: " + cell)
	}
	return n
}

func dayOf(cell string) (time.Time, bool) {
	if len(cell) < len(time.DateOnly) {
		return time.Time{}, false
	}
	t, err := time.Parse(time.DateOnly, cell[:len(time.DateOnly)])
	return t, err == nil
}

func birthdaySchoolYear(day, start time.Time) int {
	if day.Format("01-02") < start.Format("01-02") {
		return day.Year() - 1
	}
	return day.Year()
}

func occurrence(year int, start, born time.Time) time.Time {
	y := year
	if born.Format("01-02") < start.Format("01-02") {
		y++
	}
	t := time.Date(y, born.Month(), born.Day(), 0, 0, 0, 0, time.UTC)
	if t.Month() != born.Month() {
		t = t.AddDate(0, 0, -1)
	}
	return t
}
