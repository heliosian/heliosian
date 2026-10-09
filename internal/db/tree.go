package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

var operators = []string{"=", "!=", "<", "<=", ">", ">="}

func ParseJSON(raw []byte) (*Query, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var tree any
	if err := d.Decode(&tree); err != nil {
		return nil, fmt.Errorf("the query is not JSON: %v", err)
	}
	if d.More() {
		return nil, fmt.Errorf("text after the query")
	}
	s, err := scanFromJSON("from", tree, "query")
	if err != nil {
		return nil, err
	}
	return newCompiler(false).query(s)
}

func (q *Query) Tree() map[string]any {
	return scanToJSON(q.tree, 1)
}

func atom(kind atomKind, text string) *sexp {
	return &sexp{kind: kind, text: text, pos: -1}
}

func word(text string) *sexp {
	return atom(atomName, text)
}

func bracket(items ...*sexp) *sexp {
	return &sexp{isList: true, list: items, pos: -1}
}

func object(v any, where string) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", where)
	}
	return m, nil
}

func single(v any, where, what string) (string, any, error) {
	m, err := object(v, where)
	if err != nil {
		return "", nil, err
	}
	if len(m) != 1 {
		return "", nil, fmt.Errorf("%s has %d fields; %s has one", where, len(m), what)
	}
	for k, arg := range m {
		return k, arg, nil
	}
	return "", nil, nil
}

func only(m map[string]any, where string, allowed ...string) error {
	for k := range m {
		if !slices.Contains(allowed, k) {
			return fmt.Errorf("%s has no field %q; it takes %s", where, k, strings.Join(allowed, ", "))
		}
	}
	return nil
}

func scanFromJSON(head string, v any, where string) (*sexp, error) {
	m, err := object(v, where)
	if err != nil {
		return nil, err
	}
	allowed := []string{"from", "as", "where"}
	switch head {
	case "from":
		allowed = append(allowed, "order", "limit", "include")
	case "sum":
		allowed = append(allowed, "path")
	}
	if err := only(m, where, allowed...); err != nil {
		return nil, err
	}
	table, ok := m["from"].(string)
	if !ok {
		return nil, fmt.Errorf("%s names no table in from", where)
	}
	out := bracket(word(head))
	if head == "sum" {
		p, err := pathFromJSON(m["path"], where+".path")
		if err != nil {
			return nil, err
		}
		out.list = append(out.list, p)
	}
	out.list = append(out.list, word(table))
	if as, ok := m["as"]; ok {
		s, ok := as.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("%s.as is not a name", where)
		}
		out.list = append(out.list, atom(atomAt, s))
	}
	conds, err := condsFromJSON(m["where"], where+".where")
	if err != nil {
		return nil, err
	}
	if head != "from" {
		out.list = append(out.list, conds...)
		return out, nil
	}
	if len(conds) > 0 {
		out.list = append(out.list, bracket(append([]*sexp{word("where")}, conds...)...))
	}
	if order, ok := m["order"]; ok {
		part, err := orderFromJSON(order, where+".order")
		if err != nil {
			return nil, err
		}
		out.list = append(out.list, part)
	}
	if limit, ok := m["limit"]; ok {
		n, ok := limit.(json.Number)
		if !ok {
			return nil, fmt.Errorf("%s.limit is not a number", where)
		}
		out.list = append(out.list, bracket(word("limit"), atom(atomNumber, n.String())))
	}
	if include, ok := m["include"]; ok {
		paths, ok := include.([]any)
		if !ok {
			return nil, fmt.Errorf("%s.include is not a list", where)
		}
		part := bracket(word("include"))
		for i, p := range paths {
			s, ok := p.(string)
			if !ok {
				return nil, fmt.Errorf("%s.include[%d] is not a path", where, i)
			}
			part.list = append(part.list, word(s))
		}
		out.list = append(out.list, part)
	}
	return out, nil
}

func orderFromJSON(v any, where string) (*sexp, error) {
	keys, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is not a list", where)
	}
	part := bracket(word("order"))
	for i, k := range keys {
		at := fmt.Sprintf("%s[%d]", where, i)
		km, err := object(k, at)
		if err != nil {
			return nil, err
		}
		if err := only(km, at, "path", "dir"); err != nil {
			return nil, err
		}
		p, err := pathFromJSON(km["path"], at+".path")
		if err != nil {
			return nil, err
		}
		dir := "asc"
		if d, ok := km["dir"]; ok {
			if dir, ok = d.(string); !ok {
				return nil, fmt.Errorf("%s.dir is not asc or desc", at)
			}
		}
		part.list = append(part.list, p, word(dir))
	}
	return part, nil
}

func condsFromJSON(v any, where string) ([]*sexp, error) {
	if v == nil {
		return nil, nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is not a list", where)
	}
	out := []*sexp{}
	for i, item := range items {
		c, err := condFromJSON(item, fmt.Sprintf("%s[%d]", where, i))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func pathFromJSON(v any, where string) (*sexp, error) {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil, fmt.Errorf("%s is not a path", where)
	}
	if rest, at := strings.CutPrefix(s, "@"); at {
		return atom(atomAt, rest), nil
	}
	return word(s), nil
}

func condFromJSON(v any, where string) (*sexp, error) {
	if b, ok := v.(bool); ok {
		return word(fmt.Sprint(b)), nil
	}
	op, arg, err := single(v, where, "a condition")
	if err != nil {
		return nil, err
	}
	at := where + "." + op
	switch {
	case op == "path":
		return pathFromJSON(arg, at)
	case op == "and" || op == "or":
		conds, err := condsFromJSON(arg, at)
		if err != nil {
			return nil, err
		}
		return bracket(append([]*sexp{word(op)}, conds...)...), nil
	case op == "not":
		c, err := condFromJSON(arg, at)
		if err != nil {
			return nil, err
		}
		return bracket(word("not"), c), nil
	case op == "blank":
		p, err := operandFromJSON(arg, at)
		if err != nil {
			return nil, err
		}
		return bracket(word("blank"), p), nil
	case op == "exists":
		return scanFromJSON("exists", arg, at)
	case op == "in" || op == "contains" || slices.Contains(operators, op):
		args, ok := arg.([]any)
		if !ok {
			return nil, fmt.Errorf("%s is not a list", at)
		}
		out := bracket(word(op))
		for i, a := range args {
			o, err := operandFromJSON(a, fmt.Sprintf("%s[%d]", at, i))
			if err != nil {
				return nil, err
			}
			out.list = append(out.list, o)
		}
		return out, nil
	}
	return callFromJSON(op, arg, at)
}

func callFromJSON(op string, arg any, at string) (*sexp, error) {
	args, ok := arg.([]any)
	if !ok {
		return nil, fmt.Errorf("%s is not a list of arguments", at)
	}
	out := bracket(word(op))
	for i, a := range args {
		o, err := operandFromJSON(a, fmt.Sprintf("%s[%d]", at, i))
		if err != nil {
			return nil, err
		}
		out.list = append(out.list, o)
	}
	return out, nil
}

func operandFromJSON(v any, where string) (*sexp, error) {
	switch x := v.(type) {
	case string:
		return atom(atomString, x), nil
	case json.Number:
		return atom(atomNumber, x.String()), nil
	case bool:
		return word(fmt.Sprint(x)), nil
	}
	op, arg, err := single(v, where, "a value")
	if err != nil {
		return nil, err
	}
	at := where + "." + op
	switch op {
	case "path":
		return pathFromJSON(arg, at)
	case "today", "now":
		if arg != true {
			return nil, fmt.Errorf("%s is %s: true", at, op)
		}
		return word(op), nil
	case "count", "sum":
		return scanFromJSON(op, arg, at)
	case "select":
		sm, err := object(arg, at)
		if err != nil {
			return nil, err
		}
		if err := only(sm, at, "from", "column", "where"); err != nil {
			return nil, err
		}
		table, _ := sm["from"].(string)
		column, _ := sm["column"].(string)
		if table == "" || column == "" {
			return nil, fmt.Errorf("%s names no table in from or no column", at)
		}
		conds, err := condsFromJSON(sm["where"], at+".where")
		if err != nil {
			return nil, err
		}
		return bracket(append([]*sexp{word("select"), word(table + "." + column)}, conds...)...), nil
	}
	return callFromJSON(op, arg, at)
}

func scanToJSON(s *sexp, at int) map[string]any {
	out := map[string]any{}
	if s.head() == "sum" {
		out["path"] = pathToJSON(s.list[1])
		at = 2
	}
	out["from"] = s.list[at].text
	rest := s.list[at+1:]
	if len(rest) > 0 && !rest[0].isList && rest[0].kind == atomAt {
		out["as"] = rest[0].text
		rest = rest[1:]
	}
	if s.head() != "from" {
		if len(rest) > 0 {
			out["where"] = condsToJSON(rest)
		}
		return out
	}
	for _, part := range rest {
		switch part.head() {
		case "where":
			out["where"] = condsToJSON(part.list[1:])
		case "order":
			keys := []any{}
			for i := 1; i+1 < len(part.list); i += 2 {
				keys = append(keys, map[string]any{"path": pathToJSON(part.list[i]), "dir": part.list[i+1].text})
			}
			out["order"] = keys
		case "limit":
			out["limit"] = json.Number(part.list[1].text)
		case "include":
			paths := []any{}
			for _, p := range part.list[1:] {
				paths = append(paths, p.text)
			}
			out["include"] = paths
		}
	}
	return out
}

func condsToJSON(items []*sexp) []any {
	out := []any{}
	for _, c := range items {
		out = append(out, condToJSON(c))
	}
	return out
}

func pathToJSON(s *sexp) string {
	if s.kind == atomAt {
		return "@" + s.text
	}
	return s.text
}

func condToJSON(s *sexp) any {
	if !s.isList {
		if s.text == "true" || s.text == "false" {
			return s.text == "true"
		}
		return map[string]any{"path": pathToJSON(s)}
	}
	switch op := s.head(); op {
	case "and", "or":
		return map[string]any{op: condsToJSON(s.list[1:])}
	case "not":
		return map[string]any{"not": condToJSON(s.list[1])}
	case "blank":
		return map[string]any{"blank": operandToJSON(s.list[1])}
	case "exists":
		return map[string]any{"exists": scanToJSON(s, 1)}
	}
	args := []any{}
	for _, a := range s.list[1:] {
		args = append(args, operandToJSON(a))
	}
	return map[string]any{s.head(): args}
}

func operandToJSON(s *sexp) any {
	if s.isList {
		switch s.head() {
		case "count", "sum":
			return map[string]any{s.head(): scanToJSON(s, 1)}
		case "select":
			table, column, _ := strings.Cut(s.list[1].text, ".")
			out := map[string]any{"from": table, "column": column}
			if len(s.list) > 2 {
				out["where"] = condsToJSON(s.list[2:])
			}
			return map[string]any{"select": out}
		}
		args := []any{}
		for _, a := range s.list[1:] {
			args = append(args, operandToJSON(a))
		}
		return map[string]any{s.head(): args}
	}
	switch {
	case s.kind == atomString:
		return s.text
	case s.kind == atomNumber:
		return json.Number(s.text)
	case s.text == "true" || s.text == "false":
		return s.text == "true"
	case s.text == "today" || s.text == "now":
		return map[string]any{s.text: true}
	}
	return map[string]any{"path": pathToJSON(s)}
}
