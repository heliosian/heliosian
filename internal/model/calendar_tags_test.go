package model

import (
	"context"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
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

	err := sources.directory.Commit(context.Background(), access.System("test"), DirectoryApp,
		store.Update(tagListTable, store.Row{tagID: carpool}, store.Row{tagName: "Rideshare"}),
		store.Insert(tagsTable, store.Row{tagID: carpool, tagPerson: mia}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if g := cache.Model().Calendar.GroupOf(meetup, made.Group); g == nil || len(g.Rule.Tags) != 1 || g.Rule.Tags[0] != key {
		t.Fatalf("the group's rule after the rename: %+v", g)
	}
	fillNow(t)
	if r := rowOf(inviteView(t, jordan, meetup), mia); r == nil || r.Via != ViaGroup+made.Group {
		t.Fatalf("someone tagged on the renamed tag did not come on through the group: %+v", r)
	}
}
