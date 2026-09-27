package birthday

import (
	"regexp"
	"strings"
)

type Letter struct {
	To      string
	CC      string
	Subject string
	Body    string
}

func letter(settings Settings, sv StaffView, senderName string) Letter {
	note := ""
	if sv.Level == LevelNoNewsletter {
		note = fillLetter(settings.NoNewsletterNote, settings, sv, senderName)
	}
	body := settings.EmailBody
	if strings.Contains(body, "{no newsletter note}") {
		body = strings.ReplaceAll(body, "{no newsletter note}", note)
	} else if note != "" {
		body += "\n\n" + note
	}
	return Letter{
		To:      sv.Email,
		CC:      settings.OutreachCC,
		Subject: fillLetter(settings.EmailSubject, settings, sv, senderName),
		Body:    tidyLetter(fillLetter(body, settings, sv, senderName)),
	}
}

func fillLetter(template string, settings Settings, sv StaffView, senderName string) string {
	lastYear := ""
	if sv.LastDonation != nil {
		lines := []string{"*Last Year's Charity*", sv.LastDonation.Charity}
		if sv.LastDonation.Note != "" {
			lines = append(lines, sv.LastDonation.Note)
		}
		lastYear = strings.Join(lines, "\n")
	}
	r := strings.NewReplacer(
		"{first name}", strings.Fields(sv.Name + " ")[0],
		"{name}", sv.Name,
		"{newsletter date}", mediumDate(sv.NewsletterDate),
		"{birthday}", monthDay(sv.BirthdayThisYear),
		"{default charity}", settings.DefaultCharity,
		"{sender}", senderName,
		"{last year}", lastYear,
	)
	return r.Replace(template)
}

var (
	trailingSpace = regexp.MustCompile(`[ \t]+(\r?\n|$)`)
	manyBlank     = regexp.MustCompile(`\n{3,}`)
)

func tidyLetter(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = trailingSpace.ReplaceAllString(s, "$1")
	s = manyBlank.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func monthDay(cell string) string {
	if t, err := ParseDate(cell); err == nil {
		return t.Format("January 2")
	}
	return cell
}
