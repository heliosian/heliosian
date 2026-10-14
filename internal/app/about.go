package app

import "heliosian/internal/sharecard"

func directoryAbout(name, tagline func() string) *sharecard.About {
	return &sharecard.About{
		Style: &sharecard.Style{Palette: sharecard.Standard, Name: name, Tagline: tagline, Lockup: "web/public/who/brand/logo-lockup.png"},
		Desc:  "Students, parents and staff, browsable by person, family, classroom and grade - photos, pronunciations, room parents and the lists families keep together. Sign in with your school Google account.",
		Listing: sharecard.Listing{Heading: "What's inside", Items: []sharecard.Item{
			{Title: "People", Note: "Students, parents and staff, with photos"},
			{Title: "Families", Note: "Who belongs with whom, and how to reach them"},
			{Title: "Classrooms", Note: "Each class, its crews, staff and parents"},
			{Title: "Lists", Note: "Carpool, room parents and the tags families share"},
		}},
		Button: "Open Who?",
	}
}

func listAbout(name, tagline func() string) *sharecard.About {
	return &sharecard.About{
		Style: &sharecard.Style{Palette: sharecard.Standard, Name: name, Tagline: tagline, Lockup: "web/public/loop/brand/logo-lockup-horizontal.png"},
		Desc:  "Each email list is an address at loop.heliosian.com - a class, a team, a committee, a party's guests - whose members follow from the directory, so it stays current as families come and go, and every message sent to it reaches them. Sign in with your school Google account.",
		Listing: sharecard.Listing{Heading: "How it works", Items: []sharecard.Item{
			{Title: "Make an email list", Note: "A class, a team, a committee, a party's guests"},
			{Title: "Members follow the directory", Note: "Rules pick who's on it, and it stays current"},
			{Title: "Write to one address", Note: "Everyone on the email list gets it, and reply-all works"},
			{Title: "Leave any time", Note: "One click on any message"},
		}},
		Button: "Open Loop",
	}
}
