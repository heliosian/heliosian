package birthday

import (
	"image/color"
	"net/http"

	"heliosian/internal/sharecard"
)

// A link to the app pasted into a chat app is fetched with no session, so
// what it previews comes from two public things: Open Graph tags slipped
// into the sign-in page, and a card at /open/share/about.png drawn here. The
// app is about the staff and who is looking after whom, none of which a
// stranger sees: the preview asks them to join the Birthday team, the same
// card and the same words at every address.

// cardStyle is the app's dress for the card: the standard palette and the
// horizontal lockup as the designer drew it.
var cardStyle = &sharecard.Style{
	Page: color.RGBA{0xf4, 0xf8, 0xf8, 0xff}, Brand: color.RGBA{0x0f, 0x4e, 0x54, 0xff}, Accent: color.RGBA{0x1f, 0x83, 0x8a, 0xff},
	Ink: color.RGBA{0x0a, 0x32, 0x36, 0xff}, Yellow: color.RGBA{0xfa, 0xe1, 0x05, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe8, 0xff},
	Wordmark: "Helios Birthday", Tagline: "STAFF BIRTHDAY PROJECT MANAGEMENT",
	Lockup: "web/public/birthday/brand/logo-lockup.png",
}

// ShareTagline gives the card the app's tagline as the registry has it
// now, in place of the one written here.
func ShareTagline(now func() string) {
	cardStyle.TaglineNow = now
}

// The card's and the tags' words.
const (
	shareTitle = "Help celebrate our staff's birthdays"
	shareLead  = "Join the Birthday team."
	shareDesc  = "Every Helios staff member's birthday is marked with a donation to a charity they choose, announced in the school newsletter. The Birthday team makes it happen - a few birthdays each, a short note to ask, and the newsletter tells the school. Sign in with your school Google account to join."
)

// shareListing is the right half of the card: how the team works.
func shareListing() *sharecard.Listing {
	return &sharecard.Listing{Heading: "How it works", Items: []sharecard.Item{
		{Title: "Take a few birthdays", Note: "Each staff member gets a volunteer for the year"},
		{Title: "Ask them", Note: "A short note asks which charity they'd like"},
		{Title: "Give in their name", Note: "The association donates to the charity they chose"},
		{Title: "Tell the school", Note: "The newsletter carries the birthday and the gift"},
	}}
}

// PreviewHead is the Open Graph markup for any address on the app: the ask
// to join, and the card that makes it. Wired into the sign-in page, which is
// what an unauthenticated fetch gets.
func PreviewHead() func(r *http.Request) string {
	return func(r *http.Request) string {
		origin := "https://" + r.Host
		return sharecard.PreviewTags("Helios Birthday", shareTitle, shareDesc, origin+"/", origin+"/open/share/about.png")
	}
}

// shareCard serves /open/share/about.png: the app's card. Its ETag hashes
// the words, which change with the tagline alone.
func (a app) shareCard(w http.ResponseWriter, r *http.Request) {
	listing := shareListing()
	card := sharecard.Card{Title: shareTitle, Subtitle: shareLead, Button: "Join the team", Listing: listing}
	cardStyle.Serve(w, r, card, sharecard.ETag(append(listing.Words(), shareTitle, shareLead, cardStyle.TaglineText())...))
}
