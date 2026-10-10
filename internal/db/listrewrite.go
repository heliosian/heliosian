package db

import (
	"bytes"
	"fmt"
	"mime"
	netmail "net/mail"
	"regexp"
	"strings"

	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const loopHeader = "X-Helios-Loop"

var droppedHeaders = map[string]bool{
	"return-path": true, "bcc": true, "sender": true, "reply-to": true, "dkim-signature": true,
	"list-id": true, "list-post": true, "list-unsubscribe": true, "list-unsubscribe-post": true,
	"list-help": true, "list-subscribe": true, "list-archive": true, "list-owner": true,
	"precedence": true, "x-helios-loop": true,
}

func held(lines []mail.HeaderLine) string {
	for _, l := range lines {
		switch l.Name {
		case strings.ToLower(loopHeader):
			return "already sent through Helios Loop"
		case "auto-submitted":
			if !strings.EqualFold(l.Value(), "no") {
				return "auto-submitted mail"
			}
		case "from":
			local, _, _ := strings.Cut(strings.ToLower(mail.AddressOf(l.Value())), "@")
			if local == "mailer-daemon" || local == "postmaster" {
				return "a mail system's notice"
			}
		}
	}
	return ""
}

var bracketed = regexp.MustCompile(`<([^<>]+)>`)

func messageKey(id string) string {
	return strings.Trim(strings.TrimSpace(id), "<>")
}

func referenced(lines []mail.HeaderLine) []string {
	out := []string{}
	for _, l := range lines {
		if l.Name != "in-reply-to" && l.Name != "references" {
			continue
		}
		for _, m := range bracketed.FindAllStringSubmatch(l.Value(), -1) {
			out = append(out, messageKey(m[1]))
		}
	}
	return out
}

func messageID(lines []mail.HeaderLine) string {
	return messageKey(mail.Header(lines, "message-id"))
}

func senderName(from string) string {
	a, err := netmail.ParseAddress(from)
	if err != nil {
		local, _, _ := strings.Cut(mail.AddressOf(from), "@")
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

func rewrite(lines []mail.HeaderLine, g store.Row) ([]byte, error) {
	var from, replyTo, subject string
	hasSubject := false
	for _, l := range lines {
		switch l.Name {
		case "from":
			from = l.Value()
		case "reply-to":
			replyTo = l.Value()
		case "subject":
			subject = l.Value()
			hasSubject = true
		}
	}
	if from == "" {
		return nil, fmt.Errorf("the message has no From")
	}
	if replyTo == "" {
		replyTo = from
	}
	title, address := g["name"], listAddress(g)
	newSubject := "Subject: " + mime.QEncoding.Encode("utf-8", prefixed(decodeHeader(subject), title))
	var b bytes.Buffer
	for _, l := range lines {
		switch {
		case l.Name == "from":
			b.WriteString("From: " + (&netmail.Address{Name: senderName(from) + " via " + title, Address: address}).String() + "\r\n")
		case l.Name == "subject":
			b.WriteString(newSubject + "\r\n")
		case droppedHeaders[l.Name]:
		default:
			b.WriteString(l.Raw + "\r\n")
		}
	}
	if !hasSubject {
		b.WriteString(newSubject + "\r\n")
	}
	b.WriteString("Reply-To: " + replyTo + "\r\n")
	b.WriteString("X-Original-From: " + from + "\r\n")
	b.WriteString("List-Id: " + mime.QEncoding.Encode("utf-8", title) + " <" + g["slug"] + "." + LoopDomain + ">\r\n")
	b.WriteString("List-Post: <mailto:" + address + ">\r\n")
	b.WriteString("Precedence: list\r\n")
	b.WriteString(loopHeader + ": " + g["slug"] + "\r\n")
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
