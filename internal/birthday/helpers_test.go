package birthday

import "time"

func mustTime(s string) time.Time {
	t, err := time.Parse(DateFormat, s)
	if err != nil {
		panic(err)
	}
	return t
}

func (m *Model) charityRows() []map[string]string {
	rows := []map[string]string{}
	for _, c := range m.Charities {
		rows = append(rows, map[string]string{"Name": c.Name, "Donation Link": c.DonationLink, "Allowed": YesNo(c.Allowed)})
	}
	return rows
}

func (m *Model) settingRows() []map[string]string {
	s := m.Settings
	return []map[string]string{
		{"Key": DefaultCharityKey, "Value": s.DefaultCharity}, {"Key": YearStartKey, "Value": s.YearStart},
		{"Key": EmailSubjectKey, "Value": s.EmailSubject}, {"Key": EmailBodyKey, "Value": s.EmailBody}, {"Key": NoNewsletterNoteKey, "Value": s.NoNewsletterNote},
	}
}
