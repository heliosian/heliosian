package app

import (
	"net/http"

	"heliosian/internal/auth"
	"heliosian/internal/model"
	"heliosian/internal/serve"
)

type approvalsView struct {
	Waiting []model.Approval `json:"waiting"`
}

func approvals(s *model.Store) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (approvalsView, error) {
		m := s.Model()
		return approvalsView{m.Approvals(m.Directory.Resolve(auth.Email(r)))}, nil
	})
}
