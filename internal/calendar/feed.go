package calendar

import (
	"strings"
	"time"

	"heliosian/internal/mail"
)

var brand = mail.Brand{Name: "Helios When", Color: "#0e4d54", Tagline: "the school calendar"}

func uidOf(id string) string {
	if strings.Contains(id, "@") {
		return id
	}
	return id + "@when.heliosian.com"
}

func ICS(model *Model, directory Directory, f *Feed, linked []Linked, origin string, now time.Time) []byte {
	c := mail.Calendar{Product: brand.Name, Method: mail.MethodPublish, Name: f.Name, TimeZone: Location.String(), Stamp: now}
	for _, e := range model.eventsFor(directory, f.Email, linked) {
		if answer := model.AnswerOf(f.Email, e.ID); !f.Carries(e) || answer == AnswerHidden || answer == AnswerNo {
			continue
		}
		m := e.mailEvent(origin + EventPath(e))
		m.Description = e.Description
		if e.DayType != "" {
			m.Description = strings.TrimSpace(e.DayType + "\n\n" + e.Description)
		}
		if modified, err := time.ParseInLocation(DateTimeFormat, e.Updated, Location); err == nil {
			m.Modified = modified
		}
		if len(e.Tags) > 0 {
			m.Categories = JoinList(e.Tags)
		}
		c.Events = append(c.Events, m)
	}
	return []byte(c.ICS())
}
