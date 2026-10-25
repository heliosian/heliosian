package db

import (
	"fmt"
	"strings"
	"time"

	"heliosian/internal/store"
)

type activityTree struct {
	m        *Model
	groups   *Rows
	everyone string
}

func (a *ActivityPages) tree() activityTree {
	m := a.s.Model()
	return activityTree{m: m, groups: m.Table("GROUP"), everyone: m.slugGroup("everyone")}
}

func (t activityTree) group(id string) store.Row {
	if id == "" {
		return nil
	}
	g, _ := t.groups.Get(id)
	return g
}

func (t activityTree) isYear(g store.Row) bool {
	if g["kind"] != "activity" {
		return false
	}
	p := t.group(g["parent"])
	return p == nil || (p["kind"] == "category" && p["parent"] == "")
}

func (t activityTree) isActivity(g store.Row) bool {
	return g != nil && g["kind"] == "activity" && !t.isYear(g)
}

func (t activityTree) up(g store.Row) store.Row {
	p := t.group(g["parent"])
	for p != nil && p["kind"] == "category" {
		p = t.group(p["parent"])
	}
	return p
}

func (t activityTree) above(g store.Row) []store.Row {
	out := []store.Row{}
	for p := t.up(g); t.isActivity(p); p = t.up(p) {
		out = append(out, p)
	}
	return out
}

func (t activityTree) yearOf(g store.Row) string {
	for p := t.up(g); p != nil; p = t.up(p) {
		if t.isYear(p) {
			return p["name"]
		}
	}
	return ""
}

func (t activityTree) shown(g store.Row) bool {
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

func (t activityTree) named(key string) store.Row {
	if g := t.m.sharedGroup(key); t.isActivity(g) {
		return g
	}
	return nil
}

func (t activityTree) at(path string) store.Row {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	if len(segs) < 2 || segs[0] != "g" {
		return nil
	}
	return t.named(segs[len(segs)-1])
}

func (t activityTree) takenIn(g store.Row) int {
	n := 0
	for _, m := range t.m.Table("MEMBER").Referencing("group", g["id"]) {
		if m["member"] == "yes" {
			n++
		}
	}
	return n
}

func activityYear(at time.Time) string {
	start := at.Year()
	if at.Month() < time.July {
		start--
	}
	return fmt.Sprintf("%d - %d", start, start+1)
}
