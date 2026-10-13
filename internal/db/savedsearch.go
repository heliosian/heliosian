package db

import (
	"encoding/json"
	"fmt"
	"strings"

	"heliosian/internal/cells"
)

const goingWord = "going"

const goingCondition = `(or (in id (select MEMBER.group (in person (household @viewer)) (= member "yes"))) (in rsvp_yes (select MEMBER.group (in person (household @viewer)) (= member "yes"))))`

func searchWords(raw string) ([][]string, error) {
	var lists [][]string
	if err := json.Unmarshal([]byte(raw), &lists); err != nil {
		return nil, fmt.Errorf("%q is not a JSON list of lists of words", raw)
	}
	for _, list := range lists {
		if len(list) == 0 {
			return nil, fmt.Errorf("%q has an empty list of words", raw)
		}
		for _, w := range list {
			if w != goingWord && (w == "" || cells.CheckPretty(w) != nil) {
				return nil, fmt.Errorf("%q is not a word: a word is going or a group's slug", w)
			}
		}
	}
	return lists, nil
}

func wordCondition(w string) string {
	if w == goingWord {
		return goingCondition
	}
	return fmt.Sprintf(`(or (covers @e.audience (members (slug %q))) (and (blank @e.audience) (exists PERSON (in id (members (slug %q))))) (exists GROUP (= id (slug %q)) (in id (ancestors @e))))`, w, w, w)
}

func searchCondition(lists [][]string) string {
	clauses := []string{}
	for _, list := range lists {
		words := []string{}
		for _, w := range list {
			words = append(words, wordCondition(w))
		}
		clauses = append(clauses, "(or "+strings.Join(words, " ")+")")
	}
	if len(clauses) == 0 {
		return "true"
	}
	return "(and " + strings.Join(clauses, " ") + ")"
}

func savedSearchGenerated(row map[string]string) {
	lists, err := searchWords(row["search"])
	if err != nil {
		panic("db: a saved search the load let through: " + err.Error())
	}
	row["condition"] = searchCondition(lists)
}

func savedSearchSlugs(m *Model, c Change) ([]Edit, error) {
	if c.New == nil || c.Old != nil && c.Old["search"] == c.New["search"] {
		return nil, nil
	}
	lists, err := searchWords(c.New["search"])
	if err != nil {
		return nil, err
	}
	for _, list := range lists {
		for _, w := range list {
			if w == goingWord {
				continue
			}
			if _, ok := m.Table("GROUP").Distinct("slug", w); !ok {
				return nil, fmt.Errorf("no group has the slug %q", w)
			}
		}
	}
	return nil, nil
}
