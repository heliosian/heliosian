package db

import (
	"strconv"
	"strings"

	"heliosian/internal/logging"
)

var groupID = typ{class: classRow, kind: ID, table: "GROUP"}

func (cx *compiler) slugNames(s *sexp, many bool) ([]string, error) {
	names := []string{}
	for _, a := range s.list[1:] {
		if a.isList || a.kind != atomString {
			return nil, s.errorf("slug takes quoted slugs")
		}
		names = append(names, strings.ToLower(a.text))
	}
	if len(names) == 0 || !many && len(names) != 1 {
		return nil, s.errorf("slug takes one quoted slug where it stands for one group, and one or more where it stands for a set")
	}
	if !cx.policy {
		cx.slugs = append(cx.slugs, names...)
	}
	return names, nil
}

func (cx *compiler) slugOperand(s *sexp) (operand, error) {
	names, err := cx.slugNames(s, false)
	if err != nil {
		return operand{}, err
	}
	policy := cx.policy
	return operand{t: groupID, eval: func(f *frame) value {
		return value{kind: ID, s: f.run.slugged(names[0], policy)}
	}}, nil
}

func (cx *compiler) slugSet(s *sexp) (func(f *frame) *valueSet, typ, bool, error) {
	names, err := cx.slugNames(s, true)
	if err != nil {
		return nil, typ{}, false, err
	}
	policy := cx.policy
	return func(f *frame) *valueSet {
		out := newValueSet(false)
		for _, name := range names {
			out.add(value{kind: ID, s: f.run.slugged(name, policy)})
		}
		return out
	}, groupID, true, nil
}

// A query's slugs are all checked before it scans, so there an unresolved one never reaches a row;
// a policy's are resolved without the viewer's read checks, as the rest of a policy reads, and one naming no group stops the server.
func (r *run) slugged(slug string, policy bool) string {
	key := "slug\x00" + strconv.FormatBool(policy) + "\x00" + slug
	if found, ok := r.memo[key].(string); ok {
		return found
	}
	found := ""
	groups := r.table("GROUP")
	row, ok := groups.Distinct("slug", slug)
	switch {
	case policy && !ok:
		logging.Fatal("a policy's slug names no group", "slug", slug)
	case policy || ok && r.readable(groups.Table(), row):
		found = row["id"]
	}
	r.memo[key] = found
	return found
}
