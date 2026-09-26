package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	"heliosian/internal/auth"
	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

// An approval is one thing waiting for an admin's word, as the shared
// toolbar's badge lists it in every app: which app, the thing's title and
// start, and its page there, a path on that app's host.
type approval struct {
	App   string `json:"app"`
	Title string `json:"title"`
	Start string `json:"start,omitempty"`
	Path  string `json:"path"`
}

// approvals answers GET /api/apps/approvals on every app's host: what waits
// for the viewer's approval in each app they are an admin of - HCA-Team's
// pending activities, Celebrate's pending parties, the calendar's shared
// events - with or without Super Admin Mode, approvals being an admin's
// alert; soonest first, the undated last.
func approvals(directory *who.Cache, teamCache *team.Cache, celebrateCache *celebrate.Cache, calendarCache *calendar.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
					out = append(out, approval{App: "calendar", Title: e.Title, Start: e.Start, Path: calendar.EventPath(e)})
				}
			}
		}
		sort.SliceStable(out, func(i, j int) bool {
			if (out[i].Start == "") != (out[j].Start == "") {
				return out[j].Start == ""
			}
			return out[i].Start < out[j].Start
		})
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Waiting []approval `json:"waiting"`
		}{out}); err != nil {
			slog.ErrorContext(r.Context(), "encode approvals", "error", err)
		}
	}
}
