package loop

import (
	"image/color"
	"net/http"
	"strings"

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

// ShareWords gives the card the app's name and tagline as the registry has
// them now - the big words and the line under them - in place of the ones
// written here.
func ShareWords(name, tagline func() string) {
	shareName = name
	cardStyle.TaglineNow = tagline
}

// shareName is the app's name as the registry has it, the card's title.
var shareName func() string

// title is the card's big words: the app's name from the registry, else
// the wordmark written here.
func title() string {
	if shareName != nil {
		if now := strings.TrimSpace(shareName()); now != "" {
			return now
		}
	}
	return cardStyle.Wordmark
}

// shareDesc is the tags' longer words, after the tagline.
const shareDesc = "Each group is an address at loop.heliosian.com - a class, a team, a committee, a party's guests - whose members follow from the directory, so it stays current as families come and go, and every message sent to it reaches them. Sign in with your school Google account."

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
		return sharecard.PreviewTags(title(), title(), cardStyle.TaglineText()+". "+shareDesc, origin+"/", origin+"/open/share/about.png")
	}
}

// shareCard serves /open/share/about.png: Loop's card - its name over its
// tagline, as the registry has them. Its ETag hashes the words.
func (a app) shareCard(w http.ResponseWriter, r *http.Request) {
	listing := shareListing()
	card := sharecard.Card{Title: title(), Subtitle: cardStyle.TaglineText(), Button: "Open Loop", Listing: listing}
	cardStyle.Serve(w, r, card, sharecard.ETag(append(listing.Words(), title(), cardStyle.TaglineText())...))
}
