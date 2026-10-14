package mail

import (
	"bytes"
	"fmt"
	"io"
	netmail "net/mail"
	"strings"
)

type Reply struct {
	UID, Email, Standing string
}

func ParseReply(raw []byte) (Reply, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Reply{}, fmt.Errorf("read message: %w", err)
	}
	var ics []byte
	err = Parts(msg, func(p Part) error {
		if ics != nil || (p.MediaType != "text/calendar" && p.MediaType != "application/ics" && !strings.HasSuffix(strings.ToLower(p.Name), ".ics")) {
			return nil
		}
		out, err := io.ReadAll(io.LimitReader(p.Body, 1<<20))
		if err != nil {
			return err
		}
		if len(out) > 0 {
			ics = out
		}
		return nil
	})
	if err != nil {
		return Reply{}, err
	}
	if ics == nil {
		return Reply{}, fmt.Errorf("no calendar part")
	}
	return readReply(ics)
}

func readReply(ics []byte) (Reply, error) {
	text := strings.ReplaceAll(string(ics), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n ", "")
	text = strings.ReplaceAll(text, "\n\t", "")
	var reply Reply
	method := ""
	for _, line := range strings.Split(text, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		field, params, _ := strings.Cut(name, ";")
		switch strings.ToUpper(field) {
		case "METHOD":
			method = strings.ToUpper(strings.TrimSpace(value))
		case "UID":
			if reply.UID == "" {
				reply.UID = strings.TrimSpace(value)
			}
		case "ATTENDEE":
			if reply.Email != "" {
				continue
			}
			reply.Email = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "mailto:")
			for _, p := range strings.Split(params, ";") {
				if k, v, ok := strings.Cut(p, "="); ok && strings.EqualFold(k, "PARTSTAT") {
					reply.Standing = strings.ToUpper(strings.Trim(v, `"`))
				}
			}
		}
	}
	if method != "REPLY" {
		return Reply{}, fmt.Errorf("calendar method is %q, not REPLY", method)
	}
	if reply.UID == "" || reply.Email == "" {
		return Reply{}, fmt.Errorf("reply names no event or attendee")
	}
	return reply, nil
}
