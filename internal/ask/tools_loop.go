package ask

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/who"
)

// groupCard is one email group as the tools answer it: its address and
// words, who runs it, whether the viewer is on it, and its members for a
// group the viewer manages.
type groupCard struct {
	Title       string   `json:"title"`
	Address     string   `json:"address"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description,omitempty"`
	Visibility  string   `json:"visibility"`
	Managers    []string `json:"managers"`
	Manage      bool     `json:"youManage,omitempty"`
	OnIt        bool     `json:"youAreOnIt,omitempty"`
	Members     int      `json:"members"`
	People      []string `json:"people,omitempty"`
	Link        string   `json:"link"`
}

var myGroups = tool{
	name:        "my_groups",
	description: "The email groups on Helios Loop the viewer can see: the ones they manage, with their members; the ones open to everyone; and the ones they are on whose managers opened them to their members. Each is an address whose members follow from rules over the directory.",
	words:       "Looking at the email groups",
	properties: map[string]any{
		"query": str("Words to find in a title, address or description."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Query string }](input)
		if err != nil {
			return nil, err
		}
		sources := v.sources.LoopSources()
		out := []groupCard{}
		for _, g := range v.loop.Groups {
			manages := g.Manages(v.email)
			members := loop.Members(g, sources)
			on := slices.Contains(members, v.email)
			if !g.VisibleTo(v.email, false, sources) {
				continue
			}
			if in.Query != "" && !contains(g.Title, in.Query) && !contains(g.Name, in.Query) && !contains(g.Description, in.Query) {
				continue
			}
			c := groupCard{
				Title: g.Title, Address: g.Address(), Aliases: g.Aliases, Description: g.Description, Visibility: g.Visibility, Managers: v.names(g.Managers),
				Manage: manages, OnIt: on, Members: len(members), Link: loopBase + g.Path(),
			}
			if manages {
				for _, m := range members {
					name := v.name(m)
					if added := g.Addition(m); added != nil && added.Name != "" {
						name = added.Name + " (outside the directory)"
					}
					c.People = append(c.People, name)
				}
			}
			out = append(out, c)
		}
		return map[string]any{"groups": out}, nil
	},
}

var myLists = tool{
	name:        "my_lists",
	description: "The viewer's own lists in Helios Who?: their tags, each with the people on it, and their Magic Tags - the lists their roles give them: the parties they host, the things they co-chair, the room parent lists, the groups they manage - each with its people.",
	words:       "Reading your lists",
	run: func(v *viewer, input json.RawMessage) (any, error) {
		tags := []map[string]any{}
		for name, people := range v.sources.Tags(v.email) {
			tags = append(tags, map[string]any{"name": name, "people": v.names(people)})
		}
		sort.Slice(tags, func(i, j int) bool { return tags[i]["name"].(string) < tags[j]["name"].(string) })
		lists := []map[string]any{}
		for _, l := range v.sources.Lists(v.email) {
			guests := []string{}
			for _, g := range l.Guests {
				guests = append(guests, g.Name)
			}
			lists = append(lists, map[string]any{"name": l.Name, "kind": listKind(l.Kind), "people": v.names(l.People), "guests": guests, "link": whoBase + who.ListPath(l.Key)})
		}
		return map[string]any{"tags": tags, "magicTags": lists}, nil
	},
}

var communityLinks = tool{
	name:        "community_links",
	description: "The links on Heliosian's front page, the community's page of everything else it uses: the school's own sites, the handbook, lunch ordering, chats and the like, by section.",
	words:       "Looking at Heliosian's links",
	properties: map[string]any{
		"query": str("Words to find in a link's title or description."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Query string }](input)
		if err != nil {
			return nil, err
		}
		sections := []map[string]any{}
		for _, category := range v.sources.Links() {
			if category.Style == home.StyleEvents || category.Style == home.StyleApps {
				continue
			}
			links := []map[string]string{}
			for _, l := range category.Links {
				if !l.Visible {
					continue
				}
				if in.Query != "" && !contains(l.Title, in.Query) && !contains(l.Description, in.Query) {
					continue
				}
				links = append(links, map[string]string{"title": l.Title, "description": l.Description, "url": l.URL})
			}
			if len(links) > 0 {
				sections = append(sections, map[string]any{"section": category.Title, "links": links})
			}
		}
		if len(sections) == 0 {
			return nil, fmt.Errorf("no link matches that; Heliosian's front page is https://heliosian.com")
		}
		return map[string]any{"sections": sections}, nil
	},
}
