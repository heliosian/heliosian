package app

import (
	"net/http"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/model"
	"heliosian/internal/serve"
)

type celebrateView struct {
	Parties []model.EventCard `json:"parties"`
}

func celebrateWidget(directory *model.DirectoryCache, calendarCache *model.CalendarCache, linked func(email string) []model.Linked) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (celebrateView, error) {
		people := directory.Model()
		email := people.Resolve(auth.Email(r))
		return celebrateView{calendarCache.Model().PartiesFor(people, email, linked(email), time.Now().In(model.Location))}, nil
	})
}
