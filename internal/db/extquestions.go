package db

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

type extQuestion struct {
	ID        string   `json:"id"`
	Text      string   `json:"text"`
	Kind      string   `json:"kind"`
	Choices   []string `json:"choices"`
	Asked     string   `json:"asked"`
	Required  bool     `json:"required"`
	GoingOnly bool     `json:"goingOnly"`
}

type extReply struct {
	Question string `json:"question"`
	Person   string `json:"person,omitempty"`
	Family   string `json:"family,omitempty"`
	Value    string `json:"value"`
}

type extFamily struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (m *Model) isKid(person string) bool {
	if m.students()[person] {
		return true
	}
	p, ok := m.Table("PERSON").Get(person)
	return ok && p["child"] == "Yes"
}

func (m *Model) questionsOf(g store.Row) []store.Row {
	rows := slices.Clone(m.Table("QUESTION").Referencing("group", g["id"]))
	slices.SortFunc(rows, func(a, b store.Row) int { return strings.Compare(a["order"], b["order"]) })
	return rows
}

func (m *Model) familyOf(person string) (store.Row, bool) {
	families := m.familiesOf(person)
	if len(families) == 0 {
		return nil, false
	}
	return m.Table("GROUP").Get(families[0])
}

func (m *Model) answersFor(g store.Row, person string) []string {
	return append(m.householdOnList(g, person), m.broughtGuests(g, person)...)
}

func (m *Model) extQuestions(g store.Row, person string) ([]extQuestion, []extReply, *extFamily) {
	questions := []extQuestion{}
	replies := []extReply{}
	party := m.answersFor(g, person)
	family, hasFamily := m.familyOf(person)
	for _, q := range m.questionsOf(g) {
		choices := []string{}
		for _, c := range strings.Split(q["choices"], "\n") {
			if c = strings.TrimSpace(c); c != "" {
				choices = append(choices, c)
			}
		}
		questions = append(questions, extQuestion{ID: q["id"], Text: q["text"], Kind: q["kind"], Choices: choices, Asked: q["asked"], Required: q["required"] == "Yes", GoingOnly: q["going_only"] == "Yes"})
		for _, a := range m.Table("ANSWER").Referencing("question", q["id"]) {
			if slices.Contains(party, a["person"]) || (hasFamily && a["family"] == family["id"]) {
				replies = append(replies, extReply{Question: q["id"], Person: a["person"], Family: a["family"], Value: a["value"]})
			}
		}
	}
	if !hasFamily {
		return questions, replies, nil
	}
	return questions, replies, &extFamily{ID: family["id"], Name: family["name"]}
}

func (iv *Invitations) extReply(w http.ResponseWriter, r *http.Request) {
	c, err := iv.extCopy(r)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	var body extReply
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&body); err != nil {
		serve.Error(w, r, access.Invalid("send {\"question\", \"person\" or \"family\", \"value\"}: %v", err))
		return
	}
	m := iv.s.Model()
	q, ok := m.Table("QUESTION").Get(body.Question)
	if !ok || q["group"] != c.group["id"] {
		serve.Error(w, r, access.Missing("that question is not on this invitation"))
		return
	}
	switch {
	case q["asked"] == "household":
		family, ok := m.familyOf(c.person)
		if body.Person != "" || !ok || body.Family != family["id"] {
			serve.Error(w, r, access.Forbidden("a question for the family is answered once, for your own family"))
			return
		}
	case body.Family != "" || !slices.Contains(m.answersFor(c.group, c.person), body.Person):
		serve.Error(w, r, access.Forbidden("you answer for yourself, your family and your guests"))
		return
	}
	var edit Edit
	var existing store.Row
	for _, a := range m.Table("ANSWER").Referencing("question", q["id"]) {
		if a["person"] == body.Person && a["family"] == body.Family {
			existing = a
		}
	}
	stamped := map[string]any{"value": body.Value, "added": stamp(time.Now()), "added_by": c.person}
	switch {
	case existing != nil && body.Value == "":
		edit = Edit{Delete: existing["id"]}
	case existing != nil:
		edit = Edit{Set: existing["id"], Cells: stamped}
	case body.Value != "":
		stamped["question"], stamped["person"], stamped["family"] = q["id"], body.Person, body.Family
		edit = Edit{Insert: "ANSWER", Row: stamped}
	default:
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, err := iv.write(r.Context(), access.Actor{Email: m.addressOf(c.person)}, []Edit{edit}); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "invitations: question answered from outside", "person", c.person, "question", q["id"])
	w.WriteHeader(http.StatusNoContent)
}
