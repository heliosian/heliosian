package db

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type atomKind int

const (
	atomName atomKind = iota
	atomAt
	atomString
	atomNumber
)

type sexp struct {
	list   []*sexp
	isList bool
	kind   atomKind
	text   string
	pos    int
}

func (s *sexp) head() string {
	if !s.isList || len(s.list) == 0 || s.list[0].isList || s.list[0].kind != atomName {
		return ""
	}
	return s.list[0].text
}

func (s *sexp) errorf(format string, args ...any) error {
	if s.pos < 0 {
		return fmt.Errorf(format, args...)
	}
	return fmt.Errorf("at %d: %s", s.pos+1, fmt.Sprintf(format, args...))
}

func isNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_.=!<>", r)
}

func readSexp(src string) (*sexp, error) {
	p := &reader{src: []rune(src)}
	p.space()
	if p.done() {
		return nil, fmt.Errorf("empty query")
	}
	s, err := p.read()
	if err != nil {
		return nil, err
	}
	p.space()
	if !p.done() {
		return nil, fmt.Errorf("at %d: text after the closing bracket", p.at+1)
	}
	if !s.isList {
		return nil, s.errorf("a query is a bracket")
	}
	return s, nil
}

func readForms(src string) ([]*sexp, error) {
	p := &reader{src: []rune(src)}
	out := []*sexp{}
	for {
		p.space()
		if p.done() {
			return out, nil
		}
		s, err := p.read()
		if err != nil {
			return nil, err
		}
		if !s.isList {
			return nil, s.errorf("%s stands outside a bracket", s.flat())
		}
		out = append(out, s)
	}
}

type reader struct {
	src []rune
	at  int
}

func (p *reader) done() bool {
	return p.at >= len(p.src)
}

func (p *reader) space() {
	for !p.done() {
		switch {
		case unicode.IsSpace(p.src[p.at]):
			p.at++
		case p.src[p.at] == ';':
			for !p.done() && p.src[p.at] != '\n' {
				p.at++
			}
		default:
			return
		}
	}
}

func (p *reader) read() (*sexp, error) {
	start := p.at
	r := p.src[p.at]
	switch {
	case r == '(':
		p.at++
		out := &sexp{isList: true, pos: start}
		for {
			p.space()
			if p.done() {
				return nil, fmt.Errorf("at %d: bracket never closed", start+1)
			}
			if p.src[p.at] == ')' {
				p.at++
				return out, nil
			}
			item, err := p.read()
			if err != nil {
				return nil, err
			}
			out.list = append(out.list, item)
		}
	case r == ')':
		return nil, fmt.Errorf("at %d: closing bracket with nothing open", start+1)
	case r == '"':
		p.at++
		var b strings.Builder
		for {
			if p.done() {
				return nil, fmt.Errorf("at %d: string never closed", start+1)
			}
			c := p.src[p.at]
			p.at++
			if c == '"' {
				return &sexp{kind: atomString, text: b.String(), pos: start}, nil
			}
			if c == '\\' {
				if p.done() {
					return nil, fmt.Errorf("at %d: string never closed", start+1)
				}
				c = p.src[p.at]
				p.at++
			}
			b.WriteRune(c)
		}
	case r == '-' || unicode.IsDigit(r):
		p.at++
		for !p.done() && (unicode.IsDigit(p.src[p.at]) || p.src[p.at] == '.') {
			p.at++
		}
		text := string(p.src[start:p.at])
		if _, err := strconv.ParseFloat(text, 64); err != nil || text == "-" {
			return nil, fmt.Errorf("at %d: %q is not a number", start+1, text)
		}
		return &sexp{kind: atomNumber, text: text, pos: start}, nil
	case r == '@':
		p.at++
		for !p.done() && isNameRune(p.src[p.at]) {
			p.at++
		}
		text := string(p.src[start+1 : p.at])
		if text == "" {
			return nil, fmt.Errorf("at %d: @ names nothing", start+1)
		}
		return &sexp{kind: atomAt, text: text, pos: start}, nil
	case isNameRune(r):
		for !p.done() && isNameRune(p.src[p.at]) {
			p.at++
		}
		return &sexp{kind: atomName, text: string(p.src[start:p.at]), pos: start}, nil
	}
	return nil, fmt.Errorf("at %d: unexpected %q", start+1, r)
}

const renderWidth = 80

func (s *sexp) flat() string {
	if !s.isList {
		switch s.kind {
		case atomString:
			return strconv.Quote(s.text)
		case atomAt:
			return "@" + s.text
		}
		return s.text
	}
	parts := []string{}
	for _, item := range s.list {
		parts = append(parts, item.flat())
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func headCount(head string) int {
	switch head {
	case "from", "exists", "count", "select":
		return 2
	case "sum":
		return 3
	}
	return 1
}

func (s *sexp) render(column int) string {
	flat := s.flat()
	if !s.isList || column+len(flat) <= renderWidth {
		return flat
	}
	n := min(headCount(s.head()), len(s.list))
	if n < len(s.list) && s.list[n].kind == atomAt && !s.list[n].isList && (s.head() == "from" || s.head() == "exists" || s.head() == "count" || s.head() == "sum") {
		n++
	}
	heads := []string{}
	for _, item := range s.list[:n] {
		heads = append(heads, item.flat())
	}
	line := "(" + strings.Join(heads, " ")
	rest := s.list[n:]
	if len(rest) == 0 {
		return line + ")"
	}
	if s.head() == "from" {
		indent := column + 2
		out := line
		for _, item := range rest {
			out += "\n" + strings.Repeat(" ", indent) + item.render(indent)
		}
		return out + ")"
	}
	indent := column + len(line) + 1
	out := line + " " + rest[0].render(indent)
	for _, item := range rest[1:] {
		out += "\n" + strings.Repeat(" ", indent) + item.render(indent)
	}
	return out + ")"
}

func (s *sexp) equal(o *sexp) bool {
	if s.isList != o.isList {
		return false
	}
	if !s.isList {
		return s.kind == o.kind && s.text == o.text
	}
	if len(s.list) != len(o.list) {
		return false
	}
	for i := range s.list {
		if !s.list[i].equal(o.list[i]) {
			return false
		}
	}
	return true
}
