package ask

import (
	"encoding/json"
	"fmt"
)

var myGroups = tool{
	name:        "my_groups",
	description: "The email lists on Helios Loop the viewer can see, each with its members: the ones they manage, the ones open to everyone, and the ones they are on whose managers opened them to their members. Each is an address whose members follow from rules over the directory.",
	words:       "Looking at the email lists",
	properties: map[string]any{
		"query": str("Words to find in a title, address or description."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Query string }](input)
		if err != nil {
			return nil, err
		}
		spec := "title,address,aliases,description,visibility,managers.fullName,memberCount,me.managing,me.member,members.name,members.person.fullName,link"
		return t.ask(query{name: "groups", path: collection("email-lists", params("q", in.Query, "include", "managers,members.person")), fields: fields(spec)})
	},
}

var myLists = tool{
	name:        "my_lists",
	description: "The viewer's own lists in Helios Who?: their tags, each with the people on it, and their Magic Tags - the lists their roles give them: the parties they host, the things they co-chair, the room parent lists, the email lists they manage - each with its people.",
	words:       "Reading your lists",
	run: func(t *turn, input json.RawMessage) (any, error) {
		return t.ask(
			query{name: "tags", path: collection("tags", params("mine", "true", "include", "people")), fields: fields("name,people.fullName,link")},
			query{name: "magicTags", path: collection("magic-tags", params("include", "people")), fields: fields("name,kind,archived,people.fullName,guests.name,link")},
		)
	},
}

var communityLinks = tool{
	name:        "community_links",
	description: "The links on Heliosian's front page, the community's page of everything else it uses: the school's own sites, the handbook, lunch ordering, chats and the like, each with its section.",
	words:       "Looking at Heliosian's links",
	properties: map[string]any{
		"query": str("Words to find in a link's title or description."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Query string }](input)
		if err != nil {
			return nil, err
		}
		out, err := t.ask(query{name: "links", path: collection("links", params("q", in.Query, "include", "category")), fields: fields("title,description,url,visible,forMe,category.title")})
		if err != nil {
			return nil, err
		}
		if empty(out["links"]) {
			return nil, fmt.Errorf("no link matches that; Heliosian's front page is https://heliosian.com")
		}
		return out, nil
	},
}
