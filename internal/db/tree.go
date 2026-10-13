package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

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
	s, err := fromJSON(tree, "query", false)
	if err != nil {
		return nil, err
	}
	if !s.isList {
		return nil, fmt.Errorf("a query is a list")
	}
	return newCompiler(false).query(s)
}

func namedAt(head string, i int) bool {
	switch head {
	case "order", "include", "columns":
		return true
	case "from", "exists", "count", "select":
		return i == 1
	case "sum":
		return i <= 2
	}
	return false
}

func nameFromJSON(s, at string) (*sexp, error) {
	if s == "" || s == "@" {
		return nil, fmt.Errorf("%s is not a name", at)
	}
	if rest, ok := strings.CutPrefix(s, "@"); ok {
		return &sexp{kind: atomAt, text: rest, pos: -1}, nil
	}
	return &sexp{kind: atomName, text: s, pos: -1}, nil
}

func fromJSON(v any, at string, named bool) (*sexp, error) {
	switch x := v.(type) {
	case string:
		if named {
			return nameFromJSON(x, at)
		}
		return &sexp{kind: atomString, text: x, pos: -1}, nil
	case json.Number:
		return &sexp{kind: atomNumber, text: x.String(), pos: -1}, nil
	case bool:
		return &sexp{kind: atomName, text: fmt.Sprint(x), pos: -1}, nil
	case map[string]any:
		n, ok := x["n"].(string)
		if !ok || len(x) != 1 {
			return nil, fmt.Errorf(`%s is an object other than {"n": name}`, at)
		}
		return nameFromJSON(n, at)
	case []any:
		if len(x) == 0 {
			return nil, fmt.Errorf("%s is an empty list", at)
		}
		head, ok := x[0].(string)
		if !ok {
			return nil, fmt.Errorf("%s does not start with an operator's name", at)
		}
		out := &sexp{isList: true, pos: -1, list: []*sexp{{kind: atomName, text: head, pos: -1}}}
		for i := 1; i < len(x); i++ {
			item, err := fromJSON(x[i], fmt.Sprintf("%s[%d]", at, i), namedAt(head, i))
			if err != nil {
				return nil, err
			}
			out.list = append(out.list, item)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s is null", at)
}
