package who

import "heliosian/internal/sharecard"

// A link to the directory pasted into a chat app is fetched with no session,
// so what it previews comes from two public things: Open Graph tags slipped
// into the sign-in page, and a card at /open/share/about.png. The directory
// is people, so the preview says what the directory is and never who is in
// it: the same card and the same words at every address, with no name,
// photo, count or classroom from the model.

// About is the directory's card, under its name and tagline as the registry
// has them: the standard palette and the horizontal lockup as drawn.
func About(name, tagline func() string) *sharecard.About {
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
