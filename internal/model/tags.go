package model

import (
	"fmt"
	"slices"
	"strings"

	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const maxTagLength = 40

type Tag struct {
	ID        string   `json:"id"`
	Owner     string   `json:"owner"`
	OwnerName string   `json:"ownerName"`
	Name      string   `json:"name"`
	People    []string `json:"people"`
	Managers  []string `json:"managers"`
}

type tagRecord struct {
	id, owner, name  string
	people, managers []string
}

func (l *loader) readTags() error {
	tags := map[string]*tagRecord{}
	named := map[string]bool{}
	for _, row := range l.tagListRows {
		key, ok := id.Parse(row[tagID])
		if !ok {
			return fmt.Errorf("tag list row %v has no valid tag id", row)
		}
		if tags[key] != nil {
			return fmt.Errorf("tag list has two rows for tag %s", key)
		}
		owner, name := mail.Normalize(row[tagOwner]), row[tagName]
		if owner == "" || !validTagName(name) || name != strings.TrimSpace(name) {
			return fmt.Errorf("tag list row %s needs an owner and a tag name of at most %d characters with no spaces around it", key, maxTagLength)
		}
		if named[owner+"\n"+strings.ToLower(name)] {
			return fmt.Errorf("%s has two tags called %s", owner, name)
		}
		named[owner+"\n"+strings.ToLower(name)] = true
		tags[key] = &tagRecord{id: key, owner: owner, name: name}
	}
	people, err := tagEmails(tags, tagsTable, tagPerson, l.tagRows)
	if err != nil {
		return err
	}
	managers, err := tagEmails(tags, managersTable, managerEmail, l.managerRows)
	if err != nil {
		return err
	}
	for key, t := range tags {
		t.people, t.managers = people[key], managers[key]
		if len(t.people) == 0 {
			return fmt.Errorf("tag %s, %s's %s, has nobody on it: delete its tag list row", key, t.owner, t.name)
		}
	}
	l.model.tags = tags
	return nil
}

func tagEmails(tags map[string]*tagRecord, tab, column string, rows []store.Row) (map[string][]string, error) {
	out := map[string][]string{}
	for _, row := range rows {
		key, _ := id.Parse(row[tagID])
		if tags[key] == nil {
			return nil, fmt.Errorf("%s row %v names no tag in the tag list", tab, row)
		}
		email := mail.Normalize(row[column])
		if email == "" {
			return nil, fmt.Errorf("%s row %v has no %s", tab, row, column)
		}
		if slices.Contains(out[key], email) {
			return nil, fmt.Errorf("%s has two rows for %s on tag %s", tab, email, key)
		}
		out[key] = append(out[key], email)
	}
	return out, nil
}

func (m *Directory) TagIDs() []string {
	out := []string{}
	for key := range m.tags {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func (m *Directory) tagByKey(raw string) *tagRecord {
	key, ok := id.Parse(raw)
	if !ok {
		return nil
	}
	return m.tags[key]
}

func (m *Directory) listed(emails []string) []string {
	out := []string{}
	for _, email := range emails {
		if m.Person(email) != nil {
			out = append(out, email)
		}
	}
	slices.Sort(out)
	return out
}

func (m *Directory) Tag(key string) (Tag, bool) {
	t := m.tagByKey(key)
	if t == nil {
		return Tag{}, false
	}
	people := m.listed(t.people)
	if len(people) == 0 {
		return Tag{}, false
	}
	return Tag{ID: t.id, Owner: t.owner, OwnerName: m.DisplayName(t.owner), Name: t.name, People: people, Managers: m.listed(t.managers)}, true
}

func (m *Directory) sortedTags(keep func(t *tagRecord) bool) []Tag {
	out := []Tag{}
	for _, t := range m.tags {
		if !keep(t) {
			continue
		}
		if tag, ok := m.Tag(t.id); ok {
			out = append(out, tag)
		}
	}
	slices.SortFunc(out, func(a, b Tag) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		if c := strings.Compare(a.Owner, b.Owner); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

func (m *Directory) Tags(owner string) []Tag {
	owner = strings.ToLower(owner)
	return m.sortedTags(func(t *tagRecord) bool { return t.owner == owner })
}

func (m *Directory) SharedTags(email string) []Tag {
	email = strings.ToLower(email)
	return m.sortedTags(func(t *tagRecord) bool { return slices.Contains(t.managers, email) && m.Person(t.owner) != nil })
}
