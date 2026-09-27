package app

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/keypoints"
	"heliosian/internal/serve"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

var allFamilies = []string{"parentsandstaff", "parentsonly", "parentsandstudents", "community", "parents", "newstudentfamilies", "new.parents"}

const schoolDays = int(keypoints.Window / (24 * time.Hour))

type schoolEmail struct {
	Key      string   `json:"key"`
	Title    string   `json:"title"`
	Date     string   `json:"date"`
	Kind     string   `json:"kind"`
	Channel  string   `json:"channel"`
	Audience string   `json:"audience"`
	Points   []string `json:"points"`
}

type schoolView struct {
	Emails []schoolEmail `json:"emails"`
}

func schoolWidget(directory *who.Cache, artifactsCache *artifacts.Cache) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (schoolView, error) {
		people := directory.Model()
		email := people.Resolve(auth.Email(r))
		seats, grades := seatsOf(people, email), gradeNames(people)
		since := time.Now().In(when.Location).AddDate(0, 0, -schoolDays).Format("2006-01-02")
		m := artifactsCache.Model()
		out := []schoolEmail{}
		for _, d := range m.Documents {
			if d.Date < since {
				break
			}
			if !artifacts.School(d) || !forClassrooms(d, seats, m.Audience[d.Key], grades) {
				continue
			}
			points := m.Points[d.Key]
			if points == nil {
				points = []string{}
			}
			out = append(out, schoolEmail{Key: d.Key, Title: d.Title, Date: d.Date, Kind: d.Kind, Channel: d.Channel, Audience: m.Audience[d.Key], Points: points})
		}
		return schoolView{out}, nil
	})
}

type schoolDirectory struct {
	cache *who.Cache
}

func (s schoolDirectory) Classrooms() []string {
	out := []string{}
	for _, c := range s.cache.Model().Classrooms {
		out = append(out, c.Name)
	}
	return out
}

func (s schoolDirectory) Grades() []string {
	return gradeNames(s.cache.Model())
}

func (s schoolDirectory) Teaches(author string) []string {
	name, _, _ := strings.Cut(author, "<")
	name = strings.Trim(strings.TrimSpace(name), `"`)
	if name == "" {
		return nil
	}
	m := s.cache.Model()
	out := []string{}
	for _, c := range m.Crews {
		for _, email := range c.Teachers {
			if p := m.Person(email); p != nil && strings.EqualFold(p.FullName, name) && !slices.Contains(out, c.Classroom) {
				out = append(out, c.Classroom)
			}
		}
	}
	return out
}

type seat struct {
	room, grade string
}

func seatsOf(m *who.Model, email string) []seat {
	out := []seat{}
	add := func(room, grade string) {
		if s := (seat{who.ClassroomSlug(room), grade}); s.room != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	if p := m.Person(email); p != nil && p.IsStudent {
		add(p.Classroom, p.Grade)
	}
	for _, kid := range m.Children(email) {
		add(kid.Classroom, kid.Grade)
	}
	for _, c := range m.Crews {
		if slices.Contains(c.Teachers, email) {
			add(c.Classroom, "")
		}
	}
	return out
}

func gradeNames(m *who.Model) []string {
	out := []string{}
	for _, g := range m.Grades {
		out = append(out, g.Name)
	}
	return out
}

func forClassrooms(d *artifacts.Document, seats []seat, audience string, grades []string) bool {
	if d.Kind != artifacts.KindList {
		if audience == "" {
			return false
		}
		named := artifacts.Classrooms(audience)
		if len(named) == 0 {
			return true
		}
		rooms, toGrades := []string{}, []string{}
		for _, n := range named {
			if slices.Contains(grades, n) {
				toGrades = append(toGrades, n)
			} else {
				rooms = append(rooms, who.ClassroomSlug(n))
			}
		}
		for _, s := range seats {
			inRoom := len(rooms) == 0 || slices.Contains(rooms, s.room)
			inGrade := len(toGrades) == 0 || s.grade == "" || slices.Contains(toGrades, s.grade)
			if inRoom && inGrade {
				return true
			}
		}
		return false
	}
	if slices.Contains(allFamilies, d.Channel) {
		return true
	}
	room, _, _ := strings.Cut(d.Channel, ".")
	mine := []string{}
	for _, s := range seats {
		mine = append(mine, s.room)
	}
	for _, slug := range mine {
		if room == slug || (strings.Contains(room, "and") && slices.Contains(strings.Split(room, "and"), slug)) {
			return true
		}
	}
	return false
}
