package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/calendar"
	"heliosian/internal/keypoints"
	"heliosian/internal/who"
)

// allFamilies are the lists that go to every family, whoever reads them.
var allFamilies = []string{"parentsandstaff", "parentsonly", "parentsandstudents", "community", "parents", "newstudentfamilies", "new.parents"}

// schoolDays is how far back the Inbox widget reads: as far as the key
// points pass reads (keypoints.Window), so every email it lists has its
// points and, for Veracross mail, its audience judged.
const schoolDays = int(keypoints.Window / (24 * time.Hour))

// schoolEmail is one school email as the widget shows it.
type schoolEmail struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Date    string `json:"date"`
	Kind    string `json:"kind"`
	Channel string `json:"channel"`
	// Audience is whom it was judged to be written to: Everyone, or the
	// classrooms it names (artifacts.AudienceColumn); blank for a list's
	// mail, which its channel says.
	Audience string   `json:"audience"`
	Points   []string `json:"points"`
}

// schoolWidget answers GET /api/apps/school on Heliosian's host: the home
// page's Inbox widget for the viewer - the last two weeks' school
// email with its key points (internal/keypoints), newest first: the
// newsletter, the lists to every family, the lists of the viewer's own
// classrooms - their children's, their own as a student, those they teach -
// and the grade bands that hold them, and Veracross mail written to a
// classroom and grade one of them is in. The everyone chat and Loop's groups
// stay out.
func schoolWidget(directory *who.Cache, artifactsCache *artifacts.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		people := directory.Model()
		email := people.Resolve(auth.Email(r))
		seats, grades := seatsOf(people, email), gradeNames(people)
		since := time.Now().In(calendar.Location).AddDate(0, 0, -schoolDays).Format("2006-01-02")
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
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Emails []schoolEmail `json:"emails"`
		}{out}); err != nil {
			slog.ErrorContext(r.Context(), "encode school widget", "error", err)
		}
	}
}

// schoolDirectory is what the key points pass needs of the directory: the
// school's classrooms, and those an email's sender teaches - found by the
// name in its From, since Veracross sends a teacher's mail from its own
// address.
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

// A seat is one place the viewer sits in the school: a classroom as the
// lists name it ("jays"), and the grade in it - a child's, or their own as
// a student - or none for a classroom whose crew they teach, which holds
// every grade in it.
type seat struct {
	room, grade string
}

// seatsOf are the viewer's seats: their own as a student, each child's,
// and each classroom whose crew they teach.
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

// gradeNames are the school's grades as the directory names them ("Grade
// 2", "Kindergarten").
func gradeNames(m *who.Model) []string {
	out := []string{}
	for _, g := range m.Grades {
		out = append(out, g.Name)
	}
	return out
}

// forClassrooms says a school email reaches the viewer. A list says whom it
// went to: every family's always, a classroom's ("jays.parents") or a grade
// band's ("jaysandravens") when it names one of their classrooms. Mail the
// school sends through Veracross - the newsletter, but a teacher's note to
// their class too - says nothing of whom it went to, so it waits until its
// audience is judged (internal/keypoints) and then reaches everyone, or a
// seat that fits what it names: in one of its classrooms, when it names
// any, and in one of its grades, when it names any - so a note to "parents
// of 2nd, 4th, 6th, and 8th graders" misses a family of a 5th and a 7th
// grader, whatever their classrooms. A teacher's seat fits every grade in
// the classroom they teach.
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
