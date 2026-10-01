package ask

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	activityIncludes = "category,volunteers"
	activityFields   = "id,title,under,category.title,status,timing,day,dayTiming,start,end,past,location,description~400,spots,taken,full,coLeaderNeeded,volunteersHidden,wants,me.position,volunteers.name,volunteers.position,volunteers.note,link"
)

var volunteerOpportunities = tool{
	name:        "volunteer_opportunities",
	description: "What the HCA runs on HCA-Team and who has signed up: every event with the committees, roles and shifts under it, each with its spots, who is on it as the portal shows this viewer, and the viewer's own position as me.position. This school year unless another is named; what has already happened is left out unless asked for.",
	words:       "Looking at HCA-Team",
	properties: map[string]any{
		"query":        str("Words to find in a title or description."),
		"year":         str("A school year like 2026 - 2027; the current one unless said."),
		"include_past": boolean("Also what has already happened or is done."),
		"limit":        integer("How many things to return, 40 unless said, 80 at most."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			Query, Year string
			IncludePast bool `json:"include_past"`
			Limit       int
		}](input)
		if err != nil {
			return nil, err
		}
		year := in.Year
		if year == "" {
			year = "current"
		}
		past := "false"
		if in.IncludePast {
			past = ""
		}
		limit := limitOf(in.Limit, 40, 80)
		v := params("year", year, "past", past, "q", in.Query, "include", activityIncludes, "limit", shown(limit))
		out, err := t.ask(
			query{name: "things", path: collection("activities", v), fields: fields(activityFields), limit: limit},
			query{name: "settings", path: collection("team-settings", params()), fields: fields("settings.expenseFormUrl")},
		)
		if err != nil {
			return nil, err
		}
		out["expenseForm"] = first(out["settings"])["settings"]
		delete(out, "settings")
		return out, nil
	},
}

var getActivity = tool{
	name:        "get_activity",
	description: "One thing on HCA-Team in full, by its id or by its HCA-Team link as another tool gave it: the thing, its highlight, its links, and everything under it.",
	words:       "Reading an HCA-Team page",
	properties: map[string]any{
		"id":   str("The thing's id, from volunteer_opportunities, or the pretty name of an event like international-night."),
		"path": str("Its HCA-Team link as another tool gave it."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ ID, Path string }](input)
		if err != nil {
			return nil, err
		}
		key := in.ID
		if key == "" && in.Path != "" {
			r, ok := t.found.of(strings.TrimSpace(in.Path))
			if !ok || r.typ != "activities" {
				return nil, fmt.Errorf("that is not an HCA-Team link another tool gave; use the thing's id")
			}
			key = r.id
		}
		if key == "" {
			return nil, fmt.Errorf("say which thing")
		}
		env, err := t.read(map[string]string{"thing": one("activities", key, params())})
		if err != nil {
			return nil, err
		}
		id := env.Result.(map[string]any)["thing"].(string)
		spec := strings.Replace(activityFields, "description~400", "description~3000", 1) + ",highlight.headline,highlight.body,links.title,links.url,links.description"
		return t.ask(
			query{name: "thing", path: one("activities", id, params("include", activityIncludes+",links")), fields: fields(spec)},
			query{name: "under", path: collection("activities", params("under", id, "include", activityIncludes)), fields: fields(activityFields)},
		)
	},
}
