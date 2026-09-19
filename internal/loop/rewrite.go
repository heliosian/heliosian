package loop

import (
	"bytes"
	"fmt"
	"mime"
	"net/mail"
	"strings"
)

type headerLine struct {
	name string
	raw  string
}

func (h headerLine) value() string {
	_, rest, _ := strings.Cut(h.raw, ":")
	rest = strings.ReplaceAll(rest, "\r\n\t", " ")
	rest = strings.ReplaceAll(rest, "\r\n ", " ")
	return strings.TrimSpace(rest)
}

func splitMessage(raw []byte) ([]headerLine, []byte) {
	lines := []headerLine{}
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
			lines[len(lines)-1].raw += "\r\n" + string(line)
			continue
		}
		name, _, _ := strings.Cut(string(line), ":")
		lines = append(lines, headerLine{name: strings.ToLower(strings.TrimSpace(name)), raw: string(line)})
	}
	return lines, rest
}

var droppedHeaders = map[string]bool{
	"return-path": true, "bcc": true, "sender": true, "reply-to": true, "dkim-signature": true,
	"list-id": true, "list-post": true, "list-unsubscribe": true, "list-unsubscribe-post": true,
	"list-help": true, "list-subscribe": true, "list-archive": true, "list-owner": true,
	"precedence": true, "x-helios-loop": true,
}

const loopHeader = "X-Helios-Loop"

func held(lines []headerLine) string {
	for _, l := range lines {
		switch l.name {
		case strings.ToLower(loopHeader):
			return "already sent through Helios Loop"
		case "auto-submitted":
			if !strings.EqualFold(l.value(), "no") {
				return "auto-submitted mail"
			}
		case "from":
			local, _, _ := strings.Cut(strings.ToLower(addressOf(l.value())), "@")
			if local == "mailer-daemon" || local == "postmaster" {
				return "a mail system's notice"
			}
		}
	}
	return ""
}

// Mailgun prepends its own Authentication-Results, so only the topmost one
// counts; any further down are the sender's to write.
func authenticated(lines []headerLine) string {
	id, results, _ := strings.Cut(uncommented(header(lines, "authentication-results")), ";")
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(id)), ".mailgun.org") {
		return "no authentication results from Mailgun"
	}
	_, from, _ := strings.Cut(strings.ToLower(addressOf(header(lines, "from"))), "@")
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
		case fields[0] == "dmarc=pass" && aligned(props["header.from"], from):
			return ""
		case fields[0] == "dkim=pass" && aligned(props["header.d"], from):
			return ""
		case fields[0] == "spf=pass" && aligned(props["smtp.mailfrom"], from):
			return ""
		}
	}
	return "the sender's address passed neither SPF nor DKIM"
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

func addressOf(from string) string {
	if a, err := mail.ParseAddress(from); err == nil {
		return a.Address
	}
	return strings.Trim(strings.TrimSpace(from), "<>")
}

func header(lines []headerLine, name string) string {
	for _, l := range lines {
		if l.name == name {
			return l.value()
		}
	}
	return ""
}

func messageID(lines []headerLine) string {
	for _, l := range lines {
		if l.name == "message-id" {
			return messageKey(l.value())
		}
	}
	return ""
}

func senderName(from string) string {
	a, err := mail.ParseAddress(from)
	if err != nil {
		local, _, _ := strings.Cut(addressOf(from), "@")
		return local
	}
	if a.Name != "" {
		return a.Name
	}
	local, _, _ := strings.Cut(a.Address, "@")
	return local
}

func decodeHeader(s string) string {
	out, err := new(mime.WordDecoder).DecodeHeader(s)
	if err != nil {
		return s
	}
	return out
}

func prefixed(subject, title string) string {
	rest := strings.TrimSpace(subject)
	tag := "[" + title + "]"
	reply := false
	for {
		lower := strings.ToLower(rest)
		switch {
		case strings.HasPrefix(lower, "re:"):
			rest = strings.TrimSpace(rest[3:])
			reply = true
		case strings.HasPrefix(lower, strings.ToLower(tag)):
			rest = strings.TrimSpace(rest[len(tag):])
		default:
			out := tag
			if rest != "" {
				out += " " + rest
			}
			if reply {
				out = "Re: " + out
			}
			return out
		}
	}
}

func rewrite(lines []headerLine, g Group) ([]byte, error) {
	var from, replyTo, subject string
	hasSubject := false
	for _, l := range lines {
		switch l.name {
		case "from":
			from = l.value()
		case "reply-to":
			replyTo = l.value()
		case "subject":
			subject = l.value()
			hasSubject = true
		}
	}
	if from == "" {
		return nil, fmt.Errorf("the message has no From")
	}
	if replyTo == "" {
		replyTo = from
	}
	newFrom := (&mail.Address{Name: senderName(from) + " via " + g.Title, Address: g.Address()}).String()
	newSubject := "Subject: " + mime.QEncoding.Encode("utf-8", prefixed(decodeHeader(subject), g.Title))
	var b bytes.Buffer
	for _, l := range lines {
		switch {
		case l.name == "from":
			b.WriteString("From: " + newFrom + "\r\n")
		case l.name == "subject" && g.Prefix:
			b.WriteString(newSubject + "\r\n")
		case droppedHeaders[l.name]:
		default:
			b.WriteString(l.raw + "\r\n")
		}
	}
	if !hasSubject && g.Prefix {
		b.WriteString(newSubject + "\r\n")
	}
	b.WriteString("Reply-To: " + replyTo + "\r\n")
	b.WriteString("X-Original-From: " + from + "\r\n")
	b.WriteString("List-Id: " + mime.QEncoding.Encode("utf-8", g.Title) + " <" + g.Name + "." + Domain + ">\r\n")
	b.WriteString("List-Post: <mailto:" + g.Address() + ">\r\n")
	b.WriteString("Precedence: list\r\n")
	b.WriteString(loopHeader + ": " + g.Name + "\r\n")
	return b.Bytes(), nil
}

func render(head []byte, extra []string, body []byte) []byte {
	var b bytes.Buffer
	b.Write(head)
	for _, line := range extra {
		b.WriteString(line + "\r\n")
	}
	b.WriteString("\r\n")
	b.Write(body)
	return b.Bytes()
}
