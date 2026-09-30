package model

import (
	"net/http"
	"strings"

	"heliosian/internal/blob"
	"heliosian/internal/serve"
)

type DirectoryRoutes struct {
	Store   *Store
	Media   *blob.Store
	MapsKey string
}

func RegisterDirectory(mux *http.ServeMux, d DirectoryRoutes) {
	mux.HandleFunc("GET /api/directory/model", serve.JSON(d.model))
	registerTags(mux, d.Store)
	registerPeopleAdmin(mux, d.Store, d.Media)
	registerInviteTemplates(mux, d.Store)
}

type user struct {
	Name    string `json:"name"`
	Initial string `json:"initial"`
	Email   string `json:"email"`
	Slug    string `json:"slug"`
	IsAdmin bool   `json:"isAdmin"`
}

type directoryView struct {
	*Directory
	User       user       `json:"user"`
	MapsKey    string     `json:"mapsKey"`
	Tags       []Tag      `json:"tags"`
	SharedTags []Tag      `json:"sharedTags"`
	Lists      []MagicTag `json:"lists"`
	EditAnyone bool       `json:"editAnyone,omitempty"`
}

func (d DirectoryRoutes) model(r *http.Request, _ serve.None) (directoryView, error) {
	m := d.Store.Model()
	v := m.actor(r, "who")
	effective := v.Email
	directory := m.Directory
	name := directory.DisplayName(effective)
	return directoryView{
		Directory:  directory,
		User:       user{Name: name, Initial: strings.ToUpper(name[:1]), Email: effective, Slug: Slug(effective), IsAdmin: v.May(Administer)},
		MapsKey:    d.MapsKey,
		Tags:       directory.Tags(effective),
		SharedTags: directory.SharedTags(effective),
		Lists:      m.MagicTagsOf(effective, now()),
		EditAnyone: v.May(EditAnyone),
	}, nil
}
