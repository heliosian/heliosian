package db

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	goingWord      = "going"
	waitlistedWord = "waitlisted"
)

const goingCondition = `(or (in id (select MEMBER.group (in person (household @viewer)) (= member "yes"))) (in rsvp_yes (select MEMBER.group (in person (household @viewer)) (= member "yes"))))`

const waitlistedCondition = `(in waitlist (select MEMBER.group (in person (household @viewer)) (= member "yes")))`

const shownCondition = `(not (in hidden_for (select EFFECTIVE_MEMBER.group (= person @viewer))))`

func searchWords(raw string) ([][]string, error) {
	var lists [][]string
	if err := json.Unmarshal([]byte(raw), &lists); err != nil {
		return nil, fmt.Errorf("%q is not a JSON list of lists of words", raw)
	}
	if err := checkWords(lists); err != nil {
		return nil, fmt.Errorf("%q %v", raw, err)
	}
	return lists, nil
}

func checkWords(lists [][]string) error {
	for _, list := range lists {
		if len(list) == 0 {
			return fmt.Errorf("has an empty list of words")
		}
		for _, w := range list {
			if w != goingWord && w != waitlistedWord && (w == "" || cells.CheckPretty(w) != nil) {
				return fmt.Errorf("holds %q, which is not a word: a word is going, waitlisted or a group's slug", w)
			}
		}
	}
	return nil
}

func savedSearchQuery(lists [][]string, from, to string) string {
	return fmt.Sprintf(`(from GROUP @e (where (in kind "event" "party" "activity") (!= status "closed") (not (blank start)) (<= start %q) (or (>= end %q) (and (blank end) (>= start %q))) %s) (order start asc))`,
		to+" 23:59", from, from, searchCondition(lists))
}

func (m *Model) savedSearchByToken(token string) (store.Row, bool) {
	for _, row := range m.Table("SAVED_SEARCH").All() {
		if row["token"] == token {
			return row, true
		}
	}
	for _, a := range m.Table("ALIAS").All() {
		if a["alias"] == strings.ToLower(token) {
			return m.Table("SAVED_SEARCH").Get(a["target"])
		}
	}
	return nil, false
}

func RegisterFeeds(mux *http.ServeMux, s *Store, now func() time.Time) {
	mux.HandleFunc("GET /open/feed/{file}", func(w http.ResponseWriter, r *http.Request) {
		ics, ok, err := s.Model().savedSearchICS(r.Context(), strings.TrimSuffix(r.PathValue("file"), ".ics"), "https://"+r.Host, now())
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("Content-Disposition", `inline; filename="helios-calendar.ics"`)
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(ics)
	})
}

func (m *Model) savedSearchICS(ctx context.Context, token, origin string, now time.Time) ([]byte, bool, error) {
	row, ok := m.savedSearchByToken(token)
	if !ok {
		return nil, false, nil
	}
	lists, err := searchWords(row["search"])
	if err != nil {
		return nil, true, err
	}
	at := now.In(School)
	q, err := Parse(savedSearchQuery(lists, at.AddDate(-1, 0, 0).Format(DateLayout), at.AddDate(2, 0, 0).Format(DateLayout)))
	if err != nil {
		return nil, true, err
	}
	result := m.Run(ctx, q, Env{Viewer: row["person"], Now: now})
	if result.Err != nil {
		return nil, true, result.Err
	}
	c := mail.Calendar{Product: "Helios When", Method: mail.MethodPublish, Name: row["name"], TimeZone: School.String(), Stamp: now}
	for _, id := range result.IDs {
		g := result.Resources["GROUP"][id]
		allDay, _ := cells.YesNo(g["all_day"], false)
		start, err := parseMoment(g["start"])
		if err != nil {
			return nil, true, err
		}
		end := start
		if g["end"] != "" {
			if end, err = parseMoment(g["end"]); err != nil {
				return nil, true, err
			}
		}
		if allDay {
			end = end.AddDate(0, 0, 1)
		}
		key := g["slug"]
		if key == "" {
			key = id
		}
		link := origin + "/e/" + key
		c.Events = append(c.Events, mail.Event{
			UID: id + "@when.heliosian.com", Start: start, End: end, AllDay: allDay, Summary: g["name"],
			Description: strings.TrimSpace(g["description"] + "\n\n" + link), Location: g["location"], URL: link,
		})
	}
	return []byte(c.ICS()), true, nil
}

func parseMoment(s string) (time.Time, error) {
	if t, err := time.ParseInLocation(MomentLayout, s, School); err == nil {
		return t, nil
	}
	return time.ParseInLocation(DateLayout, s, School)
}

type savedSearchRequest struct {
	Search [][]string `json:"search"`
	From   string     `json:"from"`
	To     string     `json:"to"`
}

func registerSavedSearch(mux *http.ServeMux, s *Store, tokens auth.Tokens, now func() time.Time) {
	mux.HandleFunc("POST /api/do/saved-search", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, _, ok := caller(w, r, m, tokens, now())
		if !ok {
			return
		}
		var req savedSearchRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&req); err != nil {
			serve.Error(w, r, access.Invalid("send the search as JSON: %v", err))
			return
		}
		for _, d := range []string{req.From, req.To} {
			if _, err := time.Parse(DateLayout, d); err != nil {
				serve.Error(w, r, access.Invalid("from and to are dates as %s, not %q", DateLayout, d))
				return
			}
		}
		if err := checkWords(req.Search); err != nil {
			serve.Error(w, r, access.Invalid("the search %v", err))
			return
		}
		q, err := Parse(savedSearchQuery(req.Search, req.From, req.To))
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		result := m.Run(r.Context(), q, env)
		if result.Err != nil {
			serve.Error(w, r, access.Invalid("%v", result.Err))
			return
		}
		serve.Write(w, r, http.StatusOK, answer{Now: env.Now.Format(cells.StampFormat), Query: q.String(), Result: result.IDs, Resources: result.Resources})
	})
}

func wordCondition(w string) string {
	switch w {
	case goingWord:
		return goingCondition
	case waitlistedWord:
		return waitlistedCondition
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
	return "(and " + strings.Join(append(clauses, shownCondition), " ") + ")"
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
			if w == goingWord || w == waitlistedWord {
				continue
			}
			if _, ok := m.Table("GROUP").Distinct("slug", w); !ok {
				return nil, fmt.Errorf("no group has the slug %q", w)
			}
		}
	}
	return nil, nil
}
