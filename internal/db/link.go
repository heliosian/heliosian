package db

import (
	"net/url"

	"heliosian/internal/cells"
	"heliosian/internal/store"
)

func (m *Model) Link(table string, row store.Row, origin func(app string) string) string {
	key := row["slug"]
	if key == "" {
		key = row["id"]
	}
	key = url.PathEscape(key)
	switch table {
	case "PERSON":
		return origin("who") + "/people/" + url.PathEscape(row["id"])
	case "DOCUMENT":
		if row["kind"] == "wiki" {
			return origin("wiki") + (&url.URL{Path: m.WikiPath(row)}).EscapedPath()
		}
		return row["url"]
	case "GROUP":
		switch row["kind"] {
		case "family":
			return origin("who") + "/families/" + key
		case "classroom":
			return origin("who") + "/classrooms/" + key
		case "grade":
			return origin("who") + "/grades/" + key
		case "event":
			return origin("when") + "/e/" + key
		case "activity":
			return origin("team") + "/v/" + key
		case "party":
			return origin("celebrate") + "/p/" + key
		}
		if mail, _ := cells.YesNo(row["mail"], false); mail {
			return origin("loop") + "/groups/" + key
		}
	}
	return ""
}
