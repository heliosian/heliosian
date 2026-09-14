package calendar

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/imagesearch"
	"heliosian/internal/serve"
)

const shell = "web/calendar/index.html"

var pages = []string{"/{$}", "/day/{date}", "/events/{id...}", "/feeds", "/admin"}

type app struct {
	cache       *Cache
	writer      data.Writer
	queue       Enqueuer
	store       *blob.Store
	directory   Directory
	superAdmins func() []string
	linked      func(email string) []Linked
	search      ImageSearch
	// mail sends the invites a yes brings and takes in the replies.
	mail Mail
}

// ImageSearch is the picture search the other apps' editors share.
type ImageSearch = imagesearch.Search

// imageFolder is where the category images an admin uploads go, content
// addressed; maxImageSize bounds one upload.
const (
	imageFolder  = "category-images"
	maxImageSize = 8 << 20
)

func Register(mux *http.ServeMux, cache *Cache, writer data.Writer, queue Enqueuer, store *blob.Store, directory Directory, superAdmins func() []string, linked func(email string) []Linked, search ImageSearch, mailbox Mail) Answerer {
	if search.UserAgent == "" {
		search.UserAgent = "Helios Calendar image search (+https://when.heliosian.com)"
	}
	a := app{cache: cache, writer: writer, queue: queue, store: store, directory: directory, superAdmins: superAdmins, linked: linked, search: search, mail: mailbox}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/calendar/model", a.model)
	mux.HandleFunc("POST /api/calendar/feeds", a.addFeed)
	mux.HandleFunc("DELETE /api/calendar/feeds", a.removeFeed)
	mux.HandleFunc("POST /api/calendar/rsvp", a.rsvp)
	mux.HandleFunc("POST /api/calendar/settings", a.saveSetting)
	mux.HandleFunc("POST /api/calendar/keywords", a.admin(a.setKeywords))
	mux.HandleFunc("POST /api/calendar/events", a.admin(a.addEvents))
	mux.HandleFunc("POST /api/calendar/events/when", a.admin(a.moveEvent))
	mux.HandleFunc("DELETE /api/calendar/settings", a.forgetSetting)
	mux.HandleFunc("POST /api/calendar/tags", a.setTags)
	mux.HandleFunc("POST /api/calendar/image", a.uploadImage)
	mux.HandleFunc("GET /api/calendar/images/search", a.admin(a.search.ServeSearch))
	mux.HandleFunc("POST /api/calendar/images/import", a.admin(a.importImage))
	mux.HandleFunc("GET /feed/{file}", a.feed)
	// Public, past sign-in (auth.Public): the cards a chat app fetches.
	mux.HandleFunc("GET /share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /share/{id...}", a.shareCard)
	// Public too: the mail provider's call for each reply to an invite,
	// signed with the webhook secret.
	mux.HandleFunc("POST /api/calendar/replies", a.replies)
	return a.answer
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) who(r *http.Request) (string, bool) {
	email := a.directory.Resolve(strings.ToLower(auth.Email(r)))
	return email, a.cache.IsAdmin(email)
}

var now = func() time.Time {
	return time.Now().In(Location)
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	view := Render(a.cache.Model(), a.directory, email, admin, now(), a.linked(email))
	if admin {
		view.ImageSources = a.search.Sources()
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode calendar model", "error", err)
	}
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(into); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

// commit rebuilds the model over the proposed tables first, so a change the
// sheet rules reject never reaches the sheet, then applies it in memory and
// queues the write behind every earlier one.
func (a app) commit(ctx context.Context, w http.ResponseWriter, tables *Tables, flush func() error) bool {
	model, err := BuildModel(tables, a.cache.roster())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	a.cache.set(tables, model)
	a.queue.Add(func() {
		if err := flush(); err != nil {
			slog.ErrorContext(ctx, "calendar write", "error", err)
		}
	})
	return true
}

func (a app) logChange(actor, action, tab, key, column, from, to string) error {
	return a.writer.AppendCells(appName, ChangeLogTab, map[string]string{
		"Timestamp": now().Format(DateTimeFormat), "Actor": actor, "Action": action, "Tab": tab, "Key": key, "Column": column, "From": from, "To": to,
	})
}

func NewToken() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out)
}

func feedURL(r *http.Request, token string) string {
	return "https://" + r.Host + "/feed/" + token + ".ics"
}

func (a app) addFeed(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		Name       string   `json:"name"`
		Classrooms []string `json:"classrooms"`
		Tags       []string `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	token := NewToken()
	cells := map[string]string{
		"Token": token, "Email": actor, "Name": strings.TrimSpace(body.Name),
		"Classrooms": JoinList(SplitList(JoinList(body.Classrooms))), "Tags": JoinList(SplitList(JoinList(body.Tags))), "Created": now().Format(DateTimeFormat),
	}
	if !a.commit(r.Context(), w, a.cache.Tables().WithFeed(cells), func() error {
		if err := a.writer.AppendCells(appName, FeedsTab, cells); err != nil {
			return err
		}
		return a.logChange(actor, "added", FeedsTab, token, "Name", "", cells["Name"])
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: feed added", "actor", actor, "name", cells["Name"], "classrooms", cells["Classrooms"], "tags", cells["Tags"])
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token, "url": feedURL(r, token)})
}

func (a app) removeFeed(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	f := a.cache.Model().Feed(strings.TrimSpace(body.Token))
	if f == nil {
		http.Error(w, "no such feed", http.StatusNotFound)
		return
	}
	if f.Email != actor && !admin {
		http.Error(w, "only the person who made a feed, or an admin, can remove it", http.StatusForbidden)
		return
	}
	token, name := f.Token, f.Name
	if !a.commit(r.Context(), w, a.cache.Tables().WithoutFeed(token), func() error {
		if err := a.writer.Delete(appName, FeedsTab, map[string]string{"Token": token}); err != nil {
			return err
		}
		return a.logChange(actor, "removed", FeedsTab, token, "Name", name, "")
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: feed removed", "actor", actor, "name", name)
	w.WriteHeader(http.StatusNoContent)
}

func yesNoWord(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// admin wraps a handler for the calendar admins alone.
func (a app) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, admin := a.who(r); !admin {
			http.Error(w, "only a calendar admin can do that", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// importImage stores a picture picked from the search the way an upload is
// stored, under the calendar's own folder.
func (a app) importImage(w http.ResponseWriter, r *http.Request) {
	a.search.ServeImport(w, r, a.store, imageFolder, maxImageSize)
}

// setKeywords is an admin replacing an event's search words from its page:
// the Keywords cell of its Overrides row, which stands over whatever the
// import and the classifier gave, and stays through every later run. An
// empty list is written as the clearing mark, since a blank cell keeps.
func (a app) setKeywords(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		ID       string   `json:"id"`
		Keywords []string `json:"keywords"`
	}
	if !decode(w, r, &body) {
		return
	}
	e := a.cache.Model().Event(body.ID)
	if e == nil {
		http.Error(w, "that event is not in the sheet", http.StatusNotFound)
		return
	}
	words := SplitList(JoinList(body.Keywords))
	cell := Clear
	if len(words) > 0 {
		cell = JoinList(words)
	}
	was := JoinList(e.Keywords)
	if !a.commit(r.Context(), w, a.cache.Tables().WithOverride(e.ID, map[string]string{"Keywords": cell}), func() error {
		if err := a.writer.Set(appName, OverridesTab, map[string]string{"Event ID": e.ID}, map[string]string{"Keywords": cell}); err != nil {
			return err
		}
		return a.logChange(actor, "changed", OverridesTab, e.ID, "Keywords", was, cell)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: keywords set", "actor", actor, "event", e.ID, "keywords", cell)
	w.WriteHeader(http.StatusNoContent)
}

// newEventID mints an Events tab id the way the volunteer portal does:
// eight characters from a 32-symbol alphabet.
func newEventID() string {
	const alphabet = "ABCDEFGHJKMNPQRSTVWXYZ0123456789"
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out)
}

// shiftWhen moves a sheet start or end - a date, or a date with a time -
// by whole weeks.
func shiftWhen(when string, weeks int) string {
	if when == "" {
		return ""
	}
	layout := DateFormat
	if len(when) > len(DateFormat) {
		layout = DateTimeFormat
	}
	t, err := time.ParseInLocation(layout, when, Location)
	if err != nil {
		return when
	}
	return t.AddDate(0, 0, 7*weeks).Format(layout)
}

// addEvents is an admin adding an event to the Events tab from Admin Tools
// - and, asked to repeat it, the same event again every so many weeks, as
// many more times as asked, each its own row. The rows go through the
// sheet's rules first, so a bad date or an unknown tag is refused before
// anything is written.
func (a app) addEvents(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		Title       string   `json:"title"`
		Start       string   `json:"start"`
		End         string   `json:"end"`
		Location    string   `json:"location"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		DayType     string   `json:"dayType"`
		Keywords    []string `json:"keywords"`
		Source      string   `json:"source"`
		RepeatWeeks int      `json:"repeatWeeks"`
		RepeatTimes int      `json:"repeatTimes"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.RepeatTimes < 0 || body.RepeatTimes > 52 || body.RepeatWeeks < 1 && body.RepeatTimes > 0 {
		http.Error(w, "repeat up to 52 more times, some whole number of weeks apart", http.StatusBadRequest)
		return
	}
	stamp := now().Format(DateFormat)
	rows := []map[string]string{}
	for i := 0; i <= body.RepeatTimes; i++ {
		rows = append(rows, map[string]string{
			"Event ID": newEventID(), "Start": shiftWhen(strings.TrimSpace(body.Start), i*body.RepeatWeeks), "End": shiftWhen(strings.TrimSpace(body.End), i*body.RepeatWeeks),
			"Title": strings.TrimSpace(body.Title), "Location": strings.TrimSpace(body.Location), "Description": strings.TrimSpace(body.Description),
			"Tags": JoinList(SplitList(JoinList(body.Tags))), "Day Type": strings.TrimSpace(body.DayType), "Keywords": JoinList(SplitList(JoinList(body.Keywords))),
			"Added By": actor, "Added": stamp, "Source": strings.TrimSpace(body.Source),
		})
	}
	if !a.commit(r.Context(), w, a.cache.Tables().WithEvents(rows), func() error {
		for _, row := range rows {
			if err := a.writer.AppendCells(appName, EventsTab, row); err != nil {
				return err
			}
			if err := a.logChange(actor, "added", EventsTab, row["Event ID"], "Title", "", row["Title"]); err != nil {
				return err
			}
		}
		return nil
	}) {
		return
	}
	ids := []string{}
	for _, row := range rows {
		ids = append(ids, row["Event ID"])
	}
	slog.InfoContext(r.Context(), "calendar: events added", "actor", actor, "title", rows[0]["Title"], "count", len(rows))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ids": ids})
}

// moveEvent is an admin changing when a hand-added event is, from the
// Events list in Admin Tools: its Events tab row's Start and End. Only an
// event of that tab moves this way; an imported one is corrected in
// Overrides.
func (a app) moveEvent(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		ID    string `json:"id"`
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if !decode(w, r, &body) {
		return
	}
	e := a.cache.Model().Event(body.ID)
	if e == nil || e.Source != SourceSheet {
		http.Error(w, "only an event of the Events tab moves from here", http.StatusBadRequest)
		return
	}
	start, end := strings.TrimSpace(body.Start), strings.TrimSpace(body.End)
	if end == "" {
		end = start
	}
	was := e.Start + " – " + e.End
	if !a.commit(r.Context(), w, a.cache.Tables().WithEventWhen(e.ID, start, end), func() error {
		if err := a.writer.Upsert(appName, EventsTab, "Event ID", e.ID, map[string]string{"Start": start, "End": end}); err != nil {
			return err
		}
		return a.logChange(actor, "moved", EventsTab, e.ID, "Start", was, start+" – "+end)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: event moved", "actor", actor, "event", e.ID, "start", start, "end", end)
	w.WriteHeader(http.StatusNoContent)
}

// saveSetting keeps the viewer's filters as their own default: the row
// under their address in the Settings tab, which the calendar opens to
// for them on every device and Heliosian reads their Upcoming Events under.
func (a app) saveSetting(w http.ResponseWriter, r *http.Request) {
	email, _ := a.who(r)
	var body struct {
		Classrooms []string `json:"classrooms"`
		Tags       []string `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	cells := map[string]string{
		"Email": email, "Classrooms": JoinList(SplitList(JoinList(body.Classrooms))), "Categories": JoinList(SplitList(JoinList(body.Tags))), "Saved": now().Format(DateTimeFormat),
	}
	if !a.commit(r.Context(), w, a.cache.Tables().WithSetting(email, cells), func() error {
		if err := a.writer.Upsert(appName, SettingsTab, "Email", email, cells); err != nil {
			return err
		}
		return a.logChange(email, "saved", SettingsTab, email, "Categories", "", cells["Categories"])
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: view saved", "actor", email, "classrooms", cells["Classrooms"], "tags", cells["Categories"])
	w.WriteHeader(http.StatusNoContent)
}

// forgetSetting drops the viewer's saved view, so the calendar's own
// defaults are theirs again.
func (a app) forgetSetting(w http.ResponseWriter, r *http.Request) {
	email, _ := a.who(r)
	if _, ok := a.cache.Model().Settings[email]; !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.commit(r.Context(), w, a.cache.Tables().WithoutSetting(email), func() error {
		if err := a.writer.Delete(appName, SettingsTab, map[string]string{"Email": email}); err != nil {
			return err
		}
		return a.logChange(email, "forgot", SettingsTab, email, "Categories", "", "")
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: view forgotten", "actor", email)
	w.WriteHeader(http.StatusNoContent)
}

// uploadImage stores a category image an admin picked, content addressed,
// and answers with the name the Tags tab records; the save that follows
// references it. Sample mode has no bucket to put it in.
func (a app) uploadImage(w http.ResponseWriter, r *http.Request) {
	if _, admin := a.who(r); !admin {
		http.Error(w, "only a calendar admin can add an image", http.StatusForbidden)
		return
	}
	if a.store == nil {
		http.Error(w, "image uploads require real-data mode", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImageSize)
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "an image file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read the image", http.StatusBadRequest)
		return
	}
	mimeType := http.DetectContentType(content)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[mimeType]
	if ext == "" {
		http.Error(w, fmt.Sprintf("%s is not a supported image", header.Filename), http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:]) + ext
	if err := a.store.Put(imageFolder, name, mimeType, content); err != nil {
		slog.ErrorContext(r.Context(), "calendar: store image", "error", err)
		http.Error(w, "could not store the image", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"name": imageFolder + "/" + name, "url": "/" + imageFolder + "/" + name})
}

// setTags is Admin Tools saving the categories: the sheet's tags in the
// order they should have, each with its description and group, and any new
// ones at their place. Every tag the sheet has must be there - nothing is
// dropped from here, since events carry tags by name - and a built-in is
// refused, having no row. A changed description or group is written to its
// row, a new tag appended, and then the rows put in the order given, each
// change logged.
func (a app) setTags(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	if !admin {
		http.Error(w, "only a calendar admin can change the categories", http.StatusForbidden)
		return
	}
	var body struct {
		Tags []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Group       string `json:"group"`
			Default     bool   `json:"default"`
			Image       string `json:"image"`
		} `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	tables := a.cache.Tables()
	current := map[string]map[string]string{}
	for _, row := range tables.Tags {
		current[row["Tag"]] = row
	}
	rows := []map[string]string{}
	seen := map[string]bool{}
	changed := map[string]map[string]string{}
	added := [][]string{}
	logs := [][]string{}
	for _, t := range body.Tags {
		name, description, group, image := strings.TrimSpace(t.Name), strings.TrimSpace(t.Description), strings.TrimSpace(t.Group), strings.TrimSpace(t.Image)
		if name == "" || seen[name] {
			http.Error(w, "every category needs a name of its own", http.StatusBadRequest)
			return
		}
		if description == "" {
			http.Error(w, fmt.Sprintf("%q needs a description", name), http.StatusBadRequest)
			return
		}
		seen[name] = true
		row, ok := current[name]
		if !ok {
			for _, b := range builtinTags {
				if b.Name == name {
					http.Error(w, fmt.Sprintf("%q is built in and has no row of its own", name), http.StatusBadRequest)
					return
				}
			}
			row = map[string]string{"Tag": name, "Description": description, "Group": group, "Default": yesNoWord(t.Default), "Image": image}
			added = append(added, []string{name})
			logs = append(logs, []string{"added", name, "Tag", "", name})
			rows = append(rows, row)
			continue
		}
		row = maps.Clone(row)
		cells := map[string]string{}
		if row["Description"] != description {
			logs = append(logs, []string{"changed", name, "Description", row["Description"], description})
			cells["Description"] = description
		}
		if row["Group"] != group {
			logs = append(logs, []string{"changed", name, "Group", row["Group"], group})
			cells["Group"] = group
		}
		if tagDefault(row["Default"]) != t.Default {
			logs = append(logs, []string{"changed", name, "Default", row["Default"], yesNoWord(t.Default)})
			cells["Default"] = yesNoWord(t.Default)
		}
		if strings.TrimSpace(row["Image"]) != image {
			logs = append(logs, []string{"changed", name, "Image", row["Image"], image})
			cells["Image"] = image
		}
		if len(cells) > 0 {
			changed[name] = cells
			maps.Copy(row, cells)
		}
		rows = append(rows, row)
	}
	for name := range current {
		if !seen[name] {
			http.Error(w, fmt.Sprintf("%q is missing - a category cannot be removed from here", name), http.StatusBadRequest)
			return
		}
	}
	order := make([]string, 0, len(rows))
	moved := false
	for i, row := range rows {
		order = append(order, row["Tag"])
		if i < len(tables.Tags) && tables.Tags[i]["Tag"] != row["Tag"] {
			moved = true
		}
	}
	if len(changed) == 0 && len(added) == 0 && !moved {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if moved {
		logs = append(logs, []string{"reordered", "", "Tag", "", strings.Join(order, ", ")})
	}
	newRows := map[string]map[string]string{}
	for _, row := range rows {
		newRows[row["Tag"]] = row
	}
	if !a.commit(r.Context(), w, tables.WithTags(rows), func() error {
		if len(changed) > 0 {
			if err := a.writer.SetMany(appName, TagsTab, "Tag", changed); err != nil {
				return err
			}
		}
		for _, add := range added {
			if err := a.writer.AppendCells(appName, TagsTab, newRows[add[0]]); err != nil {
				return err
			}
		}
		if moved || len(added) > 0 {
			if err := a.writer.Reorder(appName, TagsTab, "Tag", order); err != nil {
				return err
			}
		}
		for _, l := range logs {
			if err := a.logChange(actor, l[0], TagsTab, l[1], l[2], l[3], l[4]); err != nil {
				return err
			}
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: categories saved", "actor", actor, "changed", len(changed), "added", len(added), "reordered", moved)
	w.WriteHeader(http.StatusNoContent)
}

// Public, past sign-in (auth.Public): the token is the whole secret.
func (a app) feed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("file"), ".ics")
	model := a.cache.Model()
	f := model.Feed(token)
	if f == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", "helios-calendar.ics"))
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(ICS(model, f, a.linked(f.Email), "https://"+r.Host, now()))
}
