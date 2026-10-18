package db

import (
	"net/url"

	"heliosian/internal/store"
)

func personHref(row store.Row, origin func(app string) string) string {
	return origin("who") + "/people/" + url.PathEscape(row["id"])
}

func groupHref(row store.Row, origin func(app string) string) string {
	key := row["slug"]
	if key == "" {
		key = row["id"]
	}
	key = url.PathEscape(key)
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
	return origin("who") + "/groups/" + key
}

func documentHref(row store.Row, docs func(id string) store.Row, origin func(app string) string) string {
	if row["kind"] == "wiki" {
		return origin("wiki") + (&url.URL{Path: wikiPathIn(docs, row["parent"], wikiSegment(row))}).EscapedPath()
	}
	return row["url"]
}

func hrefPath(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		panic("db: a generated href that doesn't parse: " + href)
	}
	return u.EscapedPath()
}
