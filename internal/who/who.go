package who

import (
	"net/http"
	"strings"

	"heliosian/internal/auth"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

var sections = []string{"people", "classrooms", "staff", "map", "email-list", "greenvelope", "my-privacy", "admin"}

var legacy = map[string]string{
	"people":   "/people",
	"explore":  "/classrooms",
	"myfamily": "/my-family",
	"staff":    "/staff",
	"map":      "/map",
	"emails":   "/email-list",
}

type app struct {
	cache   *Cache
	mapsKey string
	lister  Lister
}

func effectiveEmail(cache *Cache, r *http.Request) string {
	return cache.Model().Resolve(strings.ToLower(auth.Email(r)))
}

func Member(cache *Cache, email string) bool {
	return cache.Model().Member(cache.Model().Resolve(strings.ToLower(strings.TrimSpace(email))))
}

func OptInForm(optIn func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, optIn(), http.StatusFound)
	})
}

func Register(mux *http.ServeMux, cache *Cache, mapsKey string, lister Lister, about *sharecard.About) {
	a := app{cache: cache, mapsKey: mapsKey, lister: lister}
	mux.Handle("GET /open/share/about.png", about)
	for _, section := range sections {
		mux.HandleFunc("GET /"+section, a.page)
	}
	mux.HandleFunc("GET /my-family", a.myFamily)
	mux.HandleFunc("GET /people/{email}", a.page)
	mux.HandleFunc("GET /families/{key}", a.page)
	mux.HandleFunc("GET /classrooms/{name}", a.page)
	mux.HandleFunc("GET /grades/{name}", a.page)
	mux.HandleFunc("GET /dl/", a.legacyRedirect)
	mux.HandleFunc("GET /api/directory/model", serve.JSON(a.model))
}

func (a app) myFamily(w http.ResponseWriter, r *http.Request) {
	model := a.cache.Model()
	email := effectiveEmail(a.cache, r)
	if family, ok := model.FamilyOf(email); ok {
		http.Redirect(w, r, FamilyPath(family.Key), http.StatusFound)
		return
	}
	http.Error(w, "no family record for "+email, http.StatusNotFound)
}

func (a app) legacyRedirect(w http.ResponseWriter, r *http.Request) {
	first, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/dl/"), "/")
	target, ok := legacy[first]
	if !ok {
		target = "/people"
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, "web/who/index.html")
}

type user struct {
	Name    string `json:"name"`
	Initial string `json:"initial"`
	Email   string `json:"email"`
	Slug    string `json:"slug"`
	IsAdmin bool   `json:"isAdmin"`
}

type modelView struct {
	*Model
	User       user   `json:"user"`
	MapsKey    string `json:"mapsKey"`
	Tags       []Tag  `json:"tags"`
	SharedTags []Tag  `json:"sharedTags"`
	Lists      []List `json:"lists"`
	SuperEdit  bool   `json:"superEdit,omitempty"`
}

func (a app) model(r *http.Request, _ serve.None) (modelView, error) {
	v := requestActor(a.cache, r)
	effective := v.Email
	name := a.cache.Model().DisplayName(effective)
	slug := Slug(effective)
	return modelView{
		Model:      a.cache.Model(),
		User:       user{Name: name, Initial: strings.ToUpper(name[:1]), Email: effective, Slug: slug, IsAdmin: v.May(Administer)},
		MapsKey:    a.mapsKey,
		Tags:       a.cache.Model().Tags(effective),
		SharedTags: a.cache.Model().SharedTags(effective),
		Lists:      append(a.cache.Model().RoomParentLists(effective), a.lister.Lists(effective)...),
		SuperEdit:  v.May(EditAnyone),
	}, nil
}
