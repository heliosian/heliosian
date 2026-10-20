package db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	netmail "net/mail"
	"regexp"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
)

var errLoopMail = errors.New("loop files the mail it sends itself")

var schoolDomains = []string{"heliosschool.org", "heliosns.org"}

var listGroups = map[string][]string{
	"parentsandstaff":     {"parents", "staff"},
	"parents":             {"parents"},
	"parentsonly":         {"parents"},
	"new.parents":         {"parents"},
	"new.parents.2020-21": {"parents"},
	"newstudentfamilies":  {"parents"},
	"parentsandstudents":  {"parents", "students"},
	"community":           {"everyone"},
	"chat":                {"everyone"},
	"chat2":               {"everyone"},
}

var gradebandLists = map[string]string{"hawksandfalcons": "halcons-parents", "jaysandravens": "jayvens-parents", "condorsandospreys": "cospreys-parents"}

var schoolYear = regexp.MustCompile(`^\d{4}-\d{2}\.`)

const sameMailWithin = 10 * time.Minute

func (m *Model) mailRoot(raw []byte) (map[string]any, []string, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("not a mail message: %w", err)
	}
	list := listName(msg.Header.Get("List-Id"))
	if strings.HasSuffix(list, ".loop.heliosian.com") {
		return nil, nil, errLoopMail
	}
	sent, err := msg.Header.Date()
	if err != nil {
		return nil, nil, fmt.Errorf("its date: %w", err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		return nil, nil, fmt.Errorf("its subject: %w", err)
	}
	sentTo, err := m.mailGroups(list)
	if err != nil {
		return nil, nil, err
	}
	row := map[string]any{
		"kind":      "mail",
		"name":      strings.TrimSpace(subject),
		"published": sent.In(School).Format(cells.StampFormat),
	}
	if author := m.PersonOf(mail.AddressOf(msg.Header.Get("From"))); author != "" {
		row["author"] = author
	}
	return row, sentTo, nil
}

type mailSender struct {
	from, id, to, list string
}

func senderOf(raw []byte) (mailSender, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return mailSender{}, fmt.Errorf("not a mail message: %w", err)
	}
	h := func(name string) string { return strings.ToLower(strings.TrimSpace(msg.Header.Get(name))) }
	return mailSender{from: h("From"), id: h("Message-Id"), to: h("To"), list: h("List-Id")}, nil
}

// A copy of one email is the same message reaching another mailbox (its Message-ID), or, as the school's mailer
// sends each family its own off any list, the same subject from the same sender to someone else within minutes.
func (s mailSender) copies(other mailSender) bool {
	if s.from != other.from {
		return false
	}
	if s.id == other.id {
		return true
	}
	return s.list == "" && other.list == "" && s.to != other.to
}

func (m *Model) copyOf(ctx context.Context, bucket *blob.Bucket, name, published string, sender mailSender) (string, error) {
	at, err := time.ParseInLocation(cells.StampFormat, published, School)
	if err != nil {
		return "", err
	}
	contents := m.Table("CONTENT")
	for _, row := range m.Table("DOCUMENT").All() {
		if row["parent"] != "" || row["kind"] != "mail" || row["message"] != "" || row["name"] != name {
			continue
		}
		other, err := time.ParseInLocation(cells.StampFormat, row["published"], School)
		if err != nil || other.Sub(at).Abs() > sameMailWithin {
			continue
		}
		content, ok := contents.Get(row["content"])
		if !ok {
			continue
		}
		raw, _, err := bucket.Get(ctx, content["blob"])
		if err != nil {
			return "", err
		}
		if theirs, err := senderOf(raw); err == nil && sender.copies(theirs) {
			return row["id"], nil
		}
	}
	return "", nil
}

func listName(listID string) string {
	if i := strings.LastIndex(listID, "<"); i >= 0 {
		listID = listID[i+1:]
	}
	return strings.ToLower(strings.Trim(strings.TrimSpace(listID), "<> "))
}

func schoolList(list string) string {
	for _, domain := range schoolDomains {
		if name, ok := strings.CutSuffix(list, "."+domain); ok {
			return name
		}
	}
	return ""
}

func (m *Model) mailGroups(list string) ([]string, error) {
	if list == "" {
		return []string{}, nil
	}
	name := schoolList(list)
	if slugs, ok := listGroups[name]; ok {
		return m.groupsWhere(func(g map[string]string) bool { return g["kind"] == "group" && slices.Contains(slugs, g["slug"]) }, len(slugs), name)
	}
	if slug, ok := gradebandLists[name]; ok {
		return m.groupsWhere(func(g map[string]string) bool { return g["kind"] == "group" && g["slug"] == slug }, 1, name)
	}
	room := schoolYear.ReplaceAllString(name, "")
	if r, ok := strings.CutSuffix(room, ".parents"); ok && m.classroomNamed(r) {
		return m.groupsWhere(func(g map[string]string) bool {
			return g["kind"] == "group" && g["slug"] == strings.ToLower(r)+"-parents"
		}, 1, name)
	}
	if r, ok := strings.CutSuffix(room, ".students"); ok && m.classroomNamed(r) {
		return m.groupsWhere(func(g map[string]string) bool { return g["kind"] == "classroom" && strings.EqualFold(g["name"], r) }, 1, name)
	}
	return nil, access.Invalid("the list %q is not the community's", list)
}

func (m *Model) classroomNamed(name string) bool {
	for _, g := range m.Table("GROUP").All() {
		if g["kind"] == "classroom" && strings.EqualFold(g["name"], name) {
			return true
		}
	}
	return false
}

func (m *Model) groupsWhere(match func(map[string]string) bool, want int, list string) ([]string, error) {
	out := []string{}
	for _, g := range m.Table("GROUP").All() {
		if match(g) {
			out = append(out, g["id"])
		}
	}
	if len(out) != want {
		return nil, fmt.Errorf("the list %q should name %d groups and names %d", list, want, len(out))
	}
	return out, nil
}
