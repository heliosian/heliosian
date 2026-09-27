package app

import (
	"net/http"
	"sort"

	"heliosian/internal/admins"
	"heliosian/internal/auth"
	"heliosian/internal/celebrate"
	"heliosian/internal/serve"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

type approvalsView struct {
	Waiting []admins.Approval `json:"waiting"`
}

func approvals(directory *who.Cache, teamCache *team.Cache, celebrateCache *celebrate.Cache, calendarCache *when.Cache) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (approvalsView, error) {
		email := directory.Model().Resolve(auth.Email(r))
		out := append(append(teamCache.Pending(email), celebrateCache.Pending(email)...), calendarCache.Pending(email)...)
		sort.SliceStable(out, func(i, j int) bool {
			if (out[i].Start == "") != (out[j].Start == "") {
				return out[j].Start == ""
			}
			return out[i].Start < out[j].Start
		})
		return approvalsView{out}, nil
	})
}
