package db

import (
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"
	"unicode"

	"heliosian/internal/cells"
	"heliosian/internal/store"
)

type Query struct {
	tree    *sexp
	scan    *scan
	order   []orderKey
	limit   int
	include []*pathSpec
}

type Env struct {
	Viewer string
	System string
	Now    time.Time
}

type Result struct {
	Table     string
	IDs       []string
	Resources map[string]map[string]store.Row
}

func (r Result) Rows() []store.Row {
	out := []store.Row{}
	for _, id := range r.IDs {
		out = append(out, r.Resources[r.Table][id])
	}
	return out
}

type scope struct {
	table *Table
	name  string
	outer *scope
}

type frame struct {
	table *Table
	row   store.Row
	name  string
	outer *frame
	run   *run
}

type run struct {
	m       *Model
	whole   bool
	viewer  store.Row
	system  string
	now     time.Time
	rows    map[string]bool
	columns map[string]bool
	exists  map[*scan]bool
	selects map[*scan]*valueSet
}

type operand struct {
	t      typ
	local  bool
	direct string
	lit    *sexp
	eval   func(f *frame) value
}

type cond struct {
	eval     func(f *frame) bool
	probeCol string
	probe    *operand
}

type scan struct {
	table    *Table
	name     string
	guarded  bool
	constant bool
	conds    []cond
	probeCol string
	probe    *operand
}

type orderKey struct {
	path *operand
	desc bool
}

type pathSpec struct {
	steps []Column
}

type define struct {
	params []string
	body   *sexp
}

type compiler struct {
	policy  bool
	defines map[string]*define
	used    map[string]bool
	hidden  int
	touched []*scope
}

func newCompiler(policy bool) *compiler {
	return &compiler{policy: policy, defines: definitions, used: map[string]bool{}}
}

var keywords = []string{"true", "false", "today", "now", "asc", "desc"}

func Parse(src string) (*Query, error) {
	tree, err := readSexp(src)
	if err != nil {
		return nil, err
	}
	return newCompiler(false).query(tree)
}

func (cx *compiler) query(tree *sexp) (*Query, error) {
	if tree.head() != "from" {
		return nil, tree.errorf("a query starts with from")
	}
	q := &Query{tree: tree}
	s, rest, err := cx.scanHead(tree, 1)
	if err != nil {
		return nil, err
	}
	q.scan = s
	sc := &scope{table: s.table, name: s.name}
	for _, part := range rest {
		switch part.head() {
		case "where":
			conds, err := cx.conds(part.list[1:], sc)
			if err != nil {
				return nil, err
			}
			s.conds = append(s.conds, conds...)
		case "order":
			args := part.list[1:]
			if len(args) == 0 || len(args)%2 != 0 {
				return nil, part.errorf("order takes pairs of a path and asc or desc")
			}
			for i := 0; i < len(args); i += 2 {
				p, err := cx.path(args[i], sc)
				if err != nil {
					return nil, err
				}
				dir := args[i+1]
				if dir.isList || dir.kind != atomName || (dir.text != "asc" && dir.text != "desc") {
					return nil, dir.errorf("order wants asc or desc after each path")
				}
				q.order = append(q.order, orderKey{path: &p, desc: dir.text == "desc"})
			}
		case "limit":
			if len(part.list) != 2 || part.list[1].isList || part.list[1].kind != atomNumber {
				return nil, part.errorf("limit takes one number")
			}
			n, ok := new(big.Int).SetString(part.list[1].text, 10)
			if !ok || n.Sign() <= 0 || !n.IsInt64() {
				return nil, part.errorf("limit wants a whole number above zero")
			}
			q.limit = int(n.Int64())
		case "include":
			for _, item := range part.list[1:] {
				p, err := compileInclude(item, s.table)
				if err != nil {
					return nil, err
				}
				q.include = append(q.include, p)
			}
		default:
			return nil, part.errorf("a query's parts are where, order, limit and include")
		}
	}
	s.pickProbe()
	return q, nil
}

func (q *Query) String() string {
	return q.tree.render(0)
}

func (cx *compiler) scanHead(s *sexp, at int) (*scan, []*sexp, error) {
	if len(s.list) <= at || s.list[at].isList || s.list[at].kind != atomName {
		return nil, nil, s.errorf("%s needs a table", s.head())
	}
	t, err := tableNamed(s.list[at])
	if err != nil {
		return nil, nil, err
	}
	out := &scan{table: t, guarded: !cx.policy}
	rest := s.list[at+1:]
	if len(rest) > 0 && !rest[0].isList && rest[0].kind == atomAt {
		out.name = rest[0].text
		if out.name == "viewer" {
			return nil, nil, rest[0].errorf("@viewer is the viewer and can't name a row")
		}
		rest = rest[1:]
	}
	if out.name == "" {
		cx.hidden++
		out.name = fmt.Sprintf("_%d", cx.hidden)
	}
	return out, rest, nil
}

func tableNamed(s *sexp) (*Table, error) {
	t, ok := Lookup(s.text)
	if !ok {
		names := []string{}
		for _, t := range Tables {
			names = append(names, t.Name)
		}
		return nil, s.errorf("no table %s%s", s.text, suggest(s.text, names))
	}
	return t, nil
}

func (cx *compiler) scan(s *sexp, at int, outer *scope) (*scan, error) {
	out, rest, err := cx.scanHead(s, at)
	if err != nil {
		return nil, err
	}
	inner := &scope{table: out.table, name: out.name, outer: outer}
	mark := len(cx.touched)
	out.conds, err = cx.conds(rest, inner)
	if err != nil {
		return nil, err
	}
	out.constant = cx.closed(mark, inner)
	out.pickProbe()
	return out, nil
}

func (cx *compiler) closed(mark int, inner *scope) bool {
	for _, at := range cx.touched[mark:] {
		for at != nil && at != inner {
			at = at.outer
		}
		if at == nil {
			return false
		}
	}
	return true
}

func (s *scan) pickProbe() {
	for _, c := range s.conds {
		if c.probe != nil {
			s.probeCol, s.probe = c.probeCol, c.probe
			return
		}
	}
}

func (s *scan) candidates(f *frame) []store.Row {
	m := f.run.m
	if s.probe == nil {
		if s.table.Generated {
			return m.generated(s.table).rows
		}
		return f.run.table(s.table.Name).All()
	}
	v := s.probe.eval(f)
	if v.blank {
		return nil
	}
	if s.table.Generated {
		if s.table.Name == "EFFECTIVE_MEMBER" && s.probeCol == "group" {
			return m.effectiveRows(v.s)
		}
		return m.generated(s.table).by[s.probeCol][v.s]
	}
	rows := f.run.table(s.table.Name)
	if s.probeCol == "id" {
		if row, ok := rows.Get(v.s); ok {
			return []store.Row{row}
		}
		return nil
	}
	return rows.Referencing(s.probeCol, v.s)
}

func (s *scan) each(f *frame, fn func(inner *frame) bool) {
	for _, row := range s.candidates(f) {
		if s.guarded && !f.run.readable(s.table, row) {
			continue
		}
		inner := &frame{table: s.table, row: row, name: s.name, outer: f, run: f.run}
		if s.matches(inner) && !fn(inner) {
			return
		}
	}
}

func (s *scan) matches(f *frame) bool {
	for _, c := range s.conds {
		if !c.eval(f) {
			return false
		}
	}
	return true
}

func (cx *compiler) conds(items []*sexp, sc *scope) ([]cond, error) {
	out := []cond{}
	for _, item := range items {
		c, err := cx.cond(item, sc)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (cx *compiler) cond(s *sexp, sc *scope) (cond, error) {
	if !s.isList {
		if s.kind == atomName && (s.text == "true" || s.text == "false") {
			holds := s.text == "true"
			return cond{eval: func(*frame) bool { return holds }}, nil
		}
		p, err := cx.path(s, sc)
		if err != nil {
			return cond{}, err
		}
		if p.t.class != classBool {
			return cond{}, s.errorf("%s is %s, not true or false", s.flat(), p.t)
		}
		return cond{eval: func(f *frame) bool { v := p.eval(f); return !v.blank && v.b }}, nil
	}
	args := s.list[1:]
	switch op := s.head(); op {
	case "and", "or":
		conds, err := cx.conds(args, sc)
		if err != nil {
			return cond{}, err
		}
		if len(conds) == 0 {
			return cond{}, s.errorf("%s needs conditions", op)
		}
		if op == "and" {
			return cond{eval: func(f *frame) bool {
				for _, c := range conds {
					if !c.eval(f) {
						return false
					}
				}
				return true
			}}, nil
		}
		return cond{eval: func(f *frame) bool {
			for _, c := range conds {
				if c.eval(f) {
					return true
				}
			}
			return false
		}}, nil
	case "not":
		if len(args) != 1 {
			return cond{}, s.errorf("not takes one condition")
		}
		c, err := cx.cond(args[0], sc)
		if err != nil {
			return cond{}, err
		}
		return cond{eval: func(f *frame) bool { return !c.eval(f) }}, nil
	case "=", "!=", "<", "<=", ">", ">=":
		return cx.compare(s, op, sc)
	case "in":
		return cx.in(s, sc)
	case "blank":
		if len(args) != 1 {
			return cond{}, s.errorf("blank takes one path")
		}
		p, err := cx.path(args[0], sc)
		if err != nil {
			return cond{}, err
		}
		return cond{eval: func(f *frame) bool { return p.eval(f).blank }}, nil
	case "exists":
		inner, err := cx.scan(s, 1, sc)
		if err != nil {
			return cond{}, err
		}
		return cond{eval: func(f *frame) bool {
			if found, ok := f.run.exists[inner]; ok {
				return found
			}
			found := false
			inner.each(f, func(*frame) bool { found = true; return false })
			if inner.constant {
				f.run.exists[inner] = found
			}
			return found
		}}, nil
	case "system":
		if len(args) != 1 || args[0].isList || args[0].kind != atomString {
			return cond{}, s.errorf("system takes one quoted name")
		}
		name := args[0].text
		return cond{eval: func(f *frame) bool { return f.run.system == name }}, nil
	case "":
		return cond{}, s.errorf("a condition starts with its operator")
	default:
		if d, ok := cx.defines[op]; ok {
			expanded, err := cx.expand(s, d, sc)
			if err != nil {
				return cond{}, err
			}
			return cx.cond(expanded, sc)
		}
		return cond{}, s.errorf("no condition %s", op)
	}
}

func (cx *compiler) expand(call *sexp, d *define, sc *scope) (*sexp, error) {
	args := call.list[1:]
	if len(args) != len(d.params) {
		return nil, call.errorf("%s takes %d arguments", call.head(), len(d.params))
	}
	cx.used[call.head()] = true
	bound := map[string]*sexp{}
	for i, p := range d.params {
		bound[p] = anchor(args[i], sc)
	}
	return substitute(d.body, bound)
}

func anchor(arg *sexp, sc *scope) *sexp {
	if arg.isList {
		if slices.Contains([]string{"exists", "count", "sum", "select"}, arg.head()) {
			return arg
		}
		out := &sexp{isList: true, pos: arg.pos, list: []*sexp{arg.list[0]}}
		for _, item := range arg.list[1:] {
			out.list = append(out.list, anchor(item, sc))
		}
		return out
	}
	if arg.kind != atomName || slices.Contains(keywords, arg.text) || unicode.IsUpper([]rune(arg.text)[0]) || sc == nil || sc.name == "" {
		return arg
	}
	return &sexp{kind: atomAt, text: sc.name + "." + arg.text, pos: arg.pos}
}

func substitute(body *sexp, bound map[string]*sexp) (*sexp, error) {
	if body.isList {
		out := &sexp{isList: true, pos: body.pos}
		for i, item := range body.list {
			if i == 0 && !item.isList {
				out.list = append(out.list, item)
				continue
			}
			next, err := substitute(item, bound)
			if err != nil {
				return nil, err
			}
			out.list = append(out.list, next)
		}
		return out, nil
	}
	switch body.kind {
	case atomAt:
		name, rest, dotted := strings.Cut(body.text, ".")
		arg, ok := bound["@"+name]
		if !ok {
			return body, nil
		}
		if !dotted {
			return arg, nil
		}
		if arg.isList || arg.kind != atomAt {
			return nil, body.errorf("@%s is %s, which has no columns to follow", name, arg.flat())
		}
		return &sexp{kind: atomAt, text: arg.text + "." + rest, pos: body.pos}, nil
	case atomName:
		if arg, ok := bound[body.text]; ok {
			return arg, nil
		}
	}
	return body, nil
}

func (cx *compiler) compare(s *sexp, op string, sc *scope) (cond, error) {
	if len(s.list) != 3 {
		return cond{}, s.errorf("%s compares two things", op)
	}
	a, err := cx.operand(s.list[1], sc)
	if err != nil {
		return cond{}, err
	}
	b, err := cx.operand(s.list[2], sc)
	if err != nil {
		return cond{}, err
	}
	if a.lit != nil && b.lit != nil {
		return cond{}, s.errorf("%s compares two literals", op)
	}
	if a.lit != nil {
		if a, err = coerce(a.lit, b.t); err != nil {
			return cond{}, err
		}
	}
	if b.lit != nil {
		if b, err = coerce(b.lit, a.t); err != nil {
			return cond{}, err
		}
	}
	if err := comparable(s, a.t, b.t); err != nil {
		return cond{}, err
	}
	ordered := op != "=" && op != "!="
	if ordered && (a.t.class == classRow || a.t.class == classBool) {
		return cond{}, s.errorf("%s orders numbers, dates and text, not %s", op, a.t)
	}
	out := cond{}
	switch op {
	case "=":
		out.eval = func(f *frame) bool { return equalValues(a.eval(f), b.eval(f)) }
		if a.direct != "" && !b.local && a.t.class == classRow {
			out.probeCol, out.probe = a.direct, &b
		} else if b.direct != "" && !a.local && b.t.class == classRow {
			out.probeCol, out.probe = b.direct, &a
		}
	case "!=":
		out.eval = func(f *frame) bool {
			x, y := a.eval(f), b.eval(f)
			if x.blank && y.blank {
				return false
			}
			return !equalValues(x, y)
		}
	default:
		out.eval = func(f *frame) bool {
			n, ok := compareValues(a.eval(f), b.eval(f))
			if !ok {
				return false
			}
			switch op {
			case "<":
				return n < 0
			case "<=":
				return n <= 0
			case ">":
				return n > 0
			}
			return n >= 0
		}
	}
	return out, nil
}

func comparable(s *sexp, a, b typ) error {
	if a.class != b.class {
		return s.errorf("can't compare %s with %s", a, b)
	}
	if a.class == classRow && a.table != "" && b.table != "" && a.table != b.table {
		return s.errorf("can't compare %s with %s", a, b)
	}
	return nil
}

func (cx *compiler) in(s *sexp, sc *scope) (cond, error) {
	if len(s.list) < 3 {
		return cond{}, s.errorf("in takes a value and what it may be")
	}
	x, err := cx.operand(s.list[1], sc)
	if err != nil {
		return cond{}, err
	}
	if x.lit != nil {
		return cond{}, s.list[1].errorf("in tests a path, not a literal")
	}
	if len(s.list) == 3 && s.list[2].isList {
		set, t, err := cx.set(s.list[2], sc, x.t)
		if err != nil {
			return cond{}, err
		}
		if err := comparable(s, x.t, t); err != nil {
			return cond{}, err
		}
		return cond{eval: func(f *frame) bool { return set(f).has(x.eval(f)) }}, nil
	}
	options := []value{}
	for _, item := range s.list[2:] {
		if item.isList {
			return cond{}, item.errorf("in takes literals or one select")
		}
		o, err := coerce(item, x.t)
		if err != nil {
			return cond{}, err
		}
		options = append(options, o.eval(nil))
	}
	return cond{eval: func(f *frame) bool {
		v := x.eval(f)
		for _, o := range options {
			if equalValues(v, o) {
				return true
			}
		}
		return false
	}}, nil
}

func (cx *compiler) set(s *sexp, sc *scope, x typ) (func(f *frame) *valueSet, typ, error) {
	switch head := s.head(); head {
	case "select":
		return cx.selectSet(s, sc, x)
	case "ancestors":
		if len(s.list) != 2 {
			return nil, typ{}, s.errorf("ancestors takes one row")
		}
		g, err := cx.operand(s.list[1], sc)
		if err != nil {
			return nil, typ{}, err
		}
		if g.lit != nil || g.t.class != classRow || g.t.table == "" {
			return nil, typ{}, s.errorf("ancestors takes a row")
		}
		table, _ := Lookup(g.t.table)
		if parent, ok := table.Column("parent"); !ok || parent.Kind != Ref || parent.Target != table.Name {
			return nil, typ{}, s.errorf("%s has no parent to follow", table.Name)
		}
		t := typ{class: classRow, kind: ID, table: table.Name}
		return func(f *frame) *valueSet {
			out := newValueSet(false)
			v := g.eval(f)
			if v.blank {
				return out
			}
			for id := v.s; id != "" && !out.keys[id]; {
				out.add(value{kind: ID, s: id})
				row, ok := f.run.table(table.Name).Get(id)
				if !ok {
					break
				}
				id = row["parent"]
			}
			return out
		}, t, nil
	default:
		if d, ok := cx.defines[head]; ok {
			expanded, err := cx.expand(s, d, sc)
			if err != nil {
				return nil, typ{}, err
			}
			return cx.set(expanded, sc, x)
		}
	}
	return nil, typ{}, s.errorf("in takes literals or one select")
}

func (cx *compiler) selectSet(s *sexp, sc *scope, x typ) (func(f *frame) *valueSet, typ, error) {
	if len(s.list) < 2 || s.list[1].isList || s.list[1].kind != atomName {
		return nil, typ{}, s.errorf("select names TABLE.column")
	}
	tableName, columnName, ok := strings.Cut(s.list[1].text, ".")
	if !ok {
		return nil, typ{}, s.list[1].errorf("select names TABLE.column")
	}
	t, err := tableNamed(&sexp{text: tableName, pos: s.list[1].pos})
	if err != nil {
		return nil, typ{}, err
	}
	c, err := columnNamed(t, columnName, s.list[1])
	if err != nil {
		return nil, typ{}, err
	}
	inner := &scan{table: t, guarded: !cx.policy}
	cx.hidden++
	inner.name = fmt.Sprintf("_%d", cx.hidden)
	within := &scope{table: t, name: inner.name, outer: sc}
	mark := len(cx.touched)
	inner.conds, err = cx.conds(s.list[2:], within)
	if err != nil {
		return nil, typ{}, err
	}
	inner.constant = cx.closed(mark, within)
	inner.pickProbe()
	guarded := !cx.policy
	ct := columnType(t.Name, c)
	exact := x.kind == Order && ct.kind == Order
	return func(f *frame) *valueSet {
		if out, ok := f.run.selects[inner]; ok {
			return out
		}
		out := newValueSet(exact)
		inner.each(f, func(g *frame) bool {
			out.add(g.run.cell(guarded, t, g.row, c))
			return true
		})
		if inner.constant {
			f.run.selects[inner] = out
		}
		return out
	}, ct, nil
}

func (cx *compiler) operand(s *sexp, sc *scope) (operand, error) {
	if !s.isList {
		switch s.kind {
		case atomString, atomNumber:
			return operand{lit: s}, nil
		case atomName:
			switch s.text {
			case "true", "false":
				return operand{lit: s}, nil
			case "today":
				return operand{t: typ{class: classTime, kind: Date}, eval: func(f *frame) value {
					y, mo, d := f.run.now.Date()
					return value{kind: Date, s: "today", t: time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)}
				}}, nil
			case "now":
				return operand{t: typ{class: classTime, kind: Moment}, eval: func(f *frame) value {
					return value{kind: Moment, s: "now", t: f.run.now}
				}}, nil
			}
		}
		return cx.path(s, sc)
	}
	switch s.head() {
	case "count":
		inner, err := cx.scan(s, 1, sc)
		if err != nil {
			return operand{}, err
		}
		return operand{t: typ{class: classNumber, kind: Int}, eval: func(f *frame) value {
			n := int64(0)
			inner.each(f, func(*frame) bool { n++; return true })
			return value{kind: Int, num: new(big.Rat).SetInt64(n)}
		}}, nil
	case "sum":
		if len(s.list) < 3 {
			return operand{}, s.errorf("sum takes a path and a table")
		}
		inner, err := cx.scan(s, 2, sc)
		if err != nil {
			return operand{}, err
		}
		p, err := cx.path(s.list[1], &scope{table: inner.table, name: inner.name, outer: sc})
		if err != nil {
			return operand{}, err
		}
		if p.t.class != classNumber {
			return operand{}, s.list[1].errorf("sum adds numbers, and %s is %s", s.list[1].flat(), p.t)
		}
		return operand{t: p.t, eval: func(f *frame) value {
			total := new(big.Rat)
			inner.each(f, func(g *frame) bool {
				if v := p.eval(g); !v.blank {
					total.Add(total, v.num)
				}
				return true
			})
			return value{kind: p.t.kind, num: total}
		}}, nil
	}
	return operand{}, s.errorf("a value is a path, a literal, today, now, count or sum")
}

func coerce(lit *sexp, t typ) (operand, error) {
	v := value{kind: t.kind, s: lit.text}
	switch {
	case lit.kind == atomName && (lit.text == "true" || lit.text == "false"):
		if t.class != classBool {
			return operand{}, lit.errorf("%s is true or false, and wants %s", lit.text, t)
		}
		v.b = lit.text == "true"
	case lit.kind == atomNumber:
		if t.class != classNumber {
			return operand{}, lit.errorf("%s is a number, and wants %s", lit.text, t)
		}
		n, ok := new(big.Rat).SetString(lit.text)
		if !ok {
			return operand{}, lit.errorf("%s is not a number", lit.text)
		}
		v.num = n
	case lit.kind == atomString:
		switch t.class {
		case classText:
			if t.values != nil && !slices.ContainsFunc(t.values, func(s string) bool { return strings.EqualFold(s, lit.text) }) {
				return operand{}, lit.errorf("%q is not one of %s", lit.text, strings.Join(t.values, ", "))
			}
		case classRow:
			table, ok := TableOf(lit.text)
			if !ok {
				return operand{}, lit.errorf("%q is not an id", lit.text)
			}
			if t.table != "" && table != t.table {
				return operand{}, lit.errorf("%q is a %s id, and wants %s", lit.text, table, t)
			}
		case classTime:
			when, err := cells.When(lit.text)
			if err != nil {
				return operand{}, lit.errorf("%q is not a date like 2026-09-24 or a moment like 2026-09-24 16:00", lit.text)
			}
			if t.kind == Date && len(lit.text) != len(time.DateOnly) {
				return operand{}, lit.errorf("%q is not a date like 2026-09-24", lit.text)
			}
			v.t = when
		default:
			return operand{}, lit.errorf("%q is text, and wants %s", lit.text, t)
		}
	default:
		return operand{}, lit.errorf("%s is not a literal", lit.flat())
	}
	return operand{t: t, eval: func(*frame) value { return v }}, nil
}

func columnNamed(t *Table, name string, at *sexp) (Column, error) {
	if c, ok := t.Column(name); ok {
		return c, nil
	}
	names := []string{}
	for _, c := range t.Columns {
		names = append(names, c.Name)
	}
	return Column{}, at.errorf("no column %s on %s%s", name, t.Name, suggest(name, names))
}

func (cx *compiler) path(s *sexp, sc *scope) (operand, error) {
	if s.isList || (s.kind != atomName && s.kind != atomAt) {
		return operand{}, s.errorf("%s is not a path", s.flat())
	}
	text := s.text
	var start *Table
	var startName string
	local := true
	if s.kind == atomAt {
		name, rest, _ := strings.Cut(text, ".")
		text = rest
		startName = name
		if name == "viewer" {
			start, _ = Lookup("PERSON")
			local = false
		} else {
			found := false
			for at := sc; at != nil; at = at.outer {
				if at.name == name {
					start, found = at.table, true
					local = at == sc
					cx.touched = append(cx.touched, at)
					break
				}
			}
			if !found {
				return operand{}, s.errorf("no row named @%s", name)
			}
		}
	} else {
		if r := []rune(text); unicode.IsUpper(r[0]) {
			return operand{}, s.errorf("%s is a table, not a value", text)
		}
		if sc == nil {
			return operand{}, s.errorf("%s names no row", text)
		}
		start = sc.table
		startName = sc.name
		cx.touched = append(cx.touched, sc)
	}
	segments := []string{}
	if text != "" {
		segments = strings.Split(text, ".")
	}
	steps := []Column{}
	table := start
	for i, seg := range segments {
		c, err := columnNamed(table, seg, s)
		if err != nil {
			return operand{}, err
		}
		steps = append(steps, c)
		if i == len(segments)-1 {
			break
		}
		if c.Kind != Ref || c.Target == "" {
			return operand{}, s.errorf("%s.%s is not a reference to follow", table.Name, seg)
		}
		table, _ = Lookup(c.Target)
	}
	out := operand{local: local}
	if len(steps) == 0 {
		out.t = typ{class: classRow, kind: ID, table: start.Name}
	} else {
		out.t = columnType(table.Name, steps[len(steps)-1])
	}
	if local && len(steps) == 1 {
		out.direct = steps[0].Name
	}
	at := s.kind == atomAt
	guarded := !cx.policy
	out.eval = func(f *frame) value {
		row, t := startRow(f, at, startName)
		if row == nil {
			return blankValue
		}
		if len(steps) == 0 {
			return value{kind: ID, s: row["id"]}
		}
		for _, c := range steps[:len(steps)-1] {
			target := f.run.table(c.Target)
			next, ok := target.Get(row[c.Name])
			if !ok || (guarded && !f.run.readable(target.Table(), next)) {
				return blankValue
			}
			row, t = next, target.Table()
		}
		return f.run.cell(guarded, t, row, steps[len(steps)-1])
	}
	return out, nil
}

func startRow(f *frame, at bool, name string) (store.Row, *Table) {
	if !at {
		return f.row, f.table
	}
	if name == "viewer" {
		person, _ := Lookup("PERSON")
		return f.run.viewer, person
	}
	for g := f; g != nil; g = g.outer {
		if g.name == name {
			return g.row, g.table
		}
	}
	return nil, nil
}

func compileInclude(s *sexp, start *Table) (*pathSpec, error) {
	if s.isList || s.kind != atomName {
		return nil, s.errorf("include takes paths")
	}
	out := &pathSpec{}
	table := start
	for _, seg := range strings.Split(s.text, ".") {
		c, err := columnNamed(table, seg, s)
		if err != nil {
			return nil, err
		}
		if c.Kind != Ref || c.Target == "" {
			return nil, s.errorf("%s.%s is not a reference to include", table.Name, seg)
		}
		out.steps = append(out.steps, c)
		table, _ = Lookup(c.Target)
	}
	return out, nil
}

func (m *Model) newRun(env Env) *run {
	r := &run{m: m, whole: env.System == importReader, system: env.System, now: wallClock(env.Now), rows: map[string]bool{}, columns: map[string]bool{}, exists: map[*scan]bool{}, selects: map[*scan]*valueSet{}}
	if env.Viewer != "" {
		r.viewer, _ = r.table("PERSON").Get(env.Viewer)
	}
	return r
}

func (r *run) table(name string) *View {
	return r.m.view(name, r.whole)
}

func (m *Model) Run(q *Query, env Env) Result {
	r := m.newRun(env)
	top := &frame{run: r}
	rows := []store.Row{}
	q.scan.each(top, func(f *frame) bool {
		rows = append(rows, f.row)
		return true
	})
	if len(q.order) > 0 {
		rows = q.sorted(rows, top)
	}
	if q.limit > 0 && len(rows) > q.limit {
		rows = rows[:q.limit]
	}
	table := q.scan.table.Name
	out := Result{Table: table, IDs: []string{}, Resources: map[string]map[string]store.Row{table: {}}}
	guarded := q.scan.guarded
	for _, row := range rows {
		if guarded {
			row = r.redact(q.scan.table, row)
		}
		out.IDs = append(out.IDs, row["id"])
		out.Resources[table][row["id"]] = row
		for _, p := range q.include {
			r.includePath(guarded, row, p.steps, out.Resources)
		}
	}
	return out
}

func (q *Query) sorted(rows []store.Row, top *frame) []store.Row {
	type keyed struct {
		row  store.Row
		keys []value
	}
	all := []keyed{}
	for _, row := range rows {
		f := &frame{table: q.scan.table, row: row, name: q.scan.name, outer: top, run: top.run}
		k := keyed{row: row}
		for _, o := range q.order {
			k.keys = append(k.keys, o.path.eval(f))
		}
		all = append(all, k)
	}
	slices.SortStableFunc(all, func(a, b keyed) int {
		for i, o := range q.order {
			x, y := a.keys[i], b.keys[i]
			switch {
			case x.blank && y.blank:
				continue
			case x.blank:
				return 1
			case y.blank:
				return -1
			}
			n, _ := compareValues(x, y)
			if o.desc {
				n = -n
			}
			if n != 0 {
				return n
			}
		}
		return 0
	})
	out := []store.Row{}
	for _, k := range all {
		out = append(out, k.row)
	}
	return out
}

func (r *run) includePath(guarded bool, row store.Row, steps []Column, into map[string]map[string]store.Row) {
	if len(steps) == 0 {
		return
	}
	c := steps[0]
	target := r.table(c.Target)
	id := row[c.Name]
	next, ok := target.Get(id)
	if id == "" || !ok || (guarded && !r.readable(target.Table(), next)) {
		return
	}
	if guarded {
		next = r.redact(target.Table(), next)
	}
	if into[c.Target] == nil {
		into[c.Target] = map[string]store.Row{}
	}
	into[c.Target][id] = next
	r.includePath(guarded, next, steps[1:], into)
}

func wallClock(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
}

func suggest(name string, options []string) string {
	best, bestDistance := "", 3
	for _, o := range options {
		if d := distance(strings.ToLower(name), strings.ToLower(o)); d < bestDistance {
			best, bestDistance = o, d
		}
	}
	if best == "" {
		return ""
	}
	return "; did you mean " + best + "?"
}

func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
