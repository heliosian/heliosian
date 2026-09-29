package model

import (
	"net/http"
	"strings"

	"heliosian/internal/blob"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
)

type DirectoryRoutes struct {
	Cache      *DirectoryCache
	Invites    *InviteTemplatesCache
	Media      *blob.Store
	MapsKey    string
	Parties    *PartiesCache
	Activities *ActivitiesCache
	EmailLists *EmailListsCache
}

func RegisterDirectory(mux *http.ServeMux, d DirectoryRoutes) {
	mux.HandleFunc("GET /api/directory/model", serve.JSON(d.model))
	registerTags(mux, d.Cache)
	registerPeopleAdmin(mux, d.Cache, d.Media)
	registerInviteTemplates(mux, d.Cache, d.Invites)
}

func (c *DirectoryCache) Member(email string) bool {
	model := c.Model()
	return model.Member(model.Resolve(mail.Normalize(email)))
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
	v := requestActor(d.Cache, r)
	effective := v.Email
	directory := d.Cache.Model()
	name := directory.DisplayName(effective)
	return directoryView{
		Directory:  directory,
		User:       user{Name: name, Initial: strings.ToUpper(name[:1]), Email: effective, Slug: Slug(effective), IsAdmin: v.May(Administer)},
		MapsKey:    d.MapsKey,
		Tags:       directory.Tags(effective),
		SharedTags: directory.SharedTags(effective),
		Lists:      append(directory.RoomParentTags(effective), ManagedMagicTags(directory, d.Parties.Model(), d.Activities.Model(), d.EmailLists.Model(), d.Activities, effective, now())...),
		EditAnyone: v.May(EditAnyone),
	}, nil
}
