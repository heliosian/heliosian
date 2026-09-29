package app

import (
	"net/http"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/model"
	"heliosian/internal/serve"
	"heliosian/internal/when"
)

type celebrateView struct {
	Parties []when.Card `json:"parties"`
}

func celebrateWidget(directory *model.DirectoryCache, calendarCache *when.Cache, linked func(email string) []when.Linked) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (celebrateView, error) {
		people := directory.Model()
		email := people.Resolve(auth.Email(r))
		return celebrateView{calendarCache.Model().PartiesFor(people, email, linked(email), time.Now().In(when.Location))}, nil
	})
}
