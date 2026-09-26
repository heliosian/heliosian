package mail

import (
	"bytes"
	netmail "net/mail"
	"strconv"
	"strings"
)

type HeaderLine struct {
	Name string
	Raw  string
}

func (h HeaderLine) Value() string {
	_, rest, _ := strings.Cut(h.Raw, ":")
	rest = strings.ReplaceAll(rest, "\r\n\t", " ")
	rest = strings.ReplaceAll(rest, "\r\n ", " ")
	return strings.TrimSpace(rest)
}

func SplitMessage(raw []byte) ([]HeaderLine, []byte) {
	lines := []HeaderLine{}
	rest := raw
	for len(rest) > 0 {
		var line []byte
		if i := bytes.IndexByte(rest, '\n'); i < 0 {
			line, rest = rest, nil
		} else {
			line, rest = rest[:i], rest[i+1:]
		}
		line = bytes.TrimRight(line, "\r")
		if len(line) == 0 {
			break
		}
		if (line[0] == ' ' || line[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1].Raw += "\r\n" + string(line)
			continue
		}
		name, _, _ := strings.Cut(string(line), ":")
		lines = append(lines, HeaderLine{Name: strings.ToLower(strings.TrimSpace(name)), Raw: string(line)})
	}
	return lines, rest
}

func Header(lines []HeaderLine, name string) string {
	for _, l := range lines {
		if l.Name == name {
			return l.Value()
		}
	}
	return ""
}

func AddressOf(from string) string {
	if a, err := netmail.ParseAddress(from); err == nil {
		return a.Address
	}
	return strings.Trim(strings.TrimSpace(from), "<>")
}

// Mailgun prepends its own Authentication-Results, so only the topmost one
// counts; any further down are the sender's to write.
func Authenticated(lines []HeaderLine) string {
	results, ok := mailgunResults(lines)
	if !ok {
		return "no authentication results from Mailgun"
	}
	_, from, _ := strings.Cut(strings.ToLower(AddressOf(Header(lines, "from"))), "@")
	passed, arc := passes(results, from)
	if passed {
		return ""
	}
	if arc {
		if forwarded, _ := passes(sealedResults(lines), from); forwarded {
			return ""
		}
	}
	return "the sender's address passed neither SPF nor DKIM"
}

func mailgunResults(lines []HeaderLine) (string, bool) {
	id, results, _ := strings.Cut(uncommented(Header(lines, "authentication-results")), ";")
	return results, strings.HasSuffix(strings.ToLower(strings.TrimSpace(id)), ".mailgun.org")
}

func SealedSender(lines []HeaderLine) string {
	results, ok := mailgunResults(lines)
	if !ok {
		return ""
	}
	if _, arc := passes(results, ""); !arc {
		return ""
	}
	for _, result := range strings.Split(sealedResults(lines), ";") {
		fields := strings.Fields(strings.ToLower(result))
		if len(fields) == 0 || fields[0] != "spf=pass" {
			continue
		}
		for _, field := range fields[1:] {
			if address, ok := strings.CutPrefix(field, "smtp.mailfrom="); ok {
				return strings.Trim(address, `"`)
			}
		}
	}
	return ""
}

func passes(results, from string) (passed, arc bool) {
	for _, result := range strings.Split(results, ";") {
		fields := strings.Fields(strings.ToLower(result))
		if len(fields) == 0 {
			continue
		}
		props := map[string]string{}
		for _, field := range fields[1:] {
			name, value, _ := strings.Cut(field, "=")
			value = strings.Trim(value, `"`)
			if _, domain, ok := strings.Cut(value, "@"); ok {
				value = domain
			}
			props[name] = value
		}
		switch {
		case fields[0] == "arc=pass":
			arc = true
		case fields[0] == "dmarc=pass" && aligned(props["header.from"], from):
			passed = true
		case fields[0] == "dkim=pass" && aligned(props["header.d"], from):
			passed = true
		case fields[0] == "spf=pass" && aligned(props["smtp.mailfrom"], from):
			passed = true
		}
	}
	return passed, arc
}

// Mailgun's arc=pass vouches that every ARC set is its sealer's own, so the
// newest set's results stand for the sender only when google.com sealed it.
func sealedResults(lines []HeaderLine) string {
	newest, sealer := 0, ""
	for _, l := range lines {
		if l.Name != "arc-seal" {
			continue
		}
		tags := arcTags(l.Value())
		if i, _ := strconv.Atoi(tags["i"]); i > newest {
			newest, sealer = i, tags["d"]
		}
	}
	if sealer != "google.com" {
		return ""
	}
	for _, l := range lines {
		if l.Name != "arc-authentication-results" {
			continue
		}
		instance, rest, _ := strings.Cut(uncommented(l.Value()), ";")
		if arcTags(instance)["i"] != strconv.Itoa(newest) {
			continue
		}
		_, results, _ := strings.Cut(rest, ";")
		return results
	}
	return ""
}

func arcTags(value string) map[string]string {
	tags := map[string]string{}
	for _, tag := range strings.Split(value, ";") {
		name, v, _ := strings.Cut(tag, "=")
		tags[strings.ToLower(strings.TrimSpace(name))] = strings.ToLower(strings.TrimSpace(v))
	}
	return tags
}

func aligned(domain, from string) bool {
	if domain == "" || from == "" {
		return false
	}
	return domain == from || strings.HasSuffix(from, "."+domain) || strings.HasSuffix(domain, "."+from)
}

func uncommented(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}
