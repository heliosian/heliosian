package db

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestClassifyRequestListsEventsInOrderWithTheirHandles(t *testing.T) {
	items := []CalendarItem{}
	for i := range 11 {
		items = append(items, CalendarItem{Key: "k" + strconv.Itoa(i), Title: "Event " + strconv.Itoa(i)})
	}
	request, sent, err := classifyRequest(items)
	if err != nil {
		t.Fatal(err)
	}
	var listed []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(request[strings.Index(request, "["):]), &listed); err != nil {
		t.Fatalf("the request is not a list: %v", err)
	}
	for i, l := range listed {
		handle := "e" + strconv.Itoa(i+1)
		if l.ID != handle || l.Title != items[i].Title || sent[handle].Key != items[i].Key {
			t.Errorf("entry %d is %+v, sent as %+v, want %s with %q", i, l, sent[handle], handle, items[i].Title)
		}
	}
	if len(listed) != len(items) {
		t.Fatalf("%d listed, want %d", len(listed), len(items))
	}
}
