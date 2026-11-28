package db

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

//go:embed themebase.json
var themeBaseJSON []byte

var themeBase = func() map[string]any {
	d := map[string]any{}
	if err := json.Unmarshal(themeBaseJSON, &d); err != nil {
		panic(err)
	}
	return d
}()

var (
	themeKey       = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	cssURL         = regexp.MustCompile(`url\(\s*(?:"([^"]*)"|'([^']*)'|([^"'()\s]*))\s*\)`)
	themeMailPaths = []string{"page.backdrop", "page.ink", "page.muted", "envelope.body", "envelope.side", "envelope.front", "envelope.flap", "envelope.liner", "seal.color", "seal.ink", "card.background", "card.ink", "card.accent", "card.accentInk"}
	themeTextRoles = []string{"kicker", "title", "day", "hours", "place", "message", "hosts", "rule"}
)

const themePictureRoute = "/open/theme-picture/"

type themeText struct {
	Font       string `json:"font"`
	Size       string `json:"size"`
	Weight     string `json:"weight"`
	Style      string `json:"style"`
	Case       string `json:"case"`
	Spacing    string `json:"spacing"`
	LineHeight string `json:"lineHeight"`
	Color      string `json:"color"`
}

type gatherDesign struct {
	Fonts struct {
		Display string   `json:"display"`
		Body    string   `json:"body"`
		Load    []string `json:"load"`
	} `json:"fonts"`
	Page struct {
		Backdrop string `json:"backdrop"`
		Ink      string `json:"ink"`
		Muted    string `json:"muted"`
	} `json:"page"`
	Sidebar struct {
		Background string `json:"background"`
		Ink        string `json:"ink"`
		Active     string `json:"active"`
	} `json:"sidebar"`
	Envelope struct {
		Body   string `json:"body"`
		Side   string `json:"side"`
		Front  string `json:"front"`
		Flap   string `json:"flap"`
		Liner  string `json:"liner"`
		Radius string `json:"radius"`
	} `json:"envelope"`
	Seal struct {
		Color   string `json:"color"`
		Ink     string `json:"ink"`
		Shape   string `json:"shape"`
		Text    string `json:"text"`
		Picture string `json:"picture"`
	} `json:"seal"`
	Card struct {
		Background string `json:"background"`
		Ink        string `json:"ink"`
		Accent     string `json:"accent"`
		AccentInk  string `json:"accentInk"`
		Radius     string `json:"radius"`
		Padding    string `json:"padding"`
		Align      string `json:"align"`
		Width      string `json:"width"`
		MinHeight  string `json:"minHeight"`
		Border     string `json:"border"`
		Shadow     string `json:"shadow"`
		Art        string `json:"art"`
		ArtMask    string `json:"artMask"`
		ArtOpacity string `json:"artOpacity"`
		Frame      struct {
			Inset  string `json:"inset"`
			Border string `json:"border"`
			Shadow string `json:"shadow"`
		} `json:"frame"`
	} `json:"card"`
	Text struct {
		Kicker struct {
			Words string `json:"words"`
			themeText
		} `json:"kicker"`
		Title   themeText `json:"title"`
		Day     themeText `json:"day"`
		Hours   themeText `json:"hours"`
		Place   themeText `json:"place"`
		Message themeText `json:"message"`
		Hosts   themeText `json:"hosts"`
		Rule    struct {
			Width  string `json:"width"`
			Height string `json:"height"`
			Style  string `json:"style"`
			Color  string `json:"color"`
		} `json:"rule"`
	} `json:"text"`
}

func blankFields(v reflect.Value, at string) []string {
	out := []string{}
	switch v.Kind() {
	case reflect.String:
		if v.String() == "" {
			out = append(out, at)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if f.Anonymous {
				out = append(out, blankFields(v.Field(i), at)...)
				continue
			}
			out = append(out, blankFields(v.Field(i), at+"."+f.Tag.Get("json"))...)
		}
	}
	return out
}

func designStrings(v any, visit func(string)) {
	switch x := v.(type) {
	case string:
		visit(x)
	case map[string]any:
		for _, e := range x {
			designStrings(e, visit)
		}
	case []any:
		for _, e := range x {
			designStrings(e, visit)
		}
	}
}

func pictureNames(design map[string]any) ([]string, error) {
	names := []string{}
	var bad error
	designStrings(design, func(s string) {
		for _, m := range cssURL.FindAllStringSubmatch(s, -1) {
			target := m[1] + m[2] + m[3]
			switch {
			case strings.HasPrefix(target, "data:"):
			case target == "" || strings.ContainsAny(target, "/:\\"):
				bad = fmt.Errorf("the design names %q; a picture is a data: URL or the file name of a picture uploaded with the theme", target)
			case !slices.Contains(names, target):
				names = append(names, target)
			}
		}
	})
	return names, bad
}

func standardDesign(over map[string]any) error {
	whole, err := json.Marshal(mergedDesign(themeBase, over))
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(whole))
	dec.DisallowUnknownFields()
	var d gatherDesign
	if err := dec.Decode(&d); err != nil {
		return fmt.Errorf("the design does not keep the standard: %v", err)
	}
	if blank := blankFields(reflect.ValueOf(d), "design"); len(blank) > 0 {
		return fmt.Errorf("the design leaves blank %s; write none for a part it does without", strings.Join(blank, ", "))
	}
	if !slices.Contains([]string{"circle", "square", "stamp"}, d.Seal.Shape) {
		return fmt.Errorf("a seal is shaped circle, square or stamp, not %q", d.Seal.Shape)
	}
	if d.Seal.Picture != "none" && (!cssURL.MatchString(d.Seal.Picture) || hexColor.MatchString(d.Seal.Picture)) {
		return fmt.Errorf("seal.picture is none or picture layers drawn over seal.color, with no color of its own, not %q", d.Seal.Picture)
	}
	if !slices.Contains([]string{"center", "left"}, d.Card.Align) {
		return fmt.Errorf("a card aligns center or left, not %q", d.Card.Align)
	}
	merged := mergedDesign(themeBase, over)
	for _, p := range themeMailPaths {
		if hexColor.FindString(designValue(merged, p)) == "" {
			return fmt.Errorf("design %s has no #rrggbb color for the invitation drawn in mail", p)
		}
	}
	for _, role := range themeTextRoles {
		c := designValue(merged, "text."+role+".color")
		if c != "ink" && c != "accent" && !wholeHex.MatchString(c) {
			return fmt.Errorf("design text.%s.color is ink, accent or a #rrggbb color, not %q", role, c)
		}
	}
	_, err = pictureNames(over)
	return err
}

func storedDesign(raw string) (map[string]any, error) {
	d := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil, fmt.Errorf("design is not a JSON object: %v", err)
	}
	return d, nil
}

func themeChecked(m *Model, c Change) ([]Edit, error) {
	if c.Old != nil && c.New != nil && c.Old["key"] == "plain" && c.Old["added_by"] == "" && c.New["added_by"] != "" {
		return nil, access.Invalid("the site's plain theme is what Create Your Own starts from, so it stays the site's own")
	}
	if c.New == nil || (c.Old != nil && c.Old["key"] == c.New["key"] && c.Old["design"] == c.New["design"]) {
		return nil, nil
	}
	if !themeKey.MatchString(c.New["key"]) {
		return nil, access.Invalid("a theme's key is lower-case letters, digits and dashes, not %q", c.New["key"])
	}
	d, err := storedDesign(c.New["design"])
	if err != nil {
		return nil, access.Invalid("%v", err)
	}
	if err := standardDesign(d); err != nil {
		return nil, access.Invalid("%v", err)
	}
	return nil, nil
}

func themeInUse(m *Model, c Change) ([]Edit, error) {
	if c.Old["key"] == "plain" && c.Old["added_by"] == "" {
		return nil, access.Invalid("the site's plain theme is what Create Your Own starts from, so it stays")
	}
	copies := map[string]string{}
	edits := []Edit{}
	for _, col := range []string{"theme", "envelope", "seal", "liner"} {
		for _, g := range m.Table("GROUP").Referencing(col, c.Old["id"]) {
			maker := g["added_by"]
			alias, made := copies[maker]
			if !made {
				alias = fmt.Sprintf("copy%d", len(copies))
				copies[maker] = alias
				edits = append(edits, Edit{Insert: "THEME", As: alias, Row: map[string]any{"key": freeThemeKey(m, c.Old["key"], maker), "name": c.Old["name"], "blurb": c.Old["blurb"], "design": c.Old["design"], "added_by": maker}})
				for _, p := range m.Table("THEME_PICTURE").Referencing("theme", c.Old["id"]) {
					edits = append(edits, Edit{Insert: "THEME_PICTURE", Row: map[string]any{"theme": "@" + alias, "name": p["name"], "original": p["original"], "picture": p["picture"]}})
				}
			}
			edits = append(edits, Edit{Set: g["id"], Cells: map[string]any{col: "@" + alias}})
		}
	}
	return edits, nil
}

func freeThemeKey(m *Model, key, owner string) string {
	free := key
	for n := 1; ; n++ {
		if _, taken := m.Table("THEME").Find(free, owner); !taken {
			return free
		}
		free = key + "-copy"
		if n > 1 {
			free = fmt.Sprintf("%s-copy-%d", key, n)
		}
	}
}

func themeDesign(m *Model, id string) (map[string]any, error) {
	row, ok := m.Table("THEME").Get(id)
	if !ok {
		return nil, fmt.Errorf("no theme %s", id)
	}
	over, err := storedDesign(row["design"])
	if err != nil {
		return nil, fmt.Errorf("theme %s: %v", id, err)
	}
	d := mergedDesign(themeBase, over)
	d["id"], d["key"], d["name"], d["blurb"] = row["id"], row["key"], row["name"], row["blurb"]
	return d, nil
}

func resolvedPictures(v any, pictures map[string]string) any {
	switch x := v.(type) {
	case string:
		return cssURL.ReplaceAllStringFunc(x, func(u string) string {
			m := cssURL.FindStringSubmatch(u)
			name := m[1] + m[2] + m[3]
			if content, ok := pictures[name]; ok {
				return `url("` + themePictureRoute + content + `")`
			}
			return u
		})
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			out[k] = resolvedPictures(e, pictures)
		}
		return out
	case []any:
		out := []any{}
		for _, e := range x {
			out = append(out, resolvedPictures(e, pictures))
		}
		return out
	}
	return v
}

type shownTheme struct {
	AddedBy string         `json:"added_by"`
	Design  map[string]any `json:"design"`
}

func (p *WhenPages) themes(w http.ResponseWriter, r *http.Request) {
	m := p.s.Model()
	out := []shownTheme{}
	for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if id == "" {
			continue
		}
		d, err := themeDesign(m, id)
		if err != nil {
			serve.Error(w, r, access.Missing("no theme %s", id))
			return
		}
		pictures := map[string]string{}
		for _, pic := range m.Table("THEME_PICTURE").Referencing("theme", id) {
			pictures[pic["name"]] = pic["id"] + "?c=" + pic["original"]
		}
		row, _ := m.Table("THEME").Get(id)
		out = append(out, shownTheme{AddedBy: row["added_by"], Design: resolvedPictures(d, pictures).(map[string]any)})
	}
	w.Header().Set("Cache-Control", "no-cache")
	serve.Write(w, r, http.StatusOK, map[string]any{"themes": out})
}

func (p *WhenPages) themePicture(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row, err := p.pics.waitMade(r.Context(), madeColumn{"THEME_PICTURE", "original", "picture"}, id)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	data, mimeType, err := p.s.Model().contentBytes(r.Context(), p.bucket, row["picture"])
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "no-cache")
	if r.URL.Query().Get("c") == row["original"] {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Write(data)
}

type themeUpload struct {
	key, name, blurb string
	design           map[string]any
	pictures         map[string]upload
}

type droppedFile struct {
	name    string
	content []byte
}

func unzipped(name string, content []byte, used *uint64) ([]droppedFile, error) {
	z, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, access.Invalid("could not open %s: %v", name, err)
	}
	out := []droppedFile{}
	limit := uint64(fileLimit)
	for _, f := range z.File {
		base := path.Base(f.Name)
		if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "__MACOSX/") || strings.HasPrefix(base, ".") {
			continue
		}
		size := f.UncompressedSize64
		if size > limit || *used > limit-size {
			return nil, access.Invalid("%s holds more than a theme's worth of files", name)
		}
		*used += size
		rc, err := f.Open()
		if err != nil {
			return nil, access.Invalid("could not read %s in %s", f.Name, name)
		}
		// Read to EOF so the zip reader checks the checksum; it already stops at the declared size.
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, access.Invalid("could not read %s in %s", f.Name, name)
		}
		out = append(out, droppedFile{name: base, content: data})
	}
	return out, nil
}

func droppedFiles(r *http.Request) ([]droppedFile, error) {
	out := []droppedFile{}
	var used uint64
	limit := uint64(fileLimit)
	for _, header := range r.MultipartForm.File["file"] {
		f, err := header.Open()
		if err != nil {
			return nil, access.Invalid("could not read %s", header.Filename)
		}
		content, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return nil, access.Invalid("could not read %s", header.Filename)
		}
		name := path.Base(header.Filename)
		if http.DetectContentType(content) == "application/zip" {
			inside, err := unzipped(name, content, &used)
			if err != nil {
				return nil, err
			}
			out = append(out, inside...)
			continue
		}
		size := uint64(len(content))
		if size > limit || used > limit-size {
			return nil, access.Invalid("%s holds more than a theme's worth of files", name)
		}
		used += size
		out = append(out, droppedFile{name: name, content: content})
	}
	return out, nil
}

func themeFile(files []droppedFile) (droppedFile, error) {
	found := []droppedFile{}
	for _, f := range files {
		if f.name == "theme.json" {
			found = append(found, f)
		}
	}
	if len(found) == 0 {
		for _, f := range files {
			if strings.EqualFold(path.Ext(f.name), ".json") {
				found = append(found, f)
			}
		}
	}
	switch len(found) {
	case 0:
		return droppedFile{}, access.Invalid("there's no theme file: drop the theme's folder, a zip of it, or its theme.json")
	case 1:
		return found[0], nil
	}
	return droppedFile{}, access.Invalid("that holds %d theme files; drop one theme at a time", len(found))
}

func rewriteStrings(v any, f func(string) string) any {
	switch x := v.(type) {
	case string:
		return f(x)
	case map[string]any:
		for k, e := range x {
			x[k] = rewriteStrings(e, f)
		}
	case []any:
		for i, e := range x {
			x[i] = rewriteStrings(e, f)
		}
	}
	return v
}

func (t *themeUpload) filePictures(files []droppedFile) error {
	taken := map[string]bool{}
	for _, f := range files {
		taken[f.name] = true
	}
	byURL := map[string]string{}
	var bad error
	rewriteStrings(t.design, func(s string) string {
		return cssURL.ReplaceAllStringFunc(s, func(u string) string {
			m := cssURL.FindStringSubmatch(u)
			target := m[1] + m[2] + m[3]
			if !strings.HasPrefix(target, "data:") {
				return u
			}
			if name, ok := byURL[target]; ok {
				return `url("` + name + `")`
			}
			content, err := dataBytes(target)
			if err != nil {
				bad = access.Invalid("could not read a picture written into the design: %v", err)
				return u
			}
			mimeType := http.DetectContentType(content)
			ext, ok := blob.ImageExtensions[mimeType]
			if !ok {
				return u
			}
			if err := blob.Check(content); err != nil {
				bad = access.Invalid("could not read a picture written into the design: %v", err)
				return u
			}
			name := ""
			for n := len(byURL) + 1; name == "" || taken[name]; n++ {
				name = fmt.Sprintf("inline-%d.%s", n, ext)
			}
			taken[name] = true
			byURL[target] = name
			t.pictures[name] = upload{content: content, mimeType: mimeType}
			return `url("` + name + `")`
		})
	})
	return bad
}

func readThemeUpload(r *http.Request) (themeUpload, error) {
	files, err := droppedFiles(r)
	if err != nil {
		return themeUpload{}, err
	}
	file, err := themeFile(files)
	if err != nil {
		return themeUpload{}, err
	}
	d, err := storedDesign(string(file.content))
	if err != nil {
		return themeUpload{}, access.Invalid("%s: %v", file.name, err)
	}
	t := themeUpload{design: d, pictures: map[string]upload{}}
	for _, field := range []struct {
		name     string
		into     *string
		required bool
	}{{"key", &t.key, true}, {"name", &t.name, true}, {"blurb", &t.blurb, false}} {
		v, isText := d[field.name].(string)
		if (field.required && v == "") || (d[field.name] != nil && !isText) {
			return themeUpload{}, access.Invalid("the theme's file needs %s as text", field.name)
		}
		*field.into = v
		delete(d, field.name)
	}
	if !themeKey.MatchString(t.key) {
		return themeUpload{}, access.Invalid("a theme's key is lower-case letters, digits and dashes, not %q", t.key)
	}
	if err := standardDesign(d); err != nil {
		return themeUpload{}, access.Invalid("%v", err)
	}
	if err := t.filePictures(files); err != nil {
		return themeUpload{}, err
	}
	whole, err := json.Marshal(d)
	if err != nil {
		return themeUpload{}, err
	}
	if n := len([]rune(string(whole))); n > store.CellLimit {
		return themeUpload{}, access.Invalid("the design is %d characters long, even with its pictures moved into files; a theme's design holds at most %d - make its inline drawings smaller, or save them as PNG files beside theme.json", n, store.CellLimit)
	}
	named, _ := pictureNames(d)
	for _, f := range files {
		if !slices.Contains(named, f.name) {
			continue
		}
		if _, twice := t.pictures[f.name]; twice {
			return themeUpload{}, access.Invalid("there are two pictures named %s; keep one", f.name)
		}
		mimeType := http.DetectContentType(f.content)
		if _, ok := blob.ImageExtensions[mimeType]; !ok {
			return themeUpload{}, access.Invalid("%s is %s, not a supported picture", f.name, mimeType)
		}
		if err := blob.Check(f.content); err != nil {
			return themeUpload{}, access.Invalid("could not read %s: %v", f.name, err)
		}
		t.pictures[f.name] = upload{content: f.content, mimeType: mimeType}
	}
	for _, name := range named {
		if _, ok := t.pictures[name]; !ok {
			return themeUpload{}, access.Invalid("the design names %s; drop it in with the theme", name)
		}
	}
	return t, nil
}

func (m *Model) gatherAdmin(ctx context.Context, env Env) (bool, error) {
	q, err := Parse(`(from APP (where (= key "gather") (admin_of "gather")))`)
	if err != nil {
		return false, err
	}
	result := m.Run(ctx, q, env)
	return len(result.IDs) > 0, result.Err
}

func registerThemeUpload(mux *http.ServeMux, s *Store, pics *Pictures, tokens auth.Tokens) {
	mux.HandleFunc("POST /api/do/theme", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), tokens)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, fileLimit)
		if err := r.ParseMultipartForm(formMemory); err != nil {
			serve.Error(w, r, access.Invalid("send the theme as multipart form data: %v", err))
			return
		}
		t, err := readThemeUpload(r)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		kept := map[string]madeBytes{}
		for name, up := range t.pictures {
			if kept[name], err = keepBytes(r.Context(), s.Model(), pics.bucket, up.mimeType, up.content); err != nil {
				serve.Error(w, r, err)
				return
			}
		}
		design, _ := json.Marshal(t.design)
		whole := s.Model().whole(env)
		var ids []string
		_, err = s.Transact(r.Context(), actor, func(tx *store.Tx[Model]) error {
			m := s.In(tx)
			wr := &pendingWrites{}
			cells := map[string]any{"name": t.name, "blurb": t.blurb, "design": string(design)}
			theme := ""
			old := map[string]store.Row{}
			row, found := m.Table("THEME").Find(t.key, env.Viewer)
			if !found {
				if public, ok := m.Table("THEME").Find(t.key, ""); ok {
					admin, err := m.gatherAdmin(r.Context(), env)
					if err != nil {
						return err
					}
					row, found = public, admin
				}
			}
			if found {
				theme = wr.add(Edit{Set: row["id"], Cells: cells})
				for _, pic := range m.Table("THEME_PICTURE").Referencing("theme", row["id"]) {
					old[pic["name"]] = pic
				}
			} else {
				cells["key"], cells["added_by"] = t.key, env.Viewer
				theme = wr.add(Edit{Insert: "THEME", Row: cells})
			}
			for name, b := range kept {
				content, err := wr.content(m, b)
				if err != nil {
					return err
				}
				if pic, found := old[name]; found {
					if pic["original"] != content {
						wr.add(Edit{Set: pic["id"], Cells: map[string]any{"original": content}, uploaded: content})
					}
					continue
				}
				wr.add(Edit{Insert: "THEME_PICTURE", Row: map[string]any{"theme": theme, "name": name, "original": content}, uploaded: content})
			}
			for name, pic := range old {
				if _, found := kept[name]; !found {
					wr.add(Edit{Delete: pic["id"]})
				}
			}
			written, err := stageEdits(s, tx, env, wr.edits, "theme", whole)
			ids = written
			return err
		})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "uploaded a theme", "viewer", env.Viewer, "key", t.key, "pictures", len(kept))
		serve.Write(w, r, http.StatusOK, stored{Result: ids})
	})
}
