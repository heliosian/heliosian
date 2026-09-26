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
	"heliosian/internal/who"
)

// allFamilies are the lists that go to every family, whoever reads them.
var allFamilies = []string{"parentsandstaff", "parentsonly", "parentsandstudents", "community", "parents", "newstudentfamilies", "new.parents"}

// schoolDays is how far back the From the School widget reads.
const schoolDays = 7

// schoolEmail is one school email as the widget shows it.
type schoolEmail struct {
	Key     string   `json:"key"`
	Title   string   `json:"title"`
	Date    string   `json:"date"`
	Kind    string   `json:"kind"`
	Channel string   `json:"channel"`
	Points  []string `json:"points"`
}

// schoolWidget answers GET /api/apps/school on Heliosian's host: the home
// page's From the School widget for the viewer - the last week's school
// email with its key points (internal/keypoints), newest first: the
// newsletter, the lists to every family, and the lists of the viewer's own
// classrooms - their children's, their own as a student, those they teach -
// and the grade bands that hold them. The everyone chat and Loop's groups
// stay out.
func schoolWidget(directory *who.Cache, artifactsCache *artifacts.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		people := directory.Model()
		email := people.Resolve(auth.Email(r))
		mine := classroomSlugs(people, email)
		since := time.Now().In(calendar.Location).AddDate(0, 0, -schoolDays).Format("2006-01-02")
		m := artifactsCache.Model()
		out := []schoolEmail{}
		for _, d := range m.Documents {
			if d.Date < since {
				break
			}
			if !artifacts.School(d) || !forClassrooms(d, mine, m.Audience[d.Key]) {
				continue
			}
			points := m.Points[d.Key]
			if points == nil {
				points = []string{}
			}
			out = append(out, schoolEmail{Key: d.Key, Title: d.Title, Date: d.Date, Kind: d.Kind, Channel: d.Channel, Points: points})
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

// classroomSlugs are the viewer's classrooms as the lists name them
// ("jays"): their children's, their own as a student, and those whose crews
// they teach.
func classroomSlugs(m *who.Model, email string) []string {
	out := []string{}
	add := func(name string) {
		if slug := who.ClassroomSlug(name); slug != "" && !slices.Contains(out, slug) {
			out = append(out, slug)
		}
	}
	if p := m.Person(email); p != nil && p.IsStudent {
		add(p.Classroom)
	}
	for _, kid := range m.Children(email) {
		add(kid.Classroom)
	}
	for _, c := range m.Crews {
		if slices.Contains(c.Teachers, email) {
			add(c.Classroom)
		}
	}
	return out
}

// forClassrooms says a school email reaches the viewer. A list says whom it
// went to: every family's always, a classroom's ("jays.parents") or a grade
// band's ("jaysandravens") when it names one of their classrooms. Mail the
// school sends through Veracross - the newsletter, but a teacher's note to
// their class too - says nothing of whom it went to, so it waits until its
// audience is judged (internal/keypoints) and then reaches everyone, or the
// classrooms it was written to.
func forClassrooms(d *artifacts.Document, mine []string, audience string) bool {
	if d.Kind != artifacts.KindList {
		if audience == "" {
			return false
		}
		rooms := artifacts.Classrooms(audience)
		if len(rooms) == 0 {
			return true
		}
		for _, room := range rooms {
			if slices.Contains(mine, who.ClassroomSlug(room)) {
				return true
			}
		}
		return false
	}
	if slices.Contains(allFamilies, d.Channel) {
		return true
	}
	room, _, _ := strings.Cut(d.Channel, ".")
	for _, slug := range mine {
		if room == slug || (strings.Contains(room, "and") && slices.Contains(strings.Split(room, "and"), slug)) {
			return true
		}
	}
	return false
}
