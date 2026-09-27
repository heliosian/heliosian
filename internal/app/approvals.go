package app

import (
	"net/http"
	"sort"

	"heliosian/internal/auth"
	"heliosian/internal/celebrate"
	"heliosian/internal/serve"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

type approval struct {
	App   string `json:"app"`
	Title string `json:"title"`
	Start string `json:"start,omitempty"`
	Path  string `json:"path"`
}

type approvalsView struct {
	Waiting []approval `json:"waiting"`
}

func approvals(directory *who.Cache, teamCache *team.Cache, celebrateCache *celebrate.Cache, calendarCache *when.Cache) http.HandlerFunc {
	return serve.JSON(func(r *http.Request, _ serve.None) (approvalsView, error) {
		email := directory.Model().Resolve(auth.Email(r))
		out := []approval{}
		if teamCache.IsAdmin(email) {
			m := teamCache.Model()
			var walk func([]*team.Activity)
			walk = func(list []*team.Activity) {
				for _, a := range list {
					if a.Status == team.StatusPending {
						out = append(out, approval{App: "team", Title: a.Title, Start: a.Start, Path: m.PathOf(a)})
					}
					walk(a.Children)
				}
			}
			walk(m.Activities)
		}
		if celebrateCache.IsAdmin(email) {
			m := celebrateCache.Model()
			for _, p := range m.Parties {
				if p.Status == celebrate.StatusPending {
					out = append(out, approval{App: "celebrate", Title: p.Title, Start: p.Start, Path: m.PathOf(p)})
				}
			}
		}
		if calendarCache.IsAdmin(email) {
			for _, e := range calendarCache.Model().Pending {
				if e.Pending && !e.Declined && !e.Cancelled {
					out = append(out, approval{App: "when", Title: e.Title, Start: e.Start, Path: when.EventPath(e)})
				}
			}
		}
		sort.SliceStable(out, func(i, j int) bool {
			if (out[i].Start == "") != (out[j].Start == "") {
				return out[j].Start == ""
			}
			return out[i].Start < out[j].Start
		})
		return approvalsView{out}, nil
	})
}
