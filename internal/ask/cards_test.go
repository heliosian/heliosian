package ask

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestClassroomChipsWearTheirColor(t *testing.T) {
	tr := sampleTurn(t, rowan)
	if _, err := tr.run(context.Background(), "helios_classroom", json.RawMessage(`{"name": "Hummingbirds"}`)); err != nil {
		t.Fatal(err)
	}
	card, ok := tr.linkCard("https://who.heliosian.com/classrooms/grp00000000010")
	if !ok || card.Kind != "classroom" || card.Color != "#5b8def" || card.Name != "Hummingbirds" {
		t.Fatalf("Hummingbirds: %+v %v", card, ok)
	}
	crew, ok := tr.linkCard("https://who.heliosian.com/people/per00000000003")
	if !ok || crew.Kind != "person" || crew.Name != "Maya Lindqvist" || crew.Color != "" {
		t.Fatalf("Maya: %+v %v", crew, ok)
	}
}

func TestLinkExamplesShowWhatTheChipsShow(t *testing.T) {
	tr := sampleTurn(t, rowan)
	s, v := promptParts(t, tr)
	out, err := tr.linkExamples(s, v)
	if err != nil {
		t.Fatal(err)
	}
	addresses, err := tr.exampleLinks(s, v)
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
	for _, kind := range []string{"person", "family", "classroom", "event", "activity", "party", "group"} {
		if !kinds[kind] {
			t.Errorf("no %s among the examples:\n%s", kind, out)
		}
	}
	if !strings.Contains(out, `a badge reading "Sat Oct 10"`) {
		t.Errorf("the event example does not show its day:\n%s", out)
	}
}

func TestLinkCardsAreWhatTheViewerMayRead(t *testing.T) {
	s := sampleSources(t)
	parent := s.turn(rowan)
	if _, err := parent.run(context.Background(), "helios_whoami", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	family := "https://who.heliosian.com/families/grp00000000020"
	if _, ok := parent.linkCard(family); !ok {
		t.Fatal("the parent's own family has no chip")
	}
	outsider := s.turn(stranger)
	outsider.found = parent.found
	if card, ok := outsider.linkCard(family); ok {
		t.Fatalf("someone outside the directory was shown %+v", card)
	}
	if _, ok := parent.linkCard("https://who.heliosian.com/people/per99999999999"); ok {
		t.Fatal("a chip for a link no tool gave")
	}
}
