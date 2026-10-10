package db

import (
	"math/big"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/store"
)

type class int

const (
	classText class = iota
	classNumber
	classTime
	classBool
	classRow
)

type typ struct {
	class  class
	kind   Kind
	table  string
	values []string
}

func (t typ) String() string {
	switch t.class {
	case classNumber:
		return "a number"
	case classTime:
		return "a date"
	case classBool:
		return "true or false"
	case classRow:
		if t.table == "" {
			return "a row"
		}
		return "a " + t.table
	}
	return "text"
}

func classOf(k Kind) class {
	switch k {
	case Int, Money, Float:
		return classNumber
	case Date, Moment:
		return classTime
	case Bool:
		return classBool
	case ID, Ref:
		return classRow
	}
	return classText
}

func columnType(table string, c Column) typ {
	t := typ{class: classOf(c.Kind), kind: c.Kind}
	if c.Kind == Enum {
		t.values = c.ValueNames()
	}
	switch c.Kind {
	case ID:
		t.table = table
	case Ref:
		t.table = c.Target
	}
	return t
}

type value struct {
	blank bool
	kind  Kind
	s     string
	num   *big.Rat
	t     time.Time
	b     bool
}

var blankValue = value{blank: true}

func cellValue(c Column, cell string) value {
	s := strings.TrimSpace(cell)
	if s == "" {
		return blankValue
	}
	v := value{kind: c.Kind, s: s}
	switch classOf(c.Kind) {
	case classNumber:
		n, ok := new(big.Rat).SetString(s)
		if !ok {
			return blankValue
		}
		v.num = n
	case classTime:
		t, err := cells.When(s)
		if err != nil {
			return blankValue
		}
		v.t = t
	case classBool:
		b, err := cells.YesNo(s, false)
		if err != nil {
			return blankValue
		}
		v.b = b
	}
	return v
}

func compareValues(a, b value) (int, bool) {
	if a.blank || b.blank {
		return 0, false
	}
	switch {
	case a.num != nil && b.num != nil:
		return a.num.Cmp(b.num), true
	case !a.t.IsZero() || !b.t.IsZero():
		return a.t.Compare(b.t), true
	case a.kind == Order && b.kind == Order:
		return store.CompareKeys(a.s, b.s), true
	case classOf(a.kind) == classRow || classOf(b.kind) == classRow:
		return strings.Compare(a.s, b.s), true
	case classOf(a.kind) == classBool:
		if a.b == b.b {
			return 0, true
		}
		if a.b {
			return 1, true
		}
		return -1, true
	}
	return strings.Compare(strings.ToLower(a.s), strings.ToLower(b.s)), true
}

type valueSet struct {
	exact bool
	keys  map[string]int
}

func newValueSet(exact bool) *valueSet {
	return &valueSet{exact: exact, keys: map[string]int{}}
}

func (s *valueSet) key(v value) string {
	switch {
	case v.num != nil:
		return v.num.RatString()
	case !v.t.IsZero():
		return strconv.FormatInt(v.t.UnixNano(), 10)
	case s.exact || classOf(v.kind) == classRow:
		return v.s
	case classOf(v.kind) == classBool:
		return strconv.FormatBool(v.b)
	}
	return strings.ToLower(v.s)
}

func (s *valueSet) add(v value) {
	if v.blank {
		return
	}
	s.keys[s.key(v)]++
}

func (s *valueSet) has(v value) bool {
	return s.count(v) > 0
}

func (s *valueSet) count(v value) int {
	if v.blank {
		return 0
	}
	return s.keys[s.key(v)]
}

func equalValues(a, b value) bool {
	n, ok := compareValues(a, b)
	return ok && n == 0
}
