package model

import (
	"fmt"
	"strings"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

const (
	toDosTab        = "To Dos"
	toDoReadsTab    = "To Do Reads"
	homeToDosTab    = "To Dos"
	kindToDo        = "to-do"
	ToDoWindow      = SchoolMailWindow
	ToDoDone        = "done"
	ToDoSaved       = "saved"
	maxToDoTitle    = 120
	maxToDoSummary  = 120
	maxToDoDetails  = 500
	toDoChangedTime = "2006-01-02 15:04"
)

var (
	ToDoColumns     = []string{"Document", "Title", "Summary", "Details", "Link", "Due"}
	ToDoReadColumns = []string{"Document", "Read"}
	HomeToDoColumns = []string{"Email", "To Do", "State", "Changed"}
)

type ToDo struct {
	ID       string
	Document string
	Title    string
	Summary  string
	Details  string
	Link     string
	Due      string
}

func toDoID(key []byte, document, title string) string {
	return id.Of(key, kindToDo, document+"\x00"+strings.ToLower(title))
}

func CheckToDo(t ToDo) error {
	if t.Title == "" || len(t.Title) > maxToDoTitle {
		return fmt.Errorf("to-do %q: the title is blank or longer than %d", t.Title, maxToDoTitle)
	}
	if len(t.Summary) > maxToDoSummary || len(t.Details) > maxToDoDetails {
		return fmt.Errorf("to-do %q: the summary or details are too long", t.Title)
	}
	if t.Due != "" {
		if _, err := time.ParseInLocation(DateFormat, t.Due, Location); err != nil {
			return fmt.Errorf("to-do %q: due %q is not a date like 2026-10-05", t.Title, t.Due)
		}
	}
	if err := cells.URL(t.Link, true); err != nil {
		return fmt.Errorf("to-do %q: %w", t.Title, err)
	}
	return nil
}

func buildToDos(tables store.Tables, m *Documents) error {
	m.ToDos, m.ToDosRead = []*ToDo{}, map[string]string{}
	seen := map[string]bool{}
	for _, row := range tables[toDosTab] {
		t := &ToDo{
			Document: strings.TrimSpace(row["Document"]), Title: strings.TrimSpace(row["Title"]), Summary: strings.TrimSpace(row["Summary"]),
			Details: strings.TrimSpace(row["Details"]), Link: strings.TrimSpace(row["Link"]), Due: strings.TrimSpace(row["Due"]),
		}
		if t.Document == "" {
			return fmt.Errorf("%s: to-do %q names no document", toDosTab, t.Title)
		}
		if err := CheckToDo(*t); err != nil {
			return fmt.Errorf("%s: %w", toDosTab, err)
		}
		named := t.Document + "\x00" + strings.ToLower(t.Title)
		if seen[named] {
			return fmt.Errorf("%s has two rows for %q from document %s", toDosTab, t.Title, t.Document)
		}
		seen[named] = true
		m.ToDos = append(m.ToDos, t)
	}
	for _, row := range tables[toDoReadsTab] {
		m.ToDosRead[strings.TrimSpace(row["Document"])] = strings.TrimSpace(row["Read"])
	}
	return nil
}

func (d *Document) ToDoSource() bool {
	return d.School() || d.Kind == DocumentKindGroup
}

func (m *Documents) Current(t *ToDo, today string) bool {
	if t.Due != "" {
		return t.Due >= today
	}
	d := m.byKey[t.Document]
	if d == nil {
		return false
	}
	day, err := time.ParseInLocation(DateFormat, today, Location)
	if err != nil {
		panic(err)
	}
	return d.Date >= day.Add(-ToDoWindow).Format(DateFormat)
}

func buildHomeToDos(rows []store.Row) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for _, row := range rows {
		email := strings.ToLower(strings.TrimSpace(row["Email"]))
		toDo, ok := id.Parse(row["To Do"])
		if email == "" || !ok {
			return nil, fmt.Errorf("%s row %v names no address or no to-do", homeToDosTab, row)
		}
		state := strings.TrimSpace(row["State"])
		if state != ToDoDone && state != ToDoSaved {
			return nil, fmt.Errorf("%s row for %s: state %q is not %s or %s", homeToDosTab, email, state, ToDoDone, ToDoSaved)
		}
		if out[email] == nil {
			out[email] = map[string]string{}
		}
		out[email][toDo] = state
	}
	return out, nil
}
