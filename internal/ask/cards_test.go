package ask

import (
	"strings"
	"testing"
	"time"

	"heliosian/internal/model"
)

func TestClassroomChipsWearTheirColor(t *testing.T) {
	dir := sampleDir(t)
	if err := dir.Update(model.ConfigApp, "Classroom Colors", map[string]string{"Classroom": "Jays"}, map[string]string{"Color": "#1f6fb2"}); err != nil {
		t.Fatal(err)
	}
	if err := dir.Delete(model.ConfigApp, "Classroom Colors", map[string]string{"Classroom": "Hawks"}); err != nil {
		t.Fatal(err)
	}
	tr := sampleFrom(t, dir).turn(jordan)
	cards := map[string]linkCard{}
	for _, c := range items(call(t, tr, "get_classroom", `{}`)["classrooms"]) {
		card, ok := tr.linkCard(c["link"].(string))
		if !ok {
			t.Fatalf("no chip for %v", c)
		}
		cards[card.Name] = card
	}
	if jays := cards["Jays"]; jays.Color != "#1f6fb2" || jays.Image == "" || jays.Kind != "classroom" {
		t.Errorf("Jays: %+v", jays)
	}
	if hawks := cards["Hawks"]; hawks.Color != "" {
		t.Errorf("Hawks has no color set, but its chip has %q", hawks.Color)
	}
}

func TestLinkExamplesShowWhatTheChipsShow(t *testing.T) {
	tr := sampleTurn(t, jordan)
	tr.clock = func() time.Time { return time.Date(2026, 9, 18, 9, 0, 0, 0, model.Location) }
	out, err := tr.linkExamples()
	if err != nil {
		t.Fatal(err)
	}
	addresses, err := tr.exampleLinks()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, address := range addresses {
		card, ok := tr.linkCard(address)
		if !ok {
			t.Errorf("an example with no chip: %s", address)
			continue
		}
		kinds[card.Kind] = true
		if !strings.Contains(out, "["+card.Name+"]("+address+")") {
			t.Errorf("the examples lack %s", address)
		}
	}
	for _, kind := range []string{"person", "family", "classroom", "event", "activity", "group"} {
		if !kinds[kind] {
			t.Errorf("no %s among the examples:\n%s", kind, out)
		}
	}
	if !strings.Contains(out, `a badge reading "`) {
		t.Errorf("the event example does not show its day:\n%s", out)
	}
}

func TestLinkCardsAreWhatTheViewerMayRead(t *testing.T) {
	s := sampleSources(t)
	admin := s.turn(sampleAdmin)
	links := []string{}
	for _, c := range []struct{ tool, input, list string }{
		{"parties", `{"include_past":true}`, "parties"},
		{"volunteer_opportunities", `{"include_past":true,"limit":80}`, "things"},
		{"find_people", `{"queries":["whitfield"]}`, ""},
	} {
		result := call(t, admin, c.tool, c.input)
		list := result[c.list]
		if c.list == "" {
			list = items(result["results"])[0]["people"]
		}
		for _, item := range items(list) {
			links = append(links, item["link"].(string))
		}
	}
	stranger := s.turn("nobody@heliosschool.org")
	stranger.found = admin.found
	shown, hidden := 0, 0
	for _, link := range links {
		adminCard, ok := admin.linkCard(link)
		if !ok {
			t.Fatalf("the admin's own link has no chip: %s", link)
		}
		card, ok := stranger.linkCard(link)
		if ok {
			shown++
			if card != adminCard {
				t.Errorf("%s: %+v, the admin's %+v", link, card, adminCard)
			}
		} else {
			hidden++
		}
	}
	if shown == 0 || hidden == 0 {
		t.Fatalf("%d chips shown to a stranger, %d kept from them", shown, hidden)
	}
	if _, ok := admin.linkCard("https://who.heliosian.com/people/nobody.here"); ok {
		t.Fatal("a chip for a link no tool gave")
	}
}
