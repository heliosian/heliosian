package model

import (
	"net/http"
	"net/url"
	"testing"

	"heliosian/internal/testkit"
)

func TestRenamingATagKeepsTheInviteGroupsThatNameIt(t *testing.T) {
	mux, cache, _, sources := invitesAppWith(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	key := carpoolKey
	rec := call(t, jordan, "POST", "/api/invite-groups", `{"id":"meetup","rule":{"tags":["`+key+`"]}}`)
	made := struct{ Group string }{created(t, rec)}
	if rec.Code != 200 || len(cache.Model().Calendar.Invites[meetup]) != 2 {
		t.Fatalf("group: %d %s", rec.Code, rec.Body)
	}

	tags := http.NewServeMux()
	RegisterDirectory(tags, DirectoryRoutes{Store: sources.directory})
	if rec := testkit.Form(t, tags, host, "/api/directory/tag-rename", url.Values{"tag": {carpool}, "name": {"Rideshare"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Form(t, tags, host, "/api/directory/tag", url.Values{"tag": {carpool}, "person": {mia}, "on": {"1"}}); rec.Code != http.StatusOK {
		t.Fatalf("tag mia: %d %s", rec.Code, rec.Body)
	}

	if g := cache.Model().Calendar.GroupOf(meetup, made.Group); g == nil || len(g.Rule.Tags) != 1 || g.Rule.Tags[0] != key {
		t.Fatalf("the group's rule after the rename: %+v", g)
	}
	fillNow(t)
	if r := rowOf(inviteView(t, jordan, meetup), mia); r == nil || r.Via != ViaGroup+made.Group {
		t.Fatalf("someone tagged on the renamed tag did not come on through the group: %+v", r)
	}
}
