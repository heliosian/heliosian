package model

import (
	"slices"
	"strings"
	"time"
)

const SchoolMailWindow = 14 * 24 * time.Hour

var allFamilies = []string{"parentsandstaff", "parentsonly", "parentsandstudents", "community", "parents", "newstudentfamilies", "new.parents"}

type SchoolEmail struct {
	Key      string   `json:"key"`
	Title    string   `json:"title"`
	Date     string   `json:"date"`
	Time     string   `json:"time"`
	Kind     string   `json:"kind"`
	Channel  string   `json:"channel"`
	Audience string   `json:"audience"`
	Points   []string `json:"points"`
}

func (m *Documents) SchoolMail(people *Directory, email, since string) []SchoolEmail {
	seats, grades := classSeatsOf(people, email), people.GradeNames()
	out := []SchoolEmail{}
	for _, d := range m.Documents {
		if d.Date < since {
			break
		}
		if !d.School() || !forClassrooms(d, seats, m.Audience[d.Key], grades) {
			continue
		}
		points := m.Points[d.Key]
		if points == nil {
			points = []string{}
		}
		out = append(out, SchoolEmail{Key: d.Key, Title: d.Title, Date: d.Date, Time: d.Time, Kind: d.Kind, Channel: d.Channel, Audience: m.Audience[d.Key], Points: points})
	}
	return out
}

type classSeat struct {
	room, grade string
}

func classSeatsOf(m *Directory, email string) []classSeat {
	out := []classSeat{}
	add := func(room, grade string) {
		if s := (classSeat{ClassroomSlug(room), grade}); s.room != "" && !slices.Contains(out, s) {
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

func forClassrooms(d *Document, seats []classSeat, audience string, grades []string) bool {
	if d.Kind != DocumentKindList {
		if audience == "" {
			return false
		}
		named := AudienceClassrooms(audience)
		if len(named) == 0 {
			return true
		}
		rooms, toGrades := []string{}, []string{}
		for _, n := range named {
			if slices.Contains(grades, n) {
				toGrades = append(toGrades, n)
			} else {
				rooms = append(rooms, ClassroomSlug(n))
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
