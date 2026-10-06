package db

import (
	"bytes"
	"errors"
	"fmt"
	"mime"
	netmail "net/mail"
	"regexp"
	"slices"
	"strings"

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
}

var bandLists = map[string]string{"hawksandfalcons": "halcons-parents", "jaysandravens": "jayvens-parents", "condorsandospreys": "cospreys-parents"}

var schoolYear = regexp.MustCompile(`^\d{4}-\d{2}\.`)

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
	if slug, ok := bandLists[name]; ok {
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
	return m.groupsWhere(func(g map[string]string) bool { return g["kind"] == "group" && g["slug"] == "everyone" }, 1, list)
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
