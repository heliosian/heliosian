package db

import (
	"net/url"
	"slices"

	"heliosian/internal/store"
)

var groupHome = map[string]string{"event": "when", "activity": "team", "party": "celebrate"}

func pageKey(row store.Row) string {
	if row["slug"] != "" {
		return url.PathEscape(row["slug"])
	}
	return url.PathEscape(row["id"])
}

func personLinks(row store.Row, origin func(app string) string) {
	row["path"] = "/p/" + pageKey(row)
	row["who_href"] = origin("who") + row["path"]
	row["href"] = row["who_href"]
}

func groupLinks(row store.Row, origin func(app string) string) {
	path := "/g/" + pageKey(row)
	kind := row["kind"]
	on := func(app string, shown bool) {
		row[app+"_href"] = ""
		if shown {
			row[app+"_href"] = origin(app) + path
		}
	}
	on("who", slices.Contains([]string{"family", "classroom", "grade"}, kind))
	on("loop", slices.Contains([]string{"group", "event", "party", "activity"}, kind))
	on("when", slices.Contains(sharedKinds, kind) && row["start"] != "")
	on("team", kind == "activity")
	on("celebrate", kind == "party")
	row["path"] = path
	switch {
	case groupHome[kind] != "":
		row["href"] = origin(groupHome[kind]) + path
	case row["who_href"] != "":
		row["href"] = row["who_href"]
	default:
		row["href"] = row["loop_href"]
	}
}

func documentLinks(row store.Row, docs func(id string) store.Row, origin func(app string) string) {
	if row["kind"] != "wiki" {
		row["path"] = ""
		row["href"] = row["url"]
		return
	}
	row["path"] = (&url.URL{Path: wikiPathIn(docs, row["parent"], wikiSegment(row))}).EscapedPath()
	row["href"] = origin("wiki") + row["path"]
}
