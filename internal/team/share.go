package team

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"heliosian/internal/sharecard"
)

var cardStyle = &sharecard.Style{
	Page: color.RGBA{0xee, 0xf6, 0xea, 0xff}, Brand: color.RGBA{0x0c, 0x4c, 0x54, 0xff}, Accent: color.RGBA{0x00, 0x74, 0x6f, 0xff},
	Ink: color.RGBA{0x0e, 0x3a, 0x42, 0xff}, Yellow: color.RGBA{0xf8, 0xd9, 0x08, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe4, 0xff},
	Wordmark: "HCA-Team", Tagline: "HCA VOLUNTEER PORTAL",
	Mark: "web/public/team/brand/logo-mark.png", Lockup: "web/public/team/brand/logo-lockup.png", Corner: "web/team/toolbar_background.png",
}

func ShareWords(name, tagline func() string) {
	shareName = name
	cardStyle.TaglineNow = tagline
}

var shareName func() string

func title() string {
	if shareName != nil {
		if now := strings.TrimSpace(shareName()); now != "" {
			return now
		}
	}
	return cardStyle.Wordmark
}

func previewable(m *Model, a *Activity) bool {
	return a != nil && (a.Status == StatusOpen || a.Status == StatusDone) && m.VisibleTo(a, "", false)
}

func timed(m *Model, a *Activity) *Activity {
	for n := a; n != nil; n = m.byID[n.Parent] {
		if n.Start != "" || n.Timing != "" {
			return n
		}
	}
	return a
}

func lineage(m *Model, a *Activity) string {
	names := []string{}
	for n := m.byID[a.Parent]; n != nil; n = m.byID[n.Parent] {
		names = append([]string{n.Title}, names...)
	}
	return strings.Join(names, " › ")
}

func when(a *Activity) string {
	day, hours := whenLines(a)
	if hours != "" {
		return day + " · " + hours
	}
	return day
}

func blurb(a *Activity) string {
	text := strings.Join(strings.Fields(a.Description), " ")
	if len(text) > 200 {
		cut := strings.LastIndex(text[:200], " ")
		if cut < 120 {
			cut = 200
		}
		text = text[:cut] + "…"
	}
	return text
}

func PreviewHead(cache *Cache) func(r *http.Request) string {
	return func(r *http.Request) string {
		model := cache.Model()
		origin := "https://" + r.Host
		first := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
		var a *Activity
		if first == "v" || first == "activities" {
			a = model.Resolve(r.URL.Path)
		}
		if !previewable(model, a) {
			return upcomingHead(model, origin, time.Now().In(local))
		}
		desc := blurb(a)
		if line := when(timed(model, a)); line != "" {
			if desc != "" {
				desc = line + " — " + desc
			} else {
				desc = line
			}
		}
		if desc == "" {
			desc = "Sign up to help on HCA-Team, the HCA Volunteer Portal."
		}
		title := a.Title
		if under := lineage(model, a); under != "" {
			title = a.Title + " · " + under
		}
		return previewTags(title, desc, origin+model.PathOf(a), origin+"/open/share/"+a.ID+".png")
	}
}

func previewTags(title, desc, url, image string) string {
	return sharecard.PreviewTags("HCA-Team", title, desc, url, image)
}

func needs(m *Model, at time.Time) []*Activity {
	year, today := SchoolYear(at), at.Format(DateFormat)
	dated, undated := []*Activity{}, []*Activity{}
	for _, a := range m.Activities {
		if a.Year != year || a.Status != StatusOpen || !m.VisibleTo(a, "", false) || a.VolunteersComplete || (a.Spots > 0 && len(a.Volunteers) >= a.Spots) {
			continue
		}
		if a.Start == "" {
			undated = append(undated, a)
		} else if a.Start[:min(len(a.Start), len(DateFormat))] >= today {
			dated = append(dated, a)
		}
	}
	slices.SortStableFunc(dated, func(x, y *Activity) int { return strings.Compare(x.Start, y.Start) })
	return append(dated, undated...)
}

const needsCount = 4

func needNote(a *Activity) string {
	parts := []string{}
	if day, _ := whenLines(a); day != "" {
		parts = append(parts, day)
	}
	if a.Spots > 0 {
		left := a.Spots - len(a.Volunteers)
		if left == 1 {
			parts = append(parts, "1 spot left")
		} else {
			parts = append(parts, fmt.Sprintf("%d spots left", left))
		}
	} else if a.CoLeaderNeeded {
		parts = append(parts, "co-chair wanted")
	}
	return strings.Join(parts, " · ")
}

const (
	upcomingLead  = "Sign up for a shift, a booth or a committee."
	upcomingEmpty = "Nothing needs hands just now - check back soon."
)

func upcomingHead(m *Model, origin string, at time.Time) string {
	desc := cardStyle.TaglineText() + ". " + upcomingLead
	if list := needs(m, at); len(list) > 0 {
		names := []string{}
		for _, a := range list[:min(len(list), needsCount)] {
			name := a.Title
			if note := needNote(a); note != "" {
				name += " (" + note + ")"
			}
			names = append(names, name)
		}
		desc = "Volunteers needed: " + strings.Join(names, "; ") + ". " + upcomingLead
	}
	return previewTags(title(), desc, origin+"/", origin+"/open/share/upcoming.png")
}

func (a app) shareUpcoming(w http.ResponseWriter, r *http.Request) {
	listing := &sharecard.Listing{Heading: "Volunteers needed", Empty: upcomingEmpty}
	for _, act := range needs(a.cache.Model(), time.Now().In(local)) {
		if len(listing.Items) == needsCount {
			break
		}
		listing.Items = append(listing.Items, sharecard.Item{Title: act.Title, Note: needNote(act)})
	}
	card := sharecard.Card{Title: title(), Subtitle: cardStyle.TaglineText(), Button: "See what's open", Listing: listing}
	cardStyle.Serve(w, r, card, sharecard.ETag(append(listing.Words(), title(), cardStyle.TaglineText())...))
}

func (a app) shareCard(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("id"), ".png")
	model := a.cache.Model()
	act := model.Activity(id)
	if !previewable(model, act) {
		http.NotFound(w, r)
		return
	}
	picture, isFlyer := "", false
	for n := act; picture == "" && n != nil; n = model.byID[n.Parent] {
		picture, isFlyer = n.Flyer, true
		if picture == "" {
			picture, isFlyer = n.Image, false
		}
	}
	imageBytes := a.readImage(picture)
	line, under := when(timed(model, act)), lineage(model, act)
	sum := sha256.Sum256([]byte(act.Title + "\x00" + under + "\x00" + line + "\x00" + picture + "\x00" + cardStyle.TaglineText()))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	day, hours := whenLines(timed(model, act))
	card, err := cardStyle.Draw(sharecard.Card{
		Kicker: under, Title: act.Title, Picture: imageBytes, Whole: isFlyer,
		Lines: []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(card)
}

func (a app) readImage(key string) []byte {
	if key == "" {
		return nil
	}
	if strings.HasPrefix(key, "activity-images/") {
		if a.media == nil {
			return nil
		}
		data, _, ok := a.media.Bytes(key)
		if !ok {
			return nil
		}
		return data
	}
	for _, dir := range []string{"web/team", "web/public/team"} {
		if data, err := os.ReadFile(path.Join(dir, key)); err == nil {
			return data
		}
	}
	return nil
}

func whenLines(a *Activity) (string, string) {
	if a.Timing != "" {
		return a.Timing, ""
	}
	if days, hours := spanLines(a); days != "" {
		return days, hours
	}
	start, err := time.ParseInLocation(DateTimeFormat, a.Start, local)
	if err != nil {
		if day, err := time.ParseInLocation(DateFormat, a.Start, local); err == nil {
			return day.Format("Monday, January 2"), ""
		}
		return "", ""
	}
	day := start.Format("Monday, January 2")
	end, err := time.ParseInLocation(DateTimeFormat, a.End, local)
	if err != nil {
		return day, start.Format("3:04 PM")
	}
	return day, sharecard.Hours(start, end)
}

func spanLines(a *Activity) (string, string) {
	start, err := ParseWhen(a.Start)
	if err != nil {
		return "", ""
	}
	end, err := ParseWhen(a.End)
	if err != nil || (end.Year() == start.Year() && end.YearDay() == start.YearDay()) {
		return "", ""
	}
	days := start.Format("Monday, January 2") + " – " + end.Format("Monday, January 2")
	if len(a.Start) > len(DateFormat) && len(a.End) > len(DateFormat) {
		return days, start.Format("Mon 3:04 PM") + " – " + end.Format("Mon 3:04 PM")
	}
	if len(a.Start) > len(DateFormat) {
		return days, start.Format("Mon 3:04 PM")
	}
	return days, ""
}
