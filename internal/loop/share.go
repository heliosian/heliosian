package loop

import "heliosian/internal/sharecard"

func About(name, tagline func() string) *sharecard.About {
	return &sharecard.About{
		Style: &sharecard.Style{Palette: sharecard.Standard, Name: name, Tagline: tagline, Lockup: "web/public/loop/brand/logo-lockup-horizontal.png"},
		Desc:  "Each group is an address at loop.heliosian.com - a class, a team, a committee, a party's guests - whose members follow from the directory, so it stays current as families come and go, and every message sent to it reaches them. Sign in with your school Google account.",
		Listing: sharecard.Listing{Heading: "How it works", Items: []sharecard.Item{
			{Title: "Make a group", Note: "A class, a team, a committee, a party's guests"},
			{Title: "Members follow the directory", Note: "Rules pick who's on it, and it stays current"},
			{Title: "Write to one address", Note: "Everyone on the group gets it, and reply-all works"},
			{Title: "Leave any time", Note: "One click on any message"},
		}},
		Button: "Open Loop",
	}
}
