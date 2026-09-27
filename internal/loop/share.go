package loop

import "heliosian/internal/sharecard"

// A link to Loop pasted into a chat app is fetched with no session, so what
// it previews comes from two public things: Open Graph tags slipped into the
// sign-in page, and a card at /open/share/about.png. A group's page is its
// members', so the preview says what Loop is and never what any group
// holds: the same card and the same words at every address.

// About is Loop's card, under its name and tagline as the registry has
// them: the standard palette and the horizontal lockup as drawn.
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
