package ask

import (
	"testing"

	"heliosian/internal/celebrate"
	"heliosian/internal/team"
)

func TestLinkCardsKeepEachAppsVisibility(t *testing.T) {
	for _, email := range []string{jordan, "ruth.amari@heliosschool.org"} {
		v := sampleViewer(t, email)
		if card, ok := v.linkCard(whoLink("sam.whitfield@heliosschool.org")); !ok || card.Kind != "person" || card.Name != "Sam Whitfield" {
			t.Errorf("%s: Sam's card %+v %v", email, card, ok)
		}
		if _, ok := v.linkCard(whoLink("nobody.here@heliosschool.org")); ok {
			t.Errorf("%s: a card for someone the directory does not list", email)
		}
		for key, family := range v.directory.Families {
			if card, ok := v.linkCard(whoBase + "/families/" + key); !ok || card.Kind != "family" || card.Name != family.Name || card.Image != family.PhotoURL {
				t.Errorf("%s: family %s card %+v %v", email, key, card, ok)
			}
		}
		if _, ok := v.linkCard(whoBase + "/families/no-such-family"); ok {
			t.Errorf("%s: a card for a family the directory does not list", email)
		}
		hidden, shown := 0, 0
		for _, p := range v.celebrate.Parties {
			_, ok := v.linkCard(celebrateBase + v.celebrate.PathOf(p))
			if ok != (p.Status == celebrate.StatusOpen || p.Hosted(email)) {
				t.Errorf("%s: party %q (%s) card %v", email, p.Title, p.Status, ok)
			}
			if ok {
				shown++
			} else {
				hidden++
			}
		}
		for _, a := range v.team.Activities {
			_, ok := v.linkCard(teamBase + v.team.PathOf(a))
			if ok != v.visibleActivity(a) {
				t.Errorf("%s: activity %q (%s) card %v", email, a.Title, a.Status, ok)
			}
			if a.Status == team.StatusHidden && !v.team.Runs(a, email) && ok {
				t.Errorf("%s: a hidden activity %q has a card", email, a.Title)
			}
		}
		if shown == 0 {
			t.Errorf("%s: no party has a card", email)
		}
		t.Logf("%s: %d parties with cards, %d without", email, shown, hidden)
	}
}
