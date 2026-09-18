package artifacts

import (
	"strings"
	"unicode"
)

const (
	maxChunkChars = 6000
	pathSeparator = " › "
)

func Chunks(markdown string) []Chunk {
	out := []Chunk{}
	path := []string{}
	body := &strings.Builder{}
	emit := func() {
		for _, piece := range pieces(strings.TrimSpace(body.String())) {
			out = append(out, Chunk{Section: strings.Join(path, pathSeparator), Text: piece})
		}
		body.Reset()
	}
	for _, line := range strings.Split(markdown, "\n") {
		if level, text, ok := heading(line); ok {
			emit()
			// A mailer that marks every heading h1 still sets its sections
			// in capitals, so a heading in mixed case sits inside the one
			// in capitals before it at the same level.
			if !capitals(text) && level <= len(path) && capitals(path[level-1]) {
				level++
			}
			if level <= len(path) {
				path = path[:level-1]
			}
			for len(path) < level-1 {
				path = append(path, "")
			}
			path = append(path, text)
			path = compact(path)
			continue
		}
		body.WriteString(line + "\n")
	}
	emit()
	return out
}

func heading(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level == len(line) || line[level] != ' ' {
		return 0, "", false
	}
	text := strings.TrimSpace(line[level+1:])
	if text == "" {
		return 0, "", false
	}
	return level, text, true
}

func capitals(text string) bool {
	letters := false
	for _, r := range text {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			letters = true
		}
	}
	return letters
}

func compact(path []string) []string {
	out := []string{}
	for _, p := range path {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func pieces(text string) []string {
	if text == "" {
		return nil
	}
	if len(text) <= maxChunkChars {
		return []string{text}
	}
	out := []string{}
	current := &strings.Builder{}
	for _, paragraph := range strings.Split(text, "\n\n") {
		for _, part := range lines(paragraph) {
			if current.Len() > 0 && current.Len()+len(part)+2 > maxChunkChars {
				out = append(out, strings.TrimSpace(current.String()))
				current.Reset()
			}
			if current.Len() > 0 {
				current.WriteString("\n\n")
			}
			current.WriteString(part)
		}
	}
	if current.Len() > 0 {
		out = append(out, strings.TrimSpace(current.String()))
	}
	return out
}

func lines(paragraph string) []string {
	if len(paragraph) <= maxChunkChars {
		return []string{paragraph}
	}
	out := []string{}
	for _, line := range strings.Split(paragraph, "\n") {
		for len(line) > maxChunkChars {
			out = append(out, line[:maxChunkChars])
			line = line[maxChunkChars:]
		}
		out = append(out, line)
	}
	return out
}
