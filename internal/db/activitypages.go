package db

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const (
	needsCount    = 4
	upcomingLead  = "Sign up for a shift, a booth or a committee."
	upcomingEmpty = "Nothing needs hands just now - check back soon."
	fallbackDesc  = "Sign up to help on HCA-Team, the HCA Volunteer Portal."
)

var (
	blurbImage = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	blurbLink  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	blurbLead  = regexp.MustCompile(`(?m)^\s*(#{1,6}\s|>\s?|[-*+]\s+)`)
	blurbMarks = strings.NewReplacer("**", "", "__", "", "*", "", "`", "")
	chipLine   = regexp.MustCompile(`(?m)^>[ \t]?\[!chip\][^\n]*(\n|$)`)
	calloutTag = regexp.MustCompile(`(?m)^(>[ \t]?)\[![A-Za-z]+\][ \t]*`)
)

func withoutMarkers(markdown string) string {
	return strings.TrimSpace(calloutTag.ReplaceAllString(chipLine.ReplaceAllString(markdown, ""), "$1"))
}

type ActivityPages struct {
	s      *Store
	bucket *blob.Bucket
	style  *sharecard.Style
}

func NewActivityPages(s *Store, bucket *blob.Bucket, style *sharecard.Style) *ActivityPages {
	return &ActivityPages{s: s, bucket: bucket, style: style}
}

func (a *ActivityPages) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", a.shareCard)
}

func (a *ActivityPages) PreviewHead(r *http.Request) string {
	tr := a.tree()
	origin := "https://" + r.Host
	var g store.Row
	if first := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]; first == "g" {
		g = tr.resolve(r.URL.Path)
	}
	if g == nil {
		return a.upcomingHead(tr, origin, time.Now().In(School))
	}
	desc := blurb(g["description"])
	if line := whenText(tr.timed(g)); line != "" {
		if desc != "" {
			desc = line + " — " + desc
		} else {
			desc = line
		}
	}
	if desc == "" {
		desc = fallbackDesc
	}
	title := g["name"]
	if under := tr.lineage(g); under != "" {
		title += " · " + under
	}
	return a.style.PreviewTags(title, desc, origin+g["path"], origin+"/open/share/"+g["id"]+".png")
}

func (a *ActivityPages) upcomingHead(tr activityTree, origin string, at time.Time) string {
	desc := a.style.Tagline() + ". " + upcomingLead
	if list := tr.needs(at); len(list) > 0 {
		names := []string{}
		for _, g := range list[:min(len(list), needsCount)] {
			name := g["name"]
			if note := tr.needNote(g); note != "" {
				name += " (" + note + ")"
			}
			names = append(names, name)
		}
		desc = "Volunteers needed: " + strings.Join(names, "; ") + ". " + upcomingLead
	}
	return a.style.PreviewTags(a.style.Name(), desc, origin+"/", origin+"/open/share/upcoming.png")
}

func (a *ActivityPages) shareUpcoming(w http.ResponseWriter, r *http.Request) {
	tr := a.tree()
	listing := &sharecard.Listing{Heading: "Volunteers needed", Empty: upcomingEmpty}
	for _, g := range tr.needs(time.Now().In(School)) {
		if len(listing.Items) == needsCount {
			break
		}
		listing.Items = append(listing.Items, sharecard.Item{Title: g["name"], Note: tr.needNote(g)})
	}
	card := sharecard.Card{Title: a.style.Name(), Subtitle: a.style.Tagline(), Button: "See what's open", Listing: listing}
	a.style.Serve(w, r, card, listing.Words()...)
}

func (a *ActivityPages) shareCard(w http.ResponseWriter, r *http.Request) {
	tr := a.tree()
	g := tr.group(strings.TrimSuffix(r.PathValue("id"), ".png"))
	if !tr.isActivity(g) {
		http.NotFound(w, r)
		return
	}
	name, whole := tr.picture(g)
	under := tr.lineage(g)
	day, hours := whenLines(tr.timed(g))
	card := sharecard.Card{
		Kicker: under, Title: g["name"], Picture: tr.m.cardPicture(r.Context(), a.bucket, "activity pages", g, name), Whole: whole,
		Lines: whenCardLines(day, hours, ""),
	}
	a.style.Serve(w, r, card, g["name"], under, day, hours, name)
}

func (t activityTree) lineage(g store.Row) string {
	names := []string{}
	for _, a := range t.above(g) {
		names = append([]string{a["name"]}, names...)
	}
	return strings.Join(names, " › ")
}

func (t activityTree) timed(g store.Row) store.Row {
	for _, n := range append([]store.Row{g}, t.above(g)...) {
		if n["start"] != "" || n["timing"] != "" {
			return n
		}
	}
	return g
}

func (t activityTree) picture(g store.Row) (string, bool) {
	for _, n := range append([]store.Row{g}, t.above(g)...) {
		if n["flyer"] != "" {
			return n["flyer"], true
		}
		if image := t.m.groupPhoto(n["id"]); image != "" {
			return image, false
		}
	}
	return "", false
}

func (t activityTree) needs(at time.Time) []store.Row {
	year, today := activityYear(at), at.Format(DateLayout)
	dated, undated := []store.Row{}, []store.Row{}
	for _, g := range t.groups.All() {
		if !t.shown(g) || g["status"] != "open" || len(t.above(g)) > 0 || t.yearOf(g) != year || g["join"] == "none" {
			continue
		}
		if spots, _ := strconv.Atoi(g["capacity"]); spots > 0 && t.takenIn(g) >= spots {
			continue
		}
		if g["start"] == "" {
			undated = append(undated, g)
		} else if g["start"][:min(len(g["start"]), len(DateLayout))] >= today {
			dated = append(dated, g)
		}
	}
	slices.SortStableFunc(dated, func(x, y store.Row) int { return strings.Compare(x["start"], y["start"]) })
	return append(dated, undated...)
}

func (t activityTree) needNote(g store.Row) string {
	parts := []string{}
	if day, _ := whenLines(g); day != "" {
		parts = append(parts, day)
	}
	if spots, _ := strconv.Atoi(g["capacity"]); spots > 0 {
		left := spots - t.takenIn(g)
		if left == 1 {
			parts = append(parts, "1 spot left")
		} else {
			parts = append(parts, fmt.Sprintf("%d spots left", left))
		}
	}
	return strings.Join(parts, " · ")
}

func blurb(markdown string) string {
	text := blurbImage.ReplaceAllString(withoutMarkers(markdown), "")
	text = blurbLink.ReplaceAllString(blurbLead.ReplaceAllString(text, ""), "$1")
	text = strings.Join(strings.Fields(blurbMarks.Replace(text)), " ")
	if len(text) > 200 {
		cut := strings.LastIndex(text[:200], " ")
		if cut < 120 {
			cut = 200
		}
		text = text[:cut] + "…"
	}
	return text
}

func whenText(g store.Row) string {
	day, hours := whenLines(g)
	if hours != "" {
		return day + " · " + hours
	}
	return day
}

func whenLines(g store.Row) (string, string) {
	if g["timing"] != "" {
		return g["timing"], ""
	}
	if days, hours := spanLines(g); days != "" {
		return days, hours
	}
	start, err := time.Parse(MomentLayout, g["start"])
	if err != nil {
		if day, err := time.Parse(DateLayout, g["start"]); err == nil {
			return day.Format("Monday, January 2"), ""
		}
		return "", ""
	}
	day := start.Format("Monday, January 2")
	end, err := time.Parse(MomentLayout, g["end"])
	if err != nil {
		return day, start.Format("3:04 PM")
	}
	return day, sharecard.Hours(start, end)
}

func spanLines(g store.Row) (string, string) {
	start, err := cells.When(g["start"])
	if err != nil {
		return "", ""
	}
	end, err := cells.When(g["end"])
	if err != nil || (end.Year() == start.Year() && end.YearDay() == start.YearDay()) {
		return "", ""
	}
	days := start.Format("Monday, January 2") + " – " + end.Format("Monday, January 2")
	if len(g["start"]) > len(DateLayout) && len(g["end"]) > len(DateLayout) {
		return days, start.Format("Mon 3:04 PM") + " – " + end.Format("Mon 3:04 PM")
	}
	if len(g["start"]) > len(DateLayout) {
		return days, start.Format("Mon 3:04 PM")
	}
	return days, ""
}
