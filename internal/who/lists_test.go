package who

import (
	"slices"
	"testing"
)

func TestRoomParentsOf(t *testing.T) {
	m := &Model{RoomParents: map[string][]string{
		"K":         {"kay@x.org"},
		"1st / 2nd": {"one@x.org"},
		"3rd / 4th": {"three@x.org", "four@x.org"},
	}}
	for _, tc := range []struct {
		band string
		want []string
	}{
		{"Hummingbirds", []string{"kay@x.org"}},
		{"Halcons", []string{"one@x.org"}},
		{"Jayvens", []string{"three@x.org", "four@x.org"}},
		{"Hegrets", nil},
		{"", nil},
	} {
		if got := m.RoomParentsOf(tc.band); !slices.Equal(got, tc.want) {
			t.Errorf("RoomParentsOf(%q) = %v, want %v", tc.band, got, tc.want)
		}
	}
}
