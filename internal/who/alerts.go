package who

import (
	"slices"
	"strings"
	"time"

	"heliosian/internal/config"
)

// Alerts is what the toolbar's badges say for a person, the same in every
// app: which of their family's photos and facts want a new-year update -
// "Sam's photo", "Your facts", "Family photo" - (the bell's count and its
// card's rows), and which details their Veracross privacy settings
// show that their Helios Who? ones hide - "address", "phone" - (the warning
// triangle, its count and its card's rows). It mirrors the
// directory's own client-side reckoning in web/who/stale.js and
// web/who/pages/privacy.js, so the badges agree wherever they show.
type Alerts struct {
	Stale   []string `json:"stale"`
	Privacy []string `json:"privacy"`
}

// firstName is the name a person goes by, first word alone, as the bell's
// card calls them: their preferred name, else their full one.
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
			// Each by what it is and whose: "Your photo", "Sam's facts".
			whose := "Your"
			if e != me.Email {
				whose = firstName(p) + "'s"
			}
			if p.IsStudent && (p.PhotoURL == "" || agedPast(p.PhotoUpdated, years.Photo, now)) {
				alerts.Stale = append(alerts.Stale, whose+" photo")
			}
			if p.IsStudent && (p.Facts == "" || agedPast(p.FactsUpdated, years.Facts, now)) {
				alerts.Stale = append(alerts.Stale, whose+" facts")
			}
		}
		// The family photo is its adults' to keep, not a student's.
		if family != nil && slices.Contains(family.AdultEmails, email) && (family.PhotoURL == "" || agedPast(family.PhotoUpdated, years.FamilyPhoto, now)) {
			alerts.Stale = append(alerts.Stale, "Family photo")
		}
	}
	// One for each detail hidden here but visible on Veracross: the
	// address, the phone.
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

// Alerts is the model's reckoning for the signed-in address, as the other
// apps' toolbars ask for it.
func (c *Cache) Alerts(email string, years config.StaleYears) Alerts {
	model := c.Model()
	return model.Alerts(model.Resolve(email), years, time.Now())
}
