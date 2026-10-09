package db

import (
	"testing"
	"time"

	"heliosian/internal/geocode"
	"heliosian/internal/intercept"
	"heliosian/internal/store"
)

func TestTheGeocoderPlacesAFamilysSharedAddressOnce(t *testing.T) {
	intercept.Install(intercept.GeocodeHost, intercept.Geocode())
	s, queue := sampleWithQueue(t)
	const address = "3 Tide Pool Court, Montara, CA 94037"
	if err := commit(s, GroupsSheet,
		store.Insert("GROUP", store.Row{"id": "grp00000000508", "kind": "family", "status": "open", "name": "Okafor Family", "visible_to": "grp00000000004", "vc_address": address, "address_consent": "shared", "consent": "listed"}),
		store.Insert("GROUP", store.Row{"id": "grp00000000509", "kind": "family", "status": "open", "name": "Brandt Family", "visible_to": "grp00000000004", "vc_address": "9 Hidden Lane, Montara, CA 94037", "address_consent": "withheld", "consent": "listed"}),
		store.Insert("GROUP", store.Row{"id": "grp00000000540", "kind": "family", "status": "open", "name": "Vega Family", "visible_to": "grp00000000004", "vc_address": "Half Moon Bay, CA", "address_consent": "shared", "consent": "listed"}),
	); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, ConfigSheet, store.Insert("GEOCODE", store.Row{"id": "geo00000000009", "address": "Half Moon Bay, CA", "lat": "37.46", "lng": "-122.43"})); err != nil {
		t.Fatal(err)
	}
	if got := s.Model().unplaced(); len(got) != 1 || got[0] != address {
		t.Fatalf("unplaced: %v", got)
	}
	StartGeocoder(s, queue, newPictures(s, queue), geocode.New("test"))
	for deadline := time.Now().Add(5 * time.Second); len(s.Model().unplaced()) > 0 || len(s.Model().unplaceable()) > 0; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("still unplaced: %v, still held though city-only: %v", s.Model().unplaced(), s.Model().unplaceable())
		}
	}
	if _, ok := s.Model().Table("GEOCODE").Get("geo00000000009"); ok {
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
