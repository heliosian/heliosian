package model

import (
	"heliosian/internal/cells"
)

func (m *Birthdays) charityRows() []map[string]string {
	rows := []map[string]string{}
	for _, c := range m.Charities {
		rows = append(rows, map[string]string{"Charity ID": c.ID, "Name": c.Name, "Donation Link": c.DonationLink, "Allowed": cells.YesNoCell(c.Allowed)})
	}
	return rows
}

func (m *Birthdays) settingRows() []map[string]string {
	s := m.Settings
	return []map[string]string{
		{"Key": DefaultCharityKey, "Value": s.DefaultCharity}, {"Key": YearStartKey, "Value": s.YearStart},
		{"Key": EmailSubjectKey, "Value": s.EmailSubject}, {"Key": EmailBodyKey, "Value": s.EmailBody}, {"Key": NoNewsletterNoteKey, "Value": s.NoNewsletterNote},
	}
}
