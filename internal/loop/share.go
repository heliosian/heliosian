package loop

import (
	"image/color"
	"net/http"

	"heliosian/internal/sharecard"
)

// A link to Loop pasted into a chat app is fetched with no session, so what
// it previews comes from two public things: Open Graph tags slipped into the
// sign-in page, and a card at /open/share/about.png drawn here. A group's
// page is its members', so the preview says what Loop is and never what any
// group holds: the same card and the same words at every address.

// cardStyle is the app's dress for the card: the standard palette and the
// horizontal lockup as the designer drew it.
var cardStyle = &sharecard.Style{
	Page: color.RGBA{0xf4, 0xf8, 0xf8, 0xff}, Brand: color.RGBA{0x0f, 0x4e, 0x54, 0xff}, Accent: color.RGBA{0x1f, 0x83, 0x8a, 0xff},
	Ink: color.RGBA{0x0a, 0x32, 0x36, 0xff}, Yellow: color.RGBA{0xfa, 0xe1, 0x05, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe8, 0xff},
	Wordmark: "Helios Loop", Tagline: "EMAIL LISTS FOR EVENTS & MORE",
	Lockup: "web/public/loop/brand/logo-lockup-horizontal.png",
}

// ShareTagline gives the card the app's tagline as the registry has it
// now, in place of the one written here.
func ShareTagline(now func() string) {
	cardStyle.TaglineNow = now
}

// The card's and the tags' words.
const (
	shareTitle = "One address reaches the whole group"
	shareLead  = "Email groups for the Helios community."
	shareDesc  = "Each group is an address at loop.heliosian.com - a class, a team, a committee, a party's guests - whose members follow from the directory, so it stays current as families come and go, and every message sent to it reaches them. Sign in with your school Google account."
)

// shareListing is the right half of the card: how a group works.
func shareListing() *sharecard.Listing {
	return &sharecard.Listing{Heading: "How it works", Items: []sharecard.Item{
		{Title: "Make a group", Note: "A class, a team, a committee, a party's guests"},
		{Title: "Members follow the directory", Note: "Rules pick who's on it, and it stays current"},
		{Title: "Write to one address", Note: "Everyone on the group gets it, and reply-all works"},
		{Title: "Leave any time", Note: "One click on any message"},
	}}
}

// PreviewHead is the Open Graph markup for any address on Loop: what it is,
// and the card that says so. Wired into the sign-in page, which is what an
// unauthenticated fetch gets.
func PreviewHead() func(r *http.Request) string {
	return func(r *http.Request) string {
		origin := "https://" + r.Host
		return sharecard.PreviewTags("Helios Loop", shareTitle, cardStyle.TaglineText()+". "+shareDesc, origin+"/", origin+"/open/share/about.png")
	}
}

// shareCard serves /open/share/about.png: Loop's card. Its ETag hashes the
// words, which change with the tagline alone.
func (a app) shareCard(w http.ResponseWriter, r *http.Request) {
	listing := shareListing()
	card := sharecard.Card{Title: shareTitle, Subtitle: shareLead, Button: "Open Loop", Listing: listing}
	cardStyle.Serve(w, r, card, sharecard.ETag(append(listing.Words(), shareTitle, shareLead, cardStyle.TaglineText())...))
}
