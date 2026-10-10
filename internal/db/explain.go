package db

import (
	"net/http"
	"slices"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/serve"
)

type verdict struct {
	Clause int  `json:"clause"`
	Holds  bool `json:"holds"`
	Actor  bool `json:"actor"`
	Rest   bool `json:"rest"`
}

func judge(i int, c Clause, f *frame) verdict {
	v := verdict{Clause: i, Holds: c.cond.eval(f), Actor: true, Rest: c.rest.eval(f)}
	if c.Actor != "" {
		v.Actor = c.actor.eval(f)
	}
	return v
}

type columnVerdict struct {
	Column   string    `json:"column"`
	Private  bool      `json:"private,omitempty"`
	Readable bool      `json:"readable"`
	Clauses  []verdict `json:"clauses"`
}

type explanation struct {
	Table    string          `json:"table"`
	ID       string          `json:"id"`
	Readable bool            `json:"readable"`
	Clauses  []verdict       `json:"clauses"`
	Columns  []columnVerdict `json:"columns"`
}

type policyList struct {
	Clauses []Clause `json:"clauses"`
}

func (m *Model) explain(env, self Env, id string) (explanation, bool) {
	tableName, ok := TableOf(id)
	if !ok {
		return explanation{}, false
	}
	t, _ := Lookup(tableName)
	if t.Generated {
		return explanation{}, false
	}
	r := m.newRun(env)
	row, ok := r.table(t.Name).Get(id)
	if !ok {
		return explanation{}, false
	}
	readable := r.readable(t, row)
	if !readable && (self.Viewer == "" || !m.newRun(self).readable(t, row)) {
		return explanation{}, false
	}
	f := &frame{table: t, row: row, name: "row", run: r}
	out := explanation{Table: t.Name, ID: id, Readable: readable, Clauses: []verdict{}, Columns: []columnVerdict{}}
	for i, c := range policies.clauses {
		if c.Kind == "read" && (c.Table == t.Name || c.Table == "*") {
			out.Clauses = append(out.Clauses, judge(i, c, f))
		}
	}
	for _, col := range t.Columns {
		cv := columnVerdict{Column: col.Name, Private: col.Private, Clauses: []verdict{}}
		for i, c := range policies.clauses {
			if c.Kind == "read columns" && (c.Table == t.Name && slices.Contains(c.Columns, col.Name) || c.Table == "*" && !col.Private) {
				v := judge(i, c, f)
				cv.Clauses = append(cv.Clauses, v)
				cv.Readable = cv.Readable || v.Holds
			}
		}
		out.Columns = append(out.Columns, cv)
	}
	return out, true
}

func registerExplain(mux *http.ServeMux, s *Store, tokens auth.Tokens, now func() time.Time) {
	mux.HandleFunc("GET /api/policies", func(w http.ResponseWriter, r *http.Request) {
		serve.Write(w, r, http.StatusOK, policyList{Clauses: policies.clauses})
	})
	mux.HandleFunc("GET /api/explain/{id}", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, _, ok := caller(w, r, m, tokens, now())
		if !ok {
			return
		}
		self := Env{Viewer: m.SignedIn(auth.RealEmail(r)), Now: env.Now}
		out, ok := m.explain(env, self, r.PathValue("id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		serve.Write(w, r, http.StatusOK, out)
	})
}
