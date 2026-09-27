package birthday

import "heliosian/internal/sharecard"

// A link to the app pasted into a chat app is fetched with no session, so
// what it previews comes from two public things: Open Graph tags slipped
// into the sign-in page, and a card at /open/share/about.png. The app is
// about the staff and who is looking after whom, none of which a stranger
// sees: the preview asks them to join the Birthday team, the same card and
// the same words at every address.

// About is the app's card, under its name and tagline as the registry has
// them: the standard palette and the horizontal lockup as drawn.
func About(name, tagline func() string) *sharecard.About {
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
