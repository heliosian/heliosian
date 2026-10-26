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
	list []*sexp
	kind atomKind
	text string
	pos  int
}

func (s *sexp) isList() bool {
	return s.list != nil
}

func (s *sexp) head() string {
	if len(s.list) == 0 || s.list[0].isList() || s.list[0].kind != atomName {
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
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_.=!<>*", r)
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
	if !s.isList() {
		return nil, s.errorf("a query is a bracket")
	}
	return s, nil
}

type note struct {
	section string
	comment string
}

func readForms(src string) ([]*sexp, []note, error) {
	p := &reader{src: []rune(src)}
	out := []*sexp{}
	notes := []note{}
	section := ""
	for {
		p.comments = nil
		p.space()
		if p.done() {
			return out, notes, nil
		}
		comment := []string{}
		for _, line := range p.comments {
			if heading, ok := strings.CutPrefix(line, ";;"); ok {
				section, comment = strings.TrimSpace(heading), []string{}
				continue
			}
			comment = append(comment, strings.TrimSpace(strings.TrimPrefix(line, ";")))
		}
		s, err := p.read()
		if err != nil {
			return nil, nil, err
		}
		if !s.isList() {
			return nil, nil, s.errorf("%s stands outside a bracket", s.flat())
		}
		out = append(out, s)
		notes = append(notes, note{section: section, comment: strings.Join(comment, " ")})
	}
}

type reader struct {
	src      []rune
	at       int
	comments []string
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
			start := p.at
			for !p.done() && p.src[p.at] != '\n' {
				p.at++
			}
			p.comments = append(p.comments, string(p.src[start:p.at]))
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
		out := &sexp{list: []*sexp{}, pos: start}
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
	var out strings.Builder
	s.writeFlat(&out, nil)
	return out.String()
}

func (s *sexp) writeFlat(out *strings.Builder, spans map[*sexp][2]int) {
	start := out.Len()
	if s.isList() {
		out.WriteByte('(')
		for i, item := range s.list {
			if i > 0 {
				out.WriteByte(' ')
			}
			item.writeFlat(out, spans)
		}
		out.WriteByte(')')
	} else {
		switch s.kind {
		case atomString:
			out.WriteString(strconv.Quote(s.text))
		case atomAt:
			out.WriteByte('@')
			out.WriteString(s.text)
		default:
			out.WriteString(s.text)
		}
	}
	if spans != nil {
		spans[s] = [2]int{start, out.Len()}
	}
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
	spans := map[*sexp][2]int{}
	var flat strings.Builder
	s.writeFlat(&flat, spans)
	var out strings.Builder
	s.writeRendered(&out, column, flat.String(), spans)
	return out.String()
}

func (s *sexp) writeRendered(out *strings.Builder, column int, flat string, spans map[*sexp][2]int) {
	span := spans[s]
	if !s.isList() || column >= renderWidth || column+span[1]-span[0] <= renderWidth {
		out.WriteString(flat[span[0]:span[1]])
		return
	}
	n := min(headCount(s.head()), len(s.list))
	if n < len(s.list) && s.list[n].kind == atomAt && !s.list[n].isList() && (s.head() == "from" || s.head() == "exists" || s.head() == "count" || s.head() == "sum") {
		n++
	}
	width := 1
	out.WriteByte('(')
	for i, item := range s.list[:n] {
		if i > 0 {
			out.WriteByte(' ')
			width++
		}
		part := spans[item]
		out.WriteString(flat[part[0]:part[1]])
		width += part[1] - part[0]
	}
	rest := s.list[n:]
	indent := column + width + 1
	if s.head() == "from" {
		indent = column + 2
	}
	for i, item := range rest {
		if i > 0 || s.head() == "from" {
			out.WriteByte('\n')
			out.WriteString(strings.Repeat(" ", min(indent, renderWidth)))
		} else {
			out.WriteByte(' ')
		}
		item.writeRendered(out, indent, flat, spans)
	}
	out.WriteByte(')')
}
