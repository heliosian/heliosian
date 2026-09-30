package model

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

func TestGroupListsCarryAdditionsAsGuests(t *testing.T) {
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	groups := sampleStore(t, dir, store.NewQueue(), sampleDeps(sampleKey))
	directory := groups.Model().Directory
	sources := groups.Model().Audience(now())
	lists := groups.Model().EmailLists.MagicTags(sources, jordan)
	i := slices.IndexFunc(lists, func(l MagicTag) bool { return l.Key == "group:"+soccerID })
	if i < 0 {
		t.Fatalf("no soccer team list: %+v", lists)
	}
	list := lists[i]
	if list.Slug != "soccer-team" {
		t.Fatalf("the list's slug is %q, not the group's name", list.Slug)
	}
	coach := "coach.rivera@coastsidesoccer.example.org"
	if slices.Contains(list.People, coach) || slices.Contains(list.People, jordan) {
		t.Fatalf("people: %v", list.People)
	}
	want := []Guest{
		{ID: soccerID + ":" + coach, Name: "Coach Rivera", Email: coach},
		{ID: soccerID + ":office@coastsidesoccer.example.org", Name: "Coastside League Office", Email: "office@coastsidesoccer.example.org"},
	}
	if !slices.Equal(list.Guests, want) {
		t.Fatalf("guests: %+v", list.Guests)
	}
	for _, p := range list.People {
		if directory.Person(p) == nil {
			t.Errorf("%s is listed as a person but is not in the directory", p)
		}
	}
	if list.Archived {
		t.Fatal("a group nobody archived came marked archived")
	}
	if err := groups.Commit(context.Background(), access.Actor{Email: jordan}, emailListsAppName, store.Upsert("Archived", store.Row{"Group": soccerID, "Email": jordan}, store.Row{})); err != nil {
		t.Fatal(err)
	}
	lists = groups.Model().EmailLists.MagicTags(groups.Model().Audience(now()), jordan)
	i = slices.IndexFunc(lists, func(l MagicTag) bool { return l.Key == "group:"+soccerID })
	if i < 0 || !lists[i].Archived {
		t.Fatalf("the archived group's list is missing or unmarked: %+v", lists)
	}
}

func TestMagicTagsAreTheirOwnResource(t *testing.T) {
	h := newHarness(t)
	m := h.store.Model()
	want := []string{}
	for _, tag := range m.MagicTagsOf(jordan, now()) {
		want = append(want, m.magicTagID(tag.Key))
	}
	if got := h.get(jordan, "/api/magic-tags").ids(t); len(want) == 0 || !slices.Equal(got, want) {
		t.Fatalf("the listed Magic Tags %v are not the ones jordan holds %v", got, want)
	}
	soccer := m.magicTagID("group:" + soccerID)
	out := h.get(jordan, "/api/magic-tags/group:"+soccerID+"?include=email-list,holders")
	if got := out.id(t); got != soccer {
		t.Fatalf("the key did not resolve: %s", got)
	}
	tag := out.Resources["magic-tags"][soccer]
	if tag["key"] != "group:"+soccerID || tag["kind"] != MagicTagGroup || tag["email-list"] != soccerID {
		t.Fatalf("the soccer list's Magic Tag: %v", tag)
	}
	if holders := strings2(tag["holders"]); !slices.Contains(holders, m.personID(jordan)[0]) {
		t.Fatalf("holders %v leave out its manager", holders)
	}
	h.want("ruth.amari@heliosschool.org", http.MethodGet, "/api/magic-tags/group:"+soccerID, "", http.StatusNotFound)
}

func TestRulesNameAnEmailListsMagicTag(t *testing.T) {
	h := newHarness(t)
	rec := h.as(jordan, http.MethodPost, "/api/email-lists", `{"name":"soccer-friends","title":"Soccer friends","managers":["`+jordan+`"],"rules":[{"kind":"include","tags":["group:`+soccerID+`"]}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("a list naming another: %d %s", rec.Code, rec.Body)
	}
	d := h.store.Model().Directory
	inside := []string{}
	for _, email := range h.members("soccer-team") {
		if d.Person(email) != nil {
			inside = append(inside, email)
		}
	}
	if got := h.members("soccer-friends"); len(inside) == 0 || !slices.Equal(got, inside) {
		t.Fatalf("soccer friends %v, want the soccer team's directory members %v", got, inside)
	}
	friends := h.store.Model().EmailLists.Named("soccer-friends").ID
	h.want(jordan, http.MethodPost, "/api/email-lists/"+soccerID+"/edit", `{"rules":[{"kind":"include","tags":["group:`+friends+`"]}]}`, http.StatusBadRequest)
	h.want(jordan, http.MethodPost, "/api/email-lists/"+soccerID+"/edit", `{"rules":[{"kind":"include","tags":["group:`+soccerID+`"]}]}`, http.StatusBadRequest)
}

func TestNamingFindsTheListsThatCarryAMagicTag(t *testing.T) {
	m := &EmailLists{Groups: []EmailList{{Name: "soccer", Rules: []Rule{{Kind: RuleInclude, Tags: []string{"room:jays"}}}}}}
	if got := m.naming("activity:act0000000023"); len(got) != 0 {
		t.Errorf("decor has a list already: %v", got)
	}
	m.Groups = append(m.Groups, EmailList{Name: "decor", Rules: []Rule{{Kind: RuleInclude, Tags: []string{"activity:act0000000023"}}}})
	if got := m.naming("activity:act0000000023"); len(got) != 1 || got[0].Name != "decor" {
		t.Errorf("decor's list: %v", got)
	}
}
