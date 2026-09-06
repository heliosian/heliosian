package directory

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"

	"heliosian/internal/auth"
)

type admin struct {
	cache *Cache
}

// RegisterAdmin wires up the admin tools: image management and the admin list itself.
// Routes are always registered — even in sample mode — so the page and settings are
// reachable for testing; PutImage is the only operation that actually needs a store.
func RegisterAdmin(mux *http.ServeMux, cache *Cache) {
	a := admin{cache: cache}
	mux.HandleFunc("GET /admin", a.page)
	mux.HandleFunc("GET /api/admin/state", a.state)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/admin/stale-years", a.setStaleYears)
	mux.HandleFunc("POST /api/admin/privacy-links", a.setPrivacyLinks)
	mux.HandleFunc("POST /api/admin/images", a.setImage)
	mux.HandleFunc("POST /api/admin/super-edit", a.setSuperEdit)
	mux.HandleFunc("POST /api/admin/super-admins", a.setSuperAdmins)
	mux.HandleFunc("POST /api/admin/spoof", a.setSpoof)
}

// requireAdmin reports whether the effective identity — the spoof target, if the real
// admin has picked one to view as, otherwise the real admin themselves — is an admin
// (either tier), writing the appropriate error response itself when not. Spoofing is
// full parity, not a read-only preview: every admin handler but setSpoof runs as the
// effective identity, so it does exactly what that person could or couldn't do.
func (a admin) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := effectiveEmail(a.cache, r)
	if !a.cache.IsAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

// requireSuperAdmin gates the handful of things a regular admin never even learns
// exist: managing who else is a super admin. Also effective-identity based, so
// spoofing as a super admin grants it and spoofing as a regular admin doesn't — a
// regular admin's 403 here is identical to a non-admin's, revealing nothing about a
// tier above them.
func (a admin) requireSuperAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := effectiveEmail(a.cache, r)
	if !a.cache.IsSuperAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

// requireRealSuperAdmin is the one thing in this file keyed on the real identity
// rather than the effective one: starting or stopping a spoof has to remain reachable
// by the real admin regardless of who they're currently viewing as, or the on-page
// banner's "stop" would be locked out by the very spoof it's meant to end.
func (a admin) requireRealSuperAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := strings.ToLower(auth.Email(r))
	if !a.cache.IsSuperAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

func (a admin) page(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	http.ServeFile(w, r, "web/admin/index.html")
}

type imageInfo struct {
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl,omitempty"`
}

type personOption struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (a admin) state(w http.ResponseWriter, r *http.Request) {
	real := strings.ToLower(auth.Email(r))
	email := effectiveEmail(a.cache, r)
	if !a.cache.IsAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}
	model := a.cache.Model()
	classrooms := make([]imageInfo, 0, len(model.Classrooms))
	for _, c := range model.Classrooms {
		classrooms = append(classrooms, imageInfo{Name: c.Name, ImageURL: c.ImageURL})
	}
	grades := make([]imageInfo, 0, len(model.Grades))
	for _, g := range model.Grades {
		grades = append(grades, imageInfo{Name: g.Name, ImageURL: g.ImageURL})
	}
	people := make([]personOption, 0, len(model.People))
	for _, p := range model.People {
		people = append(people, personOption{Name: p.FullName, Email: p.Email})
	}
	sort.Slice(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	settings := a.cache.Settings()
	view := struct {
		Email        string         `json:"email"`
		HasStore     bool           `json:"hasStore"`
		Admins       []string       `json:"admins"`
		StaleYears   StaleYears     `json:"staleYears"`
		PrivacyLinks PrivacyLinks   `json:"privacyLinks"`
		SuperEdit    bool           `json:"superEdit"`
		Classrooms   []imageInfo    `json:"classrooms"`
		Grades       []imageInfo    `json:"grades"`
		People       []personOption `json:"people"`
		IsSuperAdmin bool           `json:"isSuperAdmin"`
		SuperAdmins  []string       `json:"superAdmins,omitempty"`
		SpoofingAs   string         `json:"spoofingAs,omitempty"`
	}{
		Email: email, HasStore: a.cache.HasStore(), Admins: mergedAdmins(settings), StaleYears: settings.StaleYears,
		PrivacyLinks: settings.PrivacyLinks,
		SuperEdit:    a.cache.SuperEditEnabled(email),
		Classrooms:   classrooms, Grades: grades, People: people,
	}
	// A regular admin's response stops here — nothing below this line is reachable
	// unless the effective identity is a super admin, so a regular admin's client
	// never sees a super-admin field, let alone renders it.
	if a.cache.IsSuperAdmin(email) {
		view.IsSuperAdmin = true
		view.SuperAdmins = settings.SuperAdmins
	}
	// The way back is keyed on the real identity and shown regardless of the
	// simulated tier — otherwise spoofing as a regular admin would strand the real
	// super admin on a page with no Spoof Mode tab and no way out.
	view.SpoofingAs = spoofDisplayName(a.cache, real)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		log.Printf("[ERROR] encode admin state: %v", err)
	}
}

func (a admin) setAdmins(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Admins []string `json:"admins"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	settings := a.cache.Settings()
	// The Admins tab shows every admin merged together, super admins included, so a
	// regular admin can't tell the two tiers apart. Submitting that merged list back
	// must not be able to write a super admin into the regular list (or drop them from
	// super admin by omission) — a super admin's presence here is display-only and
	// never round-trips into settings.Admins. This is also why an empty result isn't
	// rejected the way it is for super admins: losing every regular admin can't lock
	// the tools, since the super admin list alone already guarantees access.
	admins := normalizeEmails(body.Admins)
	admins = withoutSuperAdmins(admins, settings.SuperAdmins)
	settings.Admins = admins
	if err := a.cache.UpdateSettings(settings); err != nil {
		serverError(w, err)
		return
	}
	log.Printf("admin: %s set the admin list to %s", email, strings.Join(admins, ", "))
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) setStaleYears(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var years StaleYears
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&years); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if years.Photo <= 0 || years.Facts <= 0 || years.FamilyPhoto <= 0 {
		http.Error(w, "thresholds must be positive numbers of years", http.StatusBadRequest)
		return
	}
	settings := a.cache.Settings()
	settings.StaleYears = years
	if err := a.cache.UpdateSettings(settings); err != nil {
		serverError(w, err)
		return
	}
	log.Printf("admin: %s set stale-years thresholds to %+v", email, years)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) setPrivacyLinks(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var links PrivacyLinks
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&links); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	links.VeracrossPreferences = strings.TrimSpace(links.VeracrossPreferences)
	links.HeliosWhoOptIn = strings.TrimSpace(links.HeliosWhoOptIn)
	if !strings.HasPrefix(links.VeracrossPreferences, "https://") || !strings.HasPrefix(links.HeliosWhoOptIn, "https://") {
		http.Error(w, "both links must be full https:// URLs", http.StatusBadRequest)
		return
	}
	settings := a.cache.Settings()
	settings.PrivacyLinks = links
	if err := a.cache.UpdateSettings(settings); err != nil {
		serverError(w, err)
		return
	}
	log.Printf("admin: %s set privacy links to %+v", email, links)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) setSuperEdit(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	a.cache.SetSuperEdit(email, body.Enabled)
	log.Printf("admin: %s set super edit mode to %t", email, body.Enabled)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) setSuperAdmins(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		SuperAdmins []string `json:"superAdmins"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	admins := normalizeEmails(body.SuperAdmins)
	if len(admins) == 0 {
		http.Error(w, "the super admin list cannot be empty", http.StatusBadRequest)
		return
	}
	settings := a.cache.Settings()
	settings.SuperAdmins = admins
	if err := a.cache.UpdateSettings(settings); err != nil {
		serverError(w, err)
		return
	}
	log.Printf("admin: %s set the super admin list to %s", email, strings.Join(admins, ", "))
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) setSpoof(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireRealSuperAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Email))
	if target != "" && a.cache.Model().Person(target) == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	a.cache.SetSpoof(email, target)
	if target == "" {
		log.Printf("admin: %s stopped spoofing", email)
	} else {
		log.Printf("admin: %s started spoofing as %s", email, target)
	}
	w.WriteHeader(http.StatusNoContent)
}

var imageExtensions = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

func (a admin) setImage(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if !a.cache.HasStore() {
		http.Error(w, "image uploads require real-data mode", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "upload too large or malformed", http.StatusBadRequest)
		return
	}
	kind := r.FormValue("kind")
	name := strings.TrimSpace(r.FormValue("name"))
	var folder, slug string
	switch kind {
	case "classroom":
		folder = "classroom-images"
		slug = strings.ToLower(name)
	case "grade":
		folder = "grade-images"
		slug = gradeSlug(name)
	default:
		http.Error(w, "bad kind: must be classroom or grade", http.StatusBadRequest)
		return
	}
	if slug == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		http.Error(w, "unreadable file", http.StatusBadRequest)
		return
	}
	sniffed := http.DetectContentType(content)
	ext, ok := imageExtensions[sniffed]
	if !ok {
		http.Error(w, "unsupported image type "+sniffed, http.StatusBadRequest)
		return
	}

	if err := a.cache.PutImage(folder, slug+"."+ext, sniffed, content); err != nil {
		serverError(w, err)
		return
	}
	log.Printf("admin: %s replaced the %s image for %s", email, kind, name)
	w.WriteHeader(http.StatusNoContent)
}
