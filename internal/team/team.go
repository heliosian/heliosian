package team

import (
	"errors"
	"image/color"
	"log/slog"
	"net/http"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/db"
	"heliosian/internal/describe"
	"heliosian/internal/imagesearch"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

var pages = []string{
	"/{$}", "/my", "/my/{email}", "/calendar", "/approvals", "/admin", "/years/{year}", "/activities/{path...}", "/v/{path...}",
}

type Deps struct {
	Data      *db.Store
	Bucket    *blob.Bucket
	Images    blob.Images
	Search    imagesearch.Search
	Describer *describe.Describer
	Style     *sharecard.Style
}

type Team struct {
	data   *db.Store
	bucket *blob.Bucket
	style  *sharecard.Style
}

func CardStyle(name, tagline func() string) *sharecard.Style {
	return &sharecard.Style{
		Palette: sharecard.Palette{
			Page: color.RGBA{0xee, 0xf6, 0xea, 0xff}, Brand: color.RGBA{0x0c, 0x4c, 0x54, 0xff}, Accent: color.RGBA{0x00, 0x74, 0x6f, 0xff},
			Ink: color.RGBA{0x0e, 0x3a, 0x42, 0xff}, Yellow: color.RGBA{0xf8, 0xd9, 0x08, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe4, 0xff},
		},
		Name: name, Tagline: tagline,
		Mark: "web/public/team/brand/logo-mark.png", Lockup: "web/public/team/brand/logo-lockup.png", Corner: "web/team/toolbar_background.png",
	}
}

func Register(mux *http.ServeMux, d Deps) *Team {
	t := &Team{data: d.Data, bucket: d.Bucket, style: d.Style}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, func(w http.ResponseWriter, r *http.Request) {
			serve.File(w, r, "web/team/index.html")
		})
	}
	d.Search.UserAgent = "HCA-Team image search (+https://team.heliosian.com)"
	d.Search.Register(mux, "/api/team", d.Images.Folder(), imagesearch.Members)
	mux.HandleFunc("GET /open/share/upcoming.png", t.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", t.shareCard)
	mux.HandleFunc("POST /api/team/describe", serve.JSON(func(r *http.Request, body describeBody) (map[string]string, error) {
		return describeActivity(r, d.Describer, body)
	}))
	return t
}

type describeBody struct {
	Title    string `json:"title"`
	Parent   string `json:"parent"`
	Category string `json:"category"`
	When     string `json:"when"`
	Notes    string `json:"notes"`
}

func describeActivity(r *http.Request, describer *describe.Describer, body describeBody) (map[string]string, error) {
	email := auth.Email(r)
	facts := describe.ActivityFacts{Title: body.Title, Parent: body.Parent, Category: body.Category, When: body.When, Notes: body.Notes}
	description, err := describer.Activity(r.Context(), email, facts)
	if errors.Is(err, claude.ErrTooMany) {
		return nil, access.Refuse(http.StatusTooManyRequests, "%v", err)
	}
	if errors.Is(err, describe.ErrTooLong) {
		return nil, access.Invalid("%v", err)
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "team: describe", "actor", email, "title", body.Title, "error", err)
		return nil, access.Refuse(http.StatusBadGateway, "could not write a description right now")
	}
	slog.InfoContext(r.Context(), "team: described", "actor", email, "title", body.Title)
	return map[string]string{"description": description}, nil
}
