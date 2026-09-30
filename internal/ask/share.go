package ask

import "heliosian/internal/sharecard"

func About(name, tagline func() string) *sharecard.About {
	return &sharecard.About{
		Style: &sharecard.Style{Palette: sharecard.Standard, Name: name, Tagline: tagline, Lockup: "web/public/ask/brand/logo-lockup-horizontal.png"},
		Desc:  "Ask anything about Helios - the newsletters, the calendar, the programs and how the school works - and get an answer drawn from the school's own pages and mail. Sign in with your school Google account.",
		Listing: sharecard.Listing{Heading: "Try asking", Items: []sharecard.Item{
			{Title: "What's in the latest school newsletter?"},
			{Title: "What sports are offered in middle school?"},
			{Title: "When is the book fair?"},
			{Title: "Can parents attend community meeting?"},
		}},
		Button: "Open Ask",
	}
}
