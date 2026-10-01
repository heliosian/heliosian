package ask

import (
	"encoding/json"
)

const partyFields = "title,subtitle,summary,needToKnow~400,category.title,celebration.title,start,end,location,address,price,unit,capacity,sold,remaining,waiting,availability,hosts.fullName,audience,adults,students,dropOff,parentTicket,hosting,invited,tickets.name,tickets.status,tickets.quantity,tickets.mine,link"

var parties = tool{
	name:        "parties",
	description: "The fun(d)raiser parties on Helios Celebrate: every party still to come with its celebration, date, place, price, tickets left, waitlist, hosts, who may come and the tickets the viewer may see (mine marks their household's own), plus the page's introduction and ticket note. Parties that have happened are left out unless asked for. One call is enough: it returns every party at once.",
	words:       "Looking at the parties",
	properties: map[string]any{
		"query":        str("Words to find in a title, subtitle, summary or category."),
		"include_past": boolean("Also the parties that have already happened."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			Query       string
			IncludePast bool `json:"include_past"`
		}](input)
		if err != nil {
			return nil, err
		}
		past := "false"
		if in.IncludePast {
			past = ""
		}
		v := params("past", past, "q", in.Query, "include", "hosts,category,celebration,tickets")
		out, err := t.ask(
			query{name: "parties", path: collection("parties", v), fields: fields(partyFields)},
			query{name: "settings", path: collection("celebrate-settings", params()), fields: fields("settings.partiesIntro~600,settings.ticketNote~400")},
		)
		if err != nil {
			return nil, err
		}
		out["page"] = first(out["settings"])["settings"]
		delete(out, "settings")
		return out, nil
	},
}
