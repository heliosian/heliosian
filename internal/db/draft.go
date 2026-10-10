package db

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

type draftRule struct {
	Exclude     bool   `json:"exclude"`
	Target      string `json:"target"`
	Person      string `json:"person"`
	Search      string `json:"search"`
	ReplaceWith string `json:"replace_with"`
	Within      string `json:"within"`
}

type draftMember struct {
	Person string `json:"person"`
	Member string `json:"member"`
}

type groupDraft struct {
	List      string        `json:"list"`
	Name      string        `json:"name"`
	RuleWords []string      `json:"ruleWords"`
	Rules     []draftRule   `json:"rules"`
	Members   []draftMember `json:"members"`
}

type draftReason struct {
	Rule  *int `json:"rule,omitempty"`
	Added bool `json:"added,omitempty"`
}

type draftPerson struct {
	Person  string        `json:"person"`
	Name    string        `json:"name,omitempty"`
	Reasons []draftReason `json:"reasons"`
}

type draftAnswer struct {
	Members    []draftPerson `json:"members"`
	RuleCounts []int         `json:"ruleCounts"`
}

func (m *Model) visibleIDs(ctx context.Context, env Env, table, condition string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	q, err := Parse("(from " + table + " @r (where (in id " + quoted(ids) + ")" + condition + "))")
	if err != nil {
		return nil, err
	}
	for _, id := range m.Run(ctx, q, env).IDs {
		out[id] = true
	}
	return out, nil
}

func (m *Model) checkDraft(ctx context.Context, env Env, d groupDraft) error {
	if d.List != "" {
		runs, err := m.visibleIDs(ctx, env, "GROUP", " (runs_list @r)", []string{d.List})
		if err != nil {
			return err
		}
		if !runs[d.List] {
			return access.Forbidden("you do not run this email list")
		}
	}
	groups, people := []string{}, []string{}
	for _, r := range d.Rules {
		for _, g := range []string{r.Target, r.Within} {
			if g != "" {
				groups = append(groups, g)
			}
		}
		if r.Person != "" {
			people = append(people, r.Person)
		}
	}
	for _, mem := range d.Members {
		if mem.Member != "yes" && mem.Member != "excluded" {
			return access.Invalid("a member is yes or excluded, not %q", mem.Member)
		}
		people = append(people, mem.Person)
	}
	slices.Sort(groups)
	groups = slices.Compact(groups)
	seen, err := m.visibleIDs(ctx, env, "GROUP", " (sees_members @r)", groups)
	if err != nil {
		return err
	}
	for _, g := range groups {
		if !seen[g] {
			return access.Forbidden("a rule names a group whose members you cannot see: %s", g)
		}
	}
	slices.Sort(people)
	people = slices.Compact(people)
	shown, err := m.visibleIDs(ctx, env, "PERSON", "", people)
	if err != nil {
		return err
	}
	for _, p := range people {
		if !shown[p] {
			return access.Forbidden("you cannot see %s", p)
		}
	}
	return nil
}

func (m *Model) draftMembers(ctx context.Context, env Env, d groupDraft) (draftAnswer, error) {
	reasons := map[string][]draftReason{}
	excluded := map[string]bool{}
	for _, mem := range d.Members {
		if mem.Member == "excluded" {
			excluded[mem.Person] = true
			continue
		}
		reasons[mem.Person] = append(reasons[mem.Person], draftReason{Added: true})
	}
	counts := []int{}
	for i, r := range d.Rules {
		row := store.Row{"target": r.Target, "person": r.Person, "search": r.Search, "replace_with": r.ReplaceWith, "within": r.Within}
		selected := m.selectRule(row, map[string]bool{})
		counts = append(counts, len(selected))
		for person := range selected {
			if r.Exclude {
				excluded[person] = true
				continue
			}
			reasons[person] = append(reasons[person], draftReason{Rule: &i})
		}
	}
	people := m.Shown("PERSON")
	for person := range reasons {
		row, ok := people.Get(person)
		if excluded[person] || !ok || strings.TrimSpace(row["deactivated"]) != "" {
			delete(reasons, person)
		}
	}
	ids := slices.Sorted(maps.Keys(reasons))
	shown, err := m.visibleIDs(ctx, env, "PERSON", "", ids)
	if err != nil {
		return draftAnswer{}, err
	}
	out := draftAnswer{Members: []draftPerson{}, RuleCounts: counts}
	for _, person := range ids {
		p := draftPerson{Person: person, Reasons: reasons[person]}
		if shown[person] {
			row, _ := people.Get(person)
			p.Name = row["name_show"]
		}
		out.Members = append(out.Members, p)
	}
	return out, nil
}

func (m *Model) groupFacts(d groupDraft, members []draftPerson) describe.GroupFacts {
	facts := describe.GroupFacts{Title: d.Name, Rules: d.RuleWords, Members: len(members), Roles: map[string]int{}, Grades: map[string]int{}, Classrooms: map[string]int{}}
	roles := map[string]map[string]bool{}
	for _, g := range m.Table("GROUP").All() {
		if word := roleWord[g["slug"]]; g["kind"] == "group" && word != "" {
			roles[word] = map[string]bool{}
			for _, row := range m.effectiveRows(g["id"]) {
				roles[word][row["person"]] = true
			}
		}
	}
	people, groups := m.Shown("PERSON"), m.Table("GROUP")
	for _, mem := range members {
		p, _ := people.Get(mem.Person)
		switch {
		case p["source"] == "guest":
			facts.Roles["Guest"]++
		case roles["Student"][mem.Person]:
			facts.Roles["Student"]++
			if p["grade"] != "" {
				facts.Grades[p["grade"]]++
			}
			if c, ok := groups.Get(p["classroom"]); ok {
				facts.Classrooms[c["name"]]++
			}
		case roles["Staff"][mem.Person]:
			facts.Roles["Staff"]++
		case roles["Parent"][mem.Person]:
			facts.Roles["Parent"]++
		}
	}
	return facts
}

var roleWord = map[string]string{"students": "Student", "staff": "Staff", "parents": "Parent"}

func readDraft(w http.ResponseWriter, r *http.Request) (groupDraft, bool) {
	var d groupDraft
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&d); err != nil {
		serve.Error(w, r, access.Invalid("send the draft as JSON: %v", err))
		return groupDraft{}, false
	}
	return d, true
}

func RegisterDrafts(mux *http.ServeMux, s *Store, describer *describe.Describer, tokens auth.Tokens, now func() time.Time) {
	mux.HandleFunc("POST /api/do/draft-members", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, _, ok := caller(w, r, m, tokens, now())
		if !ok {
			return
		}
		d, ok := readDraft(w, r)
		if !ok {
			return
		}
		if err := m.checkDraft(r.Context(), env, d); err != nil {
			serve.Error(w, r, err)
			return
		}
		out, err := m.draftMembers(r.Context(), env, d)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		serve.Write(w, r, http.StatusOK, out)
	})
	mux.HandleFunc("POST /api/do/describe-group", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, actor, ok := caller(w, r, m, tokens, now())
		if !ok {
			return
		}
		d, ok := readDraft(w, r)
		if !ok {
			return
		}
		if err := m.checkDraft(r.Context(), env, d); err != nil {
			serve.Error(w, r, err)
			return
		}
		members, err := m.draftMembers(r.Context(), env, d)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		description, err := describer.Group(r.Context(), actor.Email, m.groupFacts(d, members.Members))
		if errors.Is(err, claude.ErrTooMany) {
			serve.Error(w, r, access.Refuse(http.StatusTooManyRequests, "%v", err))
			return
		}
		if errors.Is(err, describe.ErrTooLong) {
			serve.Error(w, r, access.Invalid("%v", err))
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "describe group", "viewer", env.Viewer, "name", d.Name, "error", err)
			serve.Error(w, r, access.Refuse(http.StatusBadGateway, "could not write a description right now"))
			return
		}
		slog.InfoContext(r.Context(), "described group", "viewer", env.Viewer, "name", d.Name, "members", len(members.Members))
		serve.Write(w, r, http.StatusOK, map[string]string{"description": description})
	})
}
