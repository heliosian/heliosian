package app

import (
	"net/http"

	"heliosian/internal/auth"
	"heliosian/internal/model"
	"heliosian/internal/serve"
)

type lateView struct {
	Late  []model.Late `json:"late"`
	Admin bool         `json:"admin"`
}

func lateBirthdays(s *model.Store) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (lateView, error) {
		m := s.Model()
		email := m.Directory.Resolve(auth.Email(r))
		return lateView{m.Late(email), m.AdminList("birthday").IsAdmin(email)}, nil
	})
}
