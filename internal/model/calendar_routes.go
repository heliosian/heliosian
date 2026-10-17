package model

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/id"
	"heliosian/internal/imagesearch"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const shell = "web/when/index.html"

var pages = []string{"/{$}", "/c/{token}", "/day/{date}", "/mine", "/mine/{list}", "/admin"}

var eventPages = []string{"/e/{id...}", "/events/{id...}"}

type calendarApp struct {
	store  *Store
	pinned *Model
	images blob.Images
	search imagesearch.Search
	queue  *store.Queue
	mail   CalendarMail
	idKey  []byte
}

type CalendarDeps struct {
	Store  *Store
	Images blob.Images
	Search imagesearch.Search
	Mail   CalendarMail
	Queue  *store.Queue
	IDKey  []byte
}

func newCalendarApp(d CalendarDeps) calendarApp {
	d.Search.UserAgent = "Helios When image search (+https://when.heliosian.com)"
	return calendarApp{store: d.Store, images: d.Images, search: d.Search, queue: d.Queue, mail: d.Mail, idKey: d.IDKey}
}

func (a calendarApp) at(m *Model) calendarApp {
	a.pinned = m
	return a
}

func (a calendarApp) all() *Model {
	if a.pinned != nil {
		return a.pinned
	}
	return a.store.Model()
}

func (a calendarApp) model() *Calendar {
	return a.all().Calendar
}

func (a calendarApp) directory() *Directory {
	return a.all().Directory
}

func (a calendarApp) settings() *Config {
	return a.all().Config
}

func (a calendarApp) clock() time.Time {
	if a.pinned != nil && a.pinned.scope != nil {
		return a.pinned.scope.now
	}
	return now()
}

func (a calendarApp) sources() AudienceSources {
	return a.all().Audience(a.clock())
}

func RegisterCalendar(mux *http.ServeMux, d CalendarDeps) CalendarHooks {
	a := newCalendarApp(d)
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	for _, page := range eventPages {
		mux.HandleFunc("GET "+page, a.eventPage)
	}
	mux.HandleFunc("GET /api/when/invites/options", serve.JSON(a.groupOptions))
	mux.HandleFunc("POST /api/when/invites/preview", serve.JSON(a.groupPreview))
	a.search.Register(mux, "/api/when", a.images.Folder(), imagesearch.Members)
	mux.HandleFunc("GET /open/flyer/{id...}", a.flyer)
	mux.HandleFunc("GET /open/banner/{id...}", a.banner)
	return CalendarHooks{app: a}
}

func (a calendarApp) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a calendarApp) eventPage(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("id")
	canonical := a.canonical(key)
	if canonical == key {
		serve.File(w, r, shell)
		return
	}
	e := a.anyEvent("", canonical)
	if e == nil {
		serve.File(w, r, shell)
		return
	}
	to := EventPath(e)
	if r.URL.RawQuery != "" {
		to += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, to, http.StatusMovedPermanently)
}

var now = func() time.Time {
	return time.Now().In(Location)
}

func (a calendarApp) actor(r *http.Request) access.Actor {
	return a.all().actor(r, "when")
}

type feedBody struct {
	Token      string   `json:"token"`
	Name       string   `json:"name"`
	Emoji      string   `json:"emoji"`
	Classrooms []string `json:"classrooms"`
	Tags       []string `json:"tags"`
}

type CalendarHooks struct {
	app calendarApp
}

type tokenBody struct {
	Token string `json:"token"`
}

type tokensBody struct {
	Tokens []string `json:"tokens"`
}

func feedEmoji(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 12 {
		s = string(r[:12])
	}
	return s
}

type keywordsBody struct {
	ID       string   `json:"id"`
	Keywords []string `json:"keywords"`
}

var addressForm = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

func validAddress(address string) bool {
	_, isID := id.Parse(address)
	return addressForm.MatchString(address) && !isID
}

func shiftWhen(when string, weeks int) string {
	if when == "" {
		return ""
	}
	layout := DateFormat
	if len(when) > len(DateFormat) {
		layout = DateTimeFormat
	}
	t, err := time.ParseInLocation(layout, when, Location)
	if err != nil {
		return when
	}
	return t.AddDate(0, 0, 7*weeks).Format(layout)
}

type eventBody struct {
	ID          string   `json:"id"`
	Address     string   `json:"address"`
	Title       string   `json:"title"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Location    string   `json:"location"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	DayType     string   `json:"dayType"`
	Keywords    []string `json:"keywords"`
	Source      string   `json:"source"`
	Image       string   `json:"image"`
	Sharing     string   `json:"sharing"`
	RepeatWeeks int      `json:"repeatWeeks"`
	RepeatTimes int      `json:"repeatTimes"`
}

type moveBody struct {
	ID    string `json:"id"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type viewBody struct {
	Classrooms []string `json:"classrooms"`
	Tags       []string `json:"tags"`
}

type tagBody struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group"`
	Default     bool   `json:"default"`
	Image       string `json:"image"`
}

type tagsBody struct {
	Tags []tagBody `json:"tags"`
}

func (a calendarApp) linked(email string) []Linked {
	return a.all().LinkedEvents(email)
}
