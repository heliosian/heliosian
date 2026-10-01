package ask

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"heliosian/internal/access"
)

const nearbyFields = "name,address,link,adults.fullName,kids.fullName,kids.grade.name,kids.classroom.name"

func (t *turn) ownFamilies() ([]string, error) {
	email := t.reg.Actor(t.r).Email
	env, err := t.read(map[string]string{"me": one("people", email, params("include", "families"))})
	var refusal *access.Refusal
	if errors.As(err, &refusal) && refusal.Status == http.StatusNotFound {
		return nil, fmt.Errorf("the directory does not list you, so there is no family to measure from")
	}
	if err != nil {
		return nil, err
	}
	key := env.Result.(map[string]any)["me"].(string)
	var families []string
	if err := json.Unmarshal(env.Resources["people"][key]["families"], &families); err != nil {
		return nil, err
	}
	return families, nil
}

var nearbyFamilies = tool{
	name:        "nearby_families",
	description: "Families who live near a family - the viewer's own unless another is named - nearest first, by straight-line distance between the street addresses families share in Helios Who?, as its map shows them: for carpools, walking groups and playdates. Each comes with milesAway, its address as shared, its adults and its students' grades and classrooms. Narrow them to a classroom, a grade or a distance. A family that shares no street address is never listed.",
	words:       "Looking for families nearby",
	properties: map[string]any{
		"name":         str("Words of a member's name or the family's name, to measure from a family other than the viewer's."),
		"classroom":    str("Only families with a student in this classroom."),
		"grade":        str("Only families with a student in this grade, as Grade 3 or Kindergarten."),
		"within_miles": map[string]any{"type": "number", "description": "Only families within this many miles."},
		"limit":        integer("How many families to return, 10 unless said, 30 at most."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			Name, Classroom, Grade string
			WithinMiles            float64 `json:"within_miles"`
			Limit                  int
		}](input)
		if err != nil {
			return nil, err
		}
		candidates := []string{}
		if in.Name != "" {
			env, err := t.read(map[string]string{"families": collection("families", params("q", in.Name))})
			if err != nil {
				return nil, err
			}
			candidates = env.Result.(map[string]any)["families"].([]string)
		} else if candidates, err = t.ownFamilies(); err != nil {
			return nil, err
		}
		if len(candidates) == 0 {
			return nil, fmt.Errorf("no family in the directory matches that")
		}
		within := ""
		if in.WithinMiles > 0 {
			within = strconv.FormatFloat(in.WithinMiles, 'f', -1, 64)
		}
		limit := limitOf(in.Limit, 10, 30)
		for _, from := range candidates {
			v := params("near", from, "within", within, "classroom", in.Classroom, "grade", in.Grade, "include", familyIncludes, "limit", shown(limit))
			out, err := t.ask(
				query{name: "families", path: collection("families", v), fields: fields(nearbyFields), scoreAs: "milesAway", limit: limit},
				query{name: "from", path: one("families", from, params()), fields: fields("name")},
			)
			var refusal *access.Refusal
			if errors.As(err, &refusal) && refusal.Status == http.StatusBadRequest {
				continue
			}
			if err != nil {
				return nil, err
			}
			out["note"] = "Distances are straight lines between the addresses families share, not driving routes."
			return out, nil
		}
		return nil, fmt.Errorf("that family shares no street address in Helios Who?, so there is nothing to measure from")
	},
}
