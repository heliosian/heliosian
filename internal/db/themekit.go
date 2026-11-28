package db

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const themeKitDir = "web/gather/themekit/"

var (
	kitModules = []struct{ file, name string }{
		{"web/public/common/elements.js", "/elements.js"},
		{"web/public/common/andlist.js", "/andlist.js"},
		{"web/public/common/themes.js", "/themes.js"},
		{"web/public/common/envelope.js", "/envelope.js"},
	}
	kitImport = regexp.MustCompile(`^import \{([^}]*)\} from '([^']+)';$`)
	kitExport = regexp.MustCompile(`^export (?:async function|function|const|let) (\w+)`)
)

func bundledModules() (string, error) {
	var out strings.Builder
	out.WriteString("const modules = {};\n")
	for _, mod := range kitModules {
		raw, err := os.ReadFile(mod.file)
		if err != nil {
			return "", err
		}
		names := []string{}
		fmt.Fprintf(&out, "modules[%q] = (() => {\n", mod.name)
		for _, line := range strings.Split(string(raw), "\n") {
			if m := kitImport.FindStringSubmatch(line); m != nil {
				fmt.Fprintf(&out, "const {%s} = modules[%q];\n", m[1], m[2])
				continue
			}
			if strings.HasPrefix(line, "import ") {
				return "", fmt.Errorf("%s: the theme kit can't bundle %q", mod.file, line)
			}
			if m := kitExport.FindStringSubmatch(line); m != nil {
				names = append(names, m[1])
				line = strings.TrimPrefix(line, "export ")
			}
			out.WriteString(line + "\n")
		}
		fmt.Fprintf(&out, "return {%s};\n})();\n", strings.Join(names, ", "))
	}
	return out.String(), nil
}

func prettyJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(v)
	return buf.Bytes(), err
}

func themePreview() ([]byte, error) {
	page, err := os.ReadFile(themeKitDir + "preview.html")
	if err != nil {
		return nil, err
	}
	style, err := os.ReadFile("web/public/common/envelope.css")
	if err != nil {
		return nil, err
	}
	code, err := bundledModules()
	if err != nil {
		return nil, err
	}
	rules, err := json.Marshal(map[string]any{
		"key": themeKey.String(), "shapes": sealShapes, "aligns": cardAligns, "details": cardDetails, "dateParts": dateParts,
		"mailPaths": themeMailPaths, "textRoles": themeTextRoles,
	})
	if err != nil {
		return nil, err
	}
	out := string(page)
	for placeholder, value := range map[string]string{
		"/*GATHER-STYLE*/": string(style), "/*GATHER-CODE*/": code, "/*GATHER-STANDARD*/": string(themeBaseJSON), "/*GATHER-RULES*/": string(rules),
	} {
		if !strings.Contains(out, placeholder) {
			return nil, fmt.Errorf("preview.html has no %s", placeholder)
		}
		out = strings.Replace(out, placeholder, value, 1)
	}
	return []byte(out), nil
}

func (a *InvitationArt) themePackage(ctx context.Context, m *Model, row store.Row, folder string, files map[string][]byte) error {
	design, err := storedDesign(row["design"])
	if err != nil {
		return err
	}
	pkg := map[string]any{"key": row["key"], "name": row["name"], "blurb": row["blurb"]}
	for k, v := range design {
		pkg[k] = v
	}
	raw, err := prettyJSON(pkg)
	if err != nil {
		return err
	}
	files[folder+"theme.json"] = raw
	for _, pic := range m.Table("THEME_PICTURE").Referencing("theme", row["id"]) {
		data, _, err := m.contentBytes(ctx, a.pics.bucket, pic["picture"])
		if err != nil {
			return err
		}
		files[folder+pic["name"]] = data
	}
	return nil
}

func (a *InvitationArt) themeKit(w http.ResponseWriter, r *http.Request) {
	m := a.pics.s.Model()
	files := map[string][]byte{"standard.json": themeBaseJSON}
	readme, err := os.ReadFile(themeKitDir + "README.md")
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	files["README.md"] = readme
	filename := "gather-theme-kit.zip"
	if id := r.URL.Query().Get("theme"); id != "" {
		row, ok := m.Table("THEME").Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if err := a.themePackage(r.Context(), m, row, "my-theme/", files); err != nil {
			serve.Error(w, r, err)
			return
		}
		filename = row["key"] + "-theme-kit.zip"
	} else {
		starter, err := os.ReadFile(themeKitDir + "starter.json")
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		files["my-theme/theme.json"] = starter
	}
	preview, err := themePreview()
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	files["preview.html"] = preview
	for _, row := range m.Table("THEME").All() {
		if row["added_by"] != "" || row["key"] == "plain" {
			continue
		}
		if err := a.themePackage(r.Context(), m, row, "examples/"+row["key"]+"/", files); err != nil {
			serve.Error(w, r, err)
			return
		}
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := z.Create("gather-theme-kit/" + name)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		f.Write(data)
	}
	if err := z.Close(); err != nil {
		serve.Error(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf.Bytes())
}
