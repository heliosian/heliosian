package team

import (
	"fmt"
	"strings"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/db"
	"heliosian/internal/store"
)

type tree struct {
	m        *db.Model
	groups   *db.Rows
	everyone string
}

func (t *Team) tree() tree {
	m := t.data.Model()
	tr := tree{m: m, groups: m.Table("GROUP")}
	for _, g := range tr.groups.All() {
		if g["kind"] == "group" && g["slug"] == "everyone" {
			tr.everyone = g["id"]
		}
	}
	return tr
}

func (t tree) group(id string) store.Row {
	if id == "" {
		return nil
	}
	g, _ := t.groups.Get(id)
	return g
}

func (t tree) isYear(g store.Row) bool {
	if g["kind"] != "activity" {
		return false
	}
	p := t.group(g["parent"])
	return p == nil || (p["kind"] == "category" && p["parent"] == "")
}

func (t tree) isActivity(g store.Row) bool {
	return g != nil && g["kind"] == "activity" && !t.isYear(g)
}

func (t tree) up(g store.Row) store.Row {
	p := t.group(g["parent"])
	for p != nil && p["kind"] == "category" {
		p = t.group(p["parent"])
	}
	return p
}

func (t tree) above(g store.Row) []store.Row {
	out := []store.Row{}
	for p := t.up(g); t.isActivity(p); p = t.up(p) {
		out = append(out, p)
	}
	return out
}

func (t tree) yearOf(g store.Row) string {
	for p := t.up(g); p != nil; p = t.up(p) {
		if t.isYear(p) {
			return p["name"]
		}
	}
	return ""
}

func (t tree) shown(g store.Row) bool {
	if !t.isActivity(g) || (g["status"] != "open" && g["status"] != "done") {
		return false
	}
	for n := g; n != nil; n = t.group(n["parent"]) {
		if n["visible_to"] != t.everyone || n["status"] == "pending" || n["status"] == "closed" {
			return false
		}
	}
	return true
}

func (t tree) path(g store.Row) string {
	return t.m.Link("GROUP", g, func(string) string { return "" })
}

func (t tree) named(key string) store.Row {
	if g := t.group(key); t.isActivity(g) {
		return g
	}
	slug := cells.NormalizePretty(key)
	for _, g := range t.groups.All() {
		if g["slug"] == slug && t.isActivity(g) {
			return g
		}
	}
	if a, ok := t.m.Table("ALIAS").Find(key); ok {
		if g := t.group(a["target"]); t.isActivity(g) {
			return g
		}
	}
	return nil
}

func (t tree) at(path string) store.Row {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	if len(segs) < 2 || (segs[0] != "v" && segs[0] != "activities") {
		return nil
	}
	return t.named(segs[len(segs)-1])
}

func (t tree) takenIn(g store.Row) int {
	n := 0
	for _, m := range t.m.Table("MEMBER").Referencing("group", g["id"]) {
		if m["member"] == "yes" {
			n++
		}
	}
	return n
}

func schoolYear(at time.Time) string {
	start := at.Year()
	if at.Month() < time.July {
		start--
	}
	return fmt.Sprintf("%d - %d", start, start+1)
}
