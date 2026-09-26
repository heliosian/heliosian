package app

import (
	"testing"

	"heliosian/internal/artifacts"
)

// TestForClassrooms checks which school lists reach a Jays family: the
// newsletter and every family's lists, the Jays' own lists and the grade band
// that holds them, and no other classroom's.
func TestForClassrooms(t *testing.T) {
	mine := []string{"jays"}
	for channel, want := range map[string]bool{
		"parentsandstaff": true, "community": true, "jays.parents": true, "jays.students": true,
		"jaysandravens": true, "ravens.parents": false, "hawksandfalcons": false, "condorsandospreys": false,
	} {
		d := &artifacts.Document{Kind: artifacts.KindList, Channel: channel}
		if got := forClassrooms(d, mine); got != want {
			t.Errorf("%s: %v, want %v", channel, got, want)
		}
	}
	if !forClassrooms(&artifacts.Document{Kind: artifacts.KindNewsletter, Channel: "newsletter"}, nil) {
		t.Error("the newsletter is everyone's")
	}
	if artifacts.School(&artifacts.Document{Kind: artifacts.KindList, Channel: "chat"}) || artifacts.School(&artifacts.Document{Kind: artifacts.KindGroup, Channel: "x"}) {
		t.Error("the chat and a group's post are not school mail")
	}
}
