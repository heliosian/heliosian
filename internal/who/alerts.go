package who

import (
	"time"

	"heliosian/internal/config"
)

// Alerts is what the toolbar's badges say for a person, the same in every
// app: how many of their family's photos and facts want a new-year update
// (the count badge), and whether their Veracross privacy settings disagree
// with their Helios Who? ones (the warning triangle). It mirrors the
// directory's own client-side reckoning in web/who/stale.js and
// web/who/pages/privacy.js, so the badges agree wherever they show.
type Alerts struct {
	Stale   int  `json:"stale"`
	Privacy bool `json:"privacy"`
}

// agedPast says whether something last updated on `updated` (a YYYY-MM-DD
// cell, or blank) is older than `years`; an unreadable date counts as old.
func agedPast(updated string, years float64, now time.Time) bool {
	when, err := time.Parse(updatedFormat, updated)
	if err != nil {
		return true
	}
	return now.Sub(when) > time.Duration(years*365.25*24)*time.Hour
}

// Alerts reckons the badges for the person signed in as email, against the
// config sheet's staleness thresholds.
func (m *Model) Alerts(email string, years config.StaleYears, now time.Time) Alerts {
	me := m.Person(email)
	if me == nil {
		return Alerts{}
	}
	var family *Family
	if keys := m.FamilyKeysOf(email); len(keys) > 0 {
		if f, ok := m.Families[keys[0]]; ok {
			family = &f
		}
	}
	var alerts Alerts
	// Staff with no family of their own have nothing to keep fresh here.
	if !me.IsStaff || me.IsParent {
		emails := []string{me.Email}
		if family != nil {
			emails = append(append(emails, family.AdultEmails...), family.KidEmails...)
		}
		seen := map[string]bool{}
		for _, e := range emails {
			p := m.Person(e)
			if p == nil || seen[e] {
				continue
			}
			seen[e] = true
			if p.IsStudent && (p.PhotoURL == "" || agedPast(p.PhotoUpdated, years.Photo, now)) {
				alerts.Stale++
			}
			if p.IsStudent && (p.Facts == "" || agedPast(p.FactsUpdated, years.Facts, now)) {
				alerts.Stale++
			}
		}
		if family != nil && (family.PhotoURL == "" || agedPast(family.PhotoUpdated, years.FamilyPhoto, now)) {
			alerts.Stale++
		}
	}
	if family != nil {
		alerts.Privacy = (family.AddressMasked && family.VeracrossAddress != "hidden") || (family.PhoneMasked && family.VeracrossPhone != "hidden")
	}
	return alerts
}

// Alerts is the model's reckoning for the signed-in address, as the other
// apps' toolbars ask for it.
func (c *Cache) Alerts(email string, years config.StaleYears) Alerts {
	model := c.Model()
	return model.Alerts(model.Resolve(email), years, time.Now())
}
