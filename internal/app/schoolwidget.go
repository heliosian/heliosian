package app

import (
	"net/http"
	"time"

	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/keypoints"
	"heliosian/internal/model"
	"heliosian/internal/serve"
	"heliosian/internal/when"
)

const schoolDays = int(keypoints.Window / (24 * time.Hour))

type schoolView struct {
	Emails []artifacts.SchoolEmail `json:"emails"`
}

func schoolWidget(directory *model.DirectoryCache, artifactsCache *artifacts.Cache) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (schoolView, error) {
		people := directory.Model()
		email := people.Resolve(auth.Email(r))
		since := time.Now().In(when.Location).AddDate(0, 0, -schoolDays).Format("2006-01-02")
		return schoolView{artifactsCache.Model().SchoolMail(people, email, since)}, nil
	})
}
