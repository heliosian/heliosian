package db

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
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
	Person  string `json:"person"`
	Member  string `json:"member"`
	Invited string `json:"invited"`
}

type groupDraft struct {
	List      string        `json:"list"`
	Name      string        `json:"name"`
	RuleWords []string      `json:"ruleWords"`
	Rules     []draftRule   `json:"rules"`
	Members   []draftMember `json:"members"`
}

type draftReason struct {
	Rule    *int `json:"rule,omitempty"`
	Added   bool `json:"added,omitempty"`
	Invited bool `json:"invited,omitempty"`
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
	result := m.Run(ctx, q, env)
	if result.Err != nil {
		return nil, result.Err
	}
	for _, id := range result.IDs {
		out[id] = true
	}
	return out, nil
}

func (m *Model) checkDraft(ctx context.Context, env Env, d groupDraft) error {
	g, _ := m.Table("GROUP").Get(d.List)
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
		if !slices.Contains([]string{"", "yes", "excluded", "cancelled"}, mem.Member) {
			return access.Invalid("invalid membership state %q", mem.Member)
		}
		if mem.Invited != "" && (g["kind"] != "activity" || !slices.Contains([]string{"yes", "excluded"}, mem.Invited)) {
			return access.Invalid("invalid activity invitation state %q", mem.Invited)
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
	g, _ := m.Table("GROUP").Get(d.List)
	reasons := map[string][]draftReason{}
	excluded := map[string]bool{}
	for _, mem := range d.Members {
		if keptOffState(mem.Member, mem.Invited, g["kind"]) {
			excluded[mem.Person] = true
			continue
		}
		if mem.Member == "yes" {
			reasons[mem.Person] = append(reasons[mem.Person], draftReason{Added: true})
		}
		if mem.Invited == "yes" {
			reasons[mem.Person] = append(reasons[mem.Person], draftReason{Invited: true})
		}
	}
	counts := []int{}
	resolver := membershipResolver{m: m, reads: reads{}, resolved: map[string]map[string][]string{}}
	for i, r := range d.Rules {
		row := store.Row{"target": r.Target, "person": r.Person, "search": r.Search, "replace_with": r.ReplaceWith, "within": r.Within}
		selected := resolver.selectRule("", row)
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

func RegisterDrafts(mux *http.ServeMux, s *Store, describer *describe.Describer, tokens auth.Tokens) {
	mux.HandleFunc("POST /api/do/draft-members", modelAction(s, tokens, func(ctx context.Context, m *Model, env Env, _ access.Actor, d groupDraft) (draftAnswer, error) {
		if err := m.checkDraft(ctx, env, d); err != nil {
			return draftAnswer{}, err
		}
		return m.draftMembers(ctx, env, d)
	}))
	mux.HandleFunc("POST /api/do/describe-group", modelAction(s, tokens, func(ctx context.Context, m *Model, env Env, actor access.Actor, d groupDraft) (map[string]string, error) {
		if err := m.checkDraft(ctx, env, d); err != nil {
			return nil, err
		}
		members, err := m.draftMembers(ctx, env, d)
		if err != nil {
			return nil, err
		}
		description, err := describer.Group(ctx, actor.Email, m.groupFacts(d, members.Members))
		if errors.Is(err, claude.ErrTooMany) {
			return nil, access.Refuse(http.StatusTooManyRequests, "%v", err)
		}
		if errors.Is(err, describe.ErrTooLong) {
			return nil, access.Invalid("%v", err)
		}
		if err != nil {
			slog.ErrorContext(ctx, "describe group", "viewer", env.Viewer, "name", d.Name, "error", err)
			return nil, access.Refuse(http.StatusBadGateway, "could not write a description right now")
		}
		slog.InfoContext(ctx, "described group", "viewer", env.Viewer, "name", d.Name, "members", len(members.Members))
		return map[string]string{"description": description}, nil
	}))
	mux.HandleFunc("POST /api/do/describe-activity", modelAction(s, tokens, func(ctx context.Context, _ *Model, _ Env, actor access.Actor, body activityDraft) (map[string]string, error) {
		facts := describe.ActivityFacts{Title: body.Title, Parent: body.Parent, Category: body.Category, When: body.When, Notes: body.Notes}
		description, err := describer.Activity(ctx, actor.Email, facts)
		if errors.Is(err, claude.ErrTooMany) {
			return nil, access.Refuse(http.StatusTooManyRequests, "%v", err)
		}
		if errors.Is(err, describe.ErrTooLong) {
			return nil, access.Invalid("%v", err)
		}
		if err != nil {
			slog.ErrorContext(ctx, "describe activity", "actor", actor.Email, "title", body.Title, "error", err)
			return nil, access.Refuse(http.StatusBadGateway, "could not write a description right now")
		}
		slog.InfoContext(ctx, "described activity", "actor", actor.Email, "title", body.Title)
		return map[string]string{"description": description}, nil
	}))
	mux.HandleFunc("POST /api/do/describe-charity", modelAction(s, tokens, func(ctx context.Context, m *Model, env Env, actor access.Actor, body charityDraft) (map[string]string, error) {
		name := strings.TrimSpace(body.Name)
		if name == "" {
			return nil, access.Invalid("give the charity's name first")
		}
		team, err := Parse(`(from APP (where (= key "birthday") (birthday_team)))`)
		if err != nil {
			return nil, err
		}
		result := m.Run(ctx, team, env)
		if result.Err != nil {
			return nil, result.Err
		}
		if len(result.IDs) == 0 {
			return nil, access.Forbidden("only the Birthdays team looks charities up")
		}
		info, err := describer.Charity(ctx, actor.Email, name, body.URL)
		if errors.Is(err, claude.ErrTooMany) {
			return nil, access.Refuse(http.StatusTooManyRequests, "%v", err)
		}
		if errors.Is(err, describe.ErrTooLong) {
			return nil, access.Invalid("%v", err)
		}
		if err != nil {
			slog.ErrorContext(ctx, "describe charity", "actor", actor.Email, "name", name, "error", err)
			return nil, access.Refuse(http.StatusBadGateway, "could not look this charity up right now")
		}
		if info.Sentence == "" {
			return nil, access.Missing("could not find this charity; please enter its information")
		}
		slog.InfoContext(ctx, "described charity", "actor", actor.Email, "name", name, "url", info.DonationLink)
		return map[string]string{"description": info.Sentence, "url": info.DonationLink}, nil
	}))
}

type charityDraft struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type activityDraft struct {
	Title    string `json:"title"`
	Parent   string `json:"parent"`
	Category string `json:"category"`
	When     string `json:"when"`
	Notes    string `json:"notes"`
}
