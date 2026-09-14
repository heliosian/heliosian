// Package theme is the colouring an admin gives an app's page: the rail
// down the left and the page behind everything else, each one colour or a
// gradient running down from it to a second, the colour of the rail's
// words, and two pictures: the rail's logo and the art at its foot. Every app keeps the four
// values as Key/Value rows in its own sheet under the same keys, edits
// them from the Appearance panel of its admin tools, and hands them to
// its page, where web/common/theme.js sets the stylesheet's variables.
package theme

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"heliosian/internal/blob"
)

// The Settings keys: a colour for each, for each the second colour of a
// gradient, and the colour of the rail's words. Each is #rrggbb or blank
// for the stylesheet's own.
const (
	SidebarKey     = "Sidebar Color"
	SidebarEndKey  = "Sidebar Color 2"
	SidebarTextKey = "Sidebar Text Color"
	PageKey        = "Page Color"
	PageEndKey     = "Page Color 2"
	// LogoKey and SidebarImageKey name pictures rather than colours: the
	// rail's logo and the art at its foot, each an uploaded object's name
	// under logos/ (Upload), or blank for the app's own.
	LogoKey         = "Logo"
	SidebarImageKey = "Sidebar Image"
)

var Keys = []string{SidebarKey, SidebarEndKey, SidebarTextKey, PageKey, PageEndKey, LogoKey, SidebarImageKey}

var colorKeys = []string{SidebarKey, SidebarEndKey, SidebarTextKey, PageKey, PageEndKey}

var pictureKeys = []string{LogoKey, SidebarImageKey}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// A picture's name: content addressed under logos/, as Upload stores it.
var pictureName = regexp.MustCompile(`^logos/[0-9a-f]{64}\.(png|jpg|gif|webp)$`)

// IsKey says whether a Settings row is one of the theme's, for a loader
// that reads the rest of the tab itself.
func IsKey(key string) bool {
	return slices.Contains(Keys, key)
}

// Theme is the colouring: the rail and the page, each a colour and, for a
// gradient, the colour it runs down to; blank for the stylesheet's own.
type Theme struct {
	Sidebar     string `json:"sidebar,omitempty"`
	SidebarEnd  string `json:"sidebarEnd,omitempty"`
	SidebarText string `json:"sidebarText,omitempty"`
	Page        string `json:"page,omitempty"`
	PageEnd     string `json:"pageEnd,omitempty"`
	// Logo and SidebarImage are the names of uploaded pictures, served at
	// /logos/...: the rail's logo, and the art at the rail's foot.
	Logo         string `json:"logo,omitempty"`
	SidebarImage string `json:"sidebarImage,omitempty"`
}

// Values are the theme as Settings rows, by key.
func (t Theme) Values() map[string]string {
	return map[string]string{
		SidebarKey: t.Sidebar, SidebarEndKey: t.SidebarEnd, SidebarTextKey: t.SidebarText, PageKey: t.Page, PageEndKey: t.PageEnd,
		LogoKey: t.Logo, SidebarImageKey: t.SidebarImage,
	}
}

// Of reads a theme from its values by key: each colour blank or #rrggbb,
// lowered, a second colour only beside a first, and each picture blank or
// a name Upload gave.
func Of(values map[string]string) (Theme, error) {
	clean := map[string]string{}
	for _, key := range colorKeys {
		value := strings.TrimSpace(values[key])
		if value != "" && !hexColor.MatchString(value) {
			return Theme{}, fmt.Errorf("%s is %q, not a #rrggbb colour", key, value)
		}
		clean[key] = strings.ToLower(value)
	}
	for _, pair := range [][2]string{{SidebarEndKey, SidebarKey}, {PageEndKey, PageKey}} {
		if clean[pair[0]] != "" && clean[pair[1]] == "" {
			return Theme{}, fmt.Errorf("%s needs %s beside it", pair[0], pair[1])
		}
	}
	for _, key := range pictureKeys {
		value := strings.TrimSpace(values[key])
		if value != "" && !pictureName.MatchString(value) {
			return Theme{}, fmt.Errorf("%s is %q, not an uploaded picture", key, value)
		}
		clean[key] = value
	}
	return Theme{
		Sidebar: clean[SidebarKey], SidebarEnd: clean[SidebarEndKey], SidebarText: clean[SidebarTextKey], Page: clean[PageKey], PageEnd: clean[PageEndKey],
		Logo: clean[LogoKey], SidebarImage: clean[SidebarImageKey],
	}, nil
}

// FromRows reads the theme out of a Key/Value tab, passing over rows that
// are not its own; a second row for one of its keys refuses it.
func FromRows(rows []map[string]string) (Theme, error) {
	values := map[string]string{}
	for _, row := range rows {
		key := strings.TrimSpace(row["Key"])
		if !IsKey(key) {
			continue
		}
		if _, dup := values[key]; dup {
			return Theme{}, fmt.Errorf("two rows for %q", key)
		}
		values[key] = row["Value"]
	}
	return Of(values)
}

// maxPictureSize bounds an uploaded logo or rail picture.
const maxPictureSize = 8 << 20

// Upload is the handler that takes a picture for the theme - a logo or the
// rail's art - as a multipart "image" field, stores it content addressed
// under logos/ in the blob store, and answers {"name": "logos/<hash>.<ext>"}
// for the admin page to put in the theme. gate admits an admin, answering
// the request itself when it does not; store is nil in sample mode.
func Upload(store *blob.Store, gate func(w http.ResponseWriter, r *http.Request) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !gate(w, r) {
			return
		}
		if store == nil {
			http.Error(w, "picture uploads require real-data mode", http.StatusBadRequest)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxPictureSize)
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
		name := "logos/" + hex.EncodeToString(sum[:]) + ext
		if err := store.Put("logos", hex.EncodeToString(sum[:])+ext, mimeType, content); err != nil {
			slog.ErrorContext(r.Context(), "store theme picture", "error", err)
			http.Error(w, "could not store the image", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"name": name}); err != nil {
			slog.ErrorContext(r.Context(), "encode picture name", "error", err)
		}
	}
}
