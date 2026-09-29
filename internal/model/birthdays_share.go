package model

import "heliosian/internal/sharecard"

func BirthdaysAbout(name, tagline func() string) *sharecard.About {
	return &sharecard.About{
		Style: &sharecard.Style{Palette: sharecard.Standard, Name: name, Tagline: tagline, Lockup: "web/public/birthday/brand/logo-lockup.png"},
		Desc:  "Every Helios staff member's birthday is marked with a donation to a charity they choose, announced in the school newsletter. The Birthday team makes it happen - a few birthdays each, a short note to ask, and the newsletter tells the school. Sign in with your school Google account to join.",
		Listing: sharecard.Listing{Heading: "How it works", Items: []sharecard.Item{
			{Title: "Take a few birthdays", Note: "Each staff member gets a volunteer for the year"},
			{Title: "Ask them", Note: "A short note asks which charity they'd like"},
			{Title: "Give in their name", Note: "The association donates to the charity they chose"},
			{Title: "Tell the school", Note: "The newsletter carries the birthday and the gift"},
		}},
		Button: "Join the team",
	}
}
