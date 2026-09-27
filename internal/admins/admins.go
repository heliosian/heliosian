package admins

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"sort"

	"heliosian/internal/access"
	"heliosian/internal/config"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const Tab = "Admins"

var (
	Columns = []string{"Email"}
	Spec    = store.Tab{Name: Tab, Columns: Columns, Key: Columns}
)

func Read(tables store.Tables) []string {
	emails := []string{}
	for _, row := range tables[Tab] {
		emails = append(emails, row["Email"])
	}
	return config.NormalizeEmails(emails)
}

type List struct {
	superAdmins func() []string
	listed      func() []string
	commit      func(ctx context.Context, actor access.Actor, ops ...store.Op) error
}

func New(superAdmins, listed func() []string, commit func(ctx context.Context, actor access.Actor, ops ...store.Op) error) List {
	return List{superAdmins: superAdmins, listed: listed, commit: commit}
}

func (l List) IsSuperAdmin(email string) bool {
	return slices.Contains(l.superAdmins(), config.NormalizeEmail(email))
}

func (l List) IsAdmin(email string) bool {
	return l.IsSuperAdmin(email) || slices.Contains(l.listed(), config.NormalizeEmail(email))
}

func (l List) Admins() []string {
	out := config.NormalizeEmails(append(slices.Clone(l.listed()), l.superAdmins()...))
	sort.Strings(out)
	return out
}

type edit struct {
	Admins []string `json:"admins"`
}

func Register(mux *http.ServeMux, app string, l List, actor func(r *http.Request) access.Actor, state func(r *http.Request, actor access.Actor) map[string]any) {
	mux.HandleFunc("GET /api/admin/state", serve.JSON(func(r *http.Request, _ serve.None) (map[string]any, error) {
		v := actor(r)
		if !v.Admin {
			return nil, access.Forbidden("admin access required")
		}
		view := state(r, v)
		view["email"] = v.Email
		view["admins"] = l.Admins()
		view["isSuperAdmin"] = l.IsSuperAdmin(v.Email)
		return view, nil
	}))
	mux.HandleFunc("POST /api/admin/admins", serve.JSON(func(r *http.Request, body edit) (serve.None, error) {
		v := actor(r)
		ops, admins, err := l.set(v, body.Admins)
		if err != nil {
			return serve.None{}, err
		}
		if err := l.commit(r.Context(), v, ops...); err != nil {
			return serve.None{}, err
		}
		slog.InfoContext(r.Context(), app+":set the admin list", "actor", v.Email, "admins", admins)
		return serve.None{}, nil
	}))
}
