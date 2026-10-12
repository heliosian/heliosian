package db

import (
	"testing"
	"time"

	"heliosian/internal/geocode"
	"heliosian/internal/intercept"
)

func TestTheGeocoderPlacesAFamilysSharedAddressOnce(t *testing.T) {
	intercept.Install(intercept.GeocodeHost, intercept.Geocode())
	s, queue := sampleWithQueue(t)
	const address = "3 Tide Pool Court, Montara, CA 94037"
	apply(t, s, queue, []Edit{
		{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "family", "status": "open", "name": "Okafor Family", "visible_to": "grp00000000004", "vc_address": address, "address_consent": "shared", "consent": "listed"}},
		{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "family", "status": "open", "name": "Brandt Family", "visible_to": "grp00000000004", "vc_address": "9 Hidden Lane, Montara, CA 94037", "address_consent": "withheld", "consent": "listed"}},
		{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "family", "status": "open", "name": "Vega Family", "visible_to": "grp00000000004", "vc_address": "Half Moon Bay, CA", "address_consent": "shared", "consent": "listed"}},
	})
	cityOnly := apply(t, s, queue, []Edit{{Insert: "GEOCODE", Row: map[string]any{"address": "Half Moon Bay, CA", "lat": "37.46", "lng": "-122.43"}}})[0]
	if got := s.Model().unplaced(); len(got) != 1 || got[0] != address {
		t.Fatalf("unplaced: %v", got)
	}
	StartGeocoder(s, queue, geocode.New("test"))
	for deadline := time.Now().Add(5 * time.Second); len(s.Model().unplaced()) > 0 || len(s.Model().unplaceable()) > 0; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("still unplaced: %v, still held though city-only: %v", s.Model().unplaced(), s.Model().unplaceable())
		}
	}
	if _, ok := s.Model().Table("GEOCODE").Get(cityOnly); ok {
		t.Fatal("the city-only pin was kept")
	}
	placed := 0
	for _, row := range s.Model().Table("GEOCODE").All() {
		if row["address"] == address {
			placed++
			if row["lat"] == "" || row["lng"] == "" {
				t.Fatalf("placed without coordinates: %v", row)
			}
		}
	}
	if placed != 1 {
		t.Fatalf("placed %d times", placed)
	}
}
