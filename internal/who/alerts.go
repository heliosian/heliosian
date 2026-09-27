package who

import (
	"slices"
	"strings"
	"time"

	"heliosian/internal/config"
)

type Alerts struct {
	Stale   []string `json:"stale"`
	Privacy []string `json:"privacy"`
}

func firstName(p *Person) string {
	name := p.PreferredName
	if name == "" {
		name = p.FullName
	}
	if words := strings.Fields(name); len(words) > 0 {
		return words[0]
	}
	return p.Email
}

func agedPast(updated string, years float64, now time.Time) bool {
	when, err := time.Parse(updatedFormat, updated)
	if err != nil {
		return true
	}
	return now.Sub(when) > time.Duration(years*365.25*24)*time.Hour
}

func (m *Model) Alerts(email string, years config.StaleYears, now time.Time) Alerts {
	me := m.Person(email)
	if me == nil {
		return Alerts{}
	}
	var family *Family
	if f, ok := m.FamilyOf(email); ok {
		family = &f
	}
	var alerts Alerts
	if !me.IsStaff || me.IsParent {
		adults, kids := m.Household(me.Email)
		for _, p := range append(append([]*Person{me}, adults...), kids...) {
			whose := "Your"
			if p.Email != me.Email {
				whose = firstName(p) + "'s"
			}
			if p.IsStudent && (p.PhotoURL == "" || agedPast(p.PhotoUpdated, years.Photo, now)) {
				alerts.Stale = append(alerts.Stale, whose+" photo")
			}
			if p.IsStudent && (p.Facts == "" || agedPast(p.FactsUpdated, years.Facts, now)) {
				alerts.Stale = append(alerts.Stale, whose+" facts")
			}
		}
		if family != nil && slices.Contains(family.AdultEmails, email) && (family.PhotoURL == "" || agedPast(family.PhotoUpdated, years.FamilyPhoto, now)) {
			alerts.Stale = append(alerts.Stale, "Family photo")
		}
	}
	if family != nil {
		if family.AddressMasked && family.VeracrossAddress != "hidden" {
			alerts.Privacy = append(alerts.Privacy, "address")
		}
		if family.PhoneMasked && family.VeracrossPhone != "hidden" {
			alerts.Privacy = append(alerts.Privacy, "phone")
		}
	}
	return alerts
}

func (c *Cache) Alerts(email string, years config.StaleYears) Alerts {
	model := c.Model()
	return model.Alerts(model.Resolve(email), years, time.Now())
}
