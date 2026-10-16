package model

import (
	"time"

	"heliosian/internal/access"
	"heliosian/internal/sharecard"
)

func partyPreviewable(p *Party) bool {
	return p != nil && p.VisibleTo(access.Actor{})
}

func partyWhenLines(p *Party) (string, string) {
	start, err := time.ParseInLocation(DateTimeFormat, p.Start, Location)
	if err != nil {
		if day, err := time.ParseInLocation(DateFormat, p.Start, Location); err == nil {
			return day.Format("Monday, January 2"), ""
		}
		return "", ""
	}
	day := start.Format("Monday, January 2")
	end, err := time.ParseInLocation(DateTimeFormat, p.End, Location)
	if err != nil {
		return day, start.Format("3:04 PM")
	}
	return day, sharecard.Hours(start, end)
}

func whenLine(p *Party) string {
	day, hours := partyWhenLines(p)
	if hours != "" {
		return day + " · " + hours
	}
	return day
}
