package db

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type specProperty struct {
	Enum             []string          `json:"enum"`
	EnumDescriptions map[string]string `json:"x-enumDescriptions"`
	Pattern          string            `json:"pattern"`
	Relation         string            `json:"x-relation"`
	Description      string            `json:"description"`
}

type specDoc struct {
	OpenAPI    string                               `json:"openapi"`
	Paths      map[string]map[string]map[string]any `json:"paths"`
	Components struct {
		Schemas map[string]struct {
			Description string                  `json:"description"`
			Properties  map[string]specProperty `json:"properties"`
		} `json:"schemas"`
	} `json:"components"`
}

func served(t *testing.T) specDoc {
	t.Helper()
	s := sample(t)
	mux := http.NewServeMux()
	Register(mux, s, newPictures(s), testTokens)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out specDoc
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTheSpecDescribesTheQueryAPI(t *testing.T) {
	spec := served(t)
	for path, method := range map[string]string{"/api/q": "query", "/api/do/photo": "post", "/api/do/file": "post", "/api/do/fetched": "post"} {
		if _, ok := spec.Paths[path][method]; !ok {
			t.Errorf("no %s %s", method, path)
		}
	}
	if _, ok := spec.Paths["/api/q"]["post"]; !ok {
		t.Error("no post /api/q")
	}
	for _, table := range Tables {
		properties := spec.Components.Schemas[table.Name].Properties
		for _, c := range table.Columns {
			if _, ok := properties[c.Name]; !ok {
				t.Errorf("%s has no %s", table.Name, c.Name)
			}
		}
	}
	if got := spec.Components.Schemas["GROUP"].Properties["parent"].Relation; got != "GROUP" {
		t.Errorf("GROUP.parent relates to %q", got)
	}
	if got := spec.Components.Schemas["MEMBER"].Properties["member"].Enum; !slices.Contains(got, "excluded") {
		t.Errorf("MEMBER.member enum %v", got)
	}
}

func TestTheSpecDescribesEveryDoCall(t *testing.T) {
	route := regexp.MustCompile(`^"(?:POST )?(/api/do/[a-z-]+)"$`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "openapi.go" {
			continue
		}
		f, err := parser.ParseFile(gotoken.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == gotoken.STRING {
				if m := route.FindStringSubmatch(lit.Value); m != nil {
					registered[m[1]] = true
				}
			}
			return true
		})
	}
	if len(registered) == 0 {
		t.Fatal("found no /api/do/ routes in the source")
	}
	spec := served(t)
	for path := range registered {
		if _, ok := spec.Paths[path]["post"]; !ok {
			t.Errorf("the spec does not describe POST %s", path)
		}
	}
	for path := range spec.Paths {
		if strings.HasPrefix(path, "/api/do/") && !registered[path] {
			t.Errorf("the spec describes %s, which nothing registers", path)
		}
	}
}

func TestTheSpecCarriesEveryDescription(t *testing.T) {
	spec := served(t)
	for _, table := range Tables {
		component := spec.Components.Schemas[table.Name]
		if !strings.HasPrefix(component.Description, table.Description) {
			t.Errorf("%s: description %q", table.Name, component.Description)
		}
		for _, c := range table.Columns {
			p := component.Properties[c.Name]
			if !strings.HasPrefix(p.Description, c.Description) {
				t.Errorf("%s.%s: description %q", table.Name, c.Name, p.Description)
			}
			for _, value := range c.Values {
				if p.EnumDescriptions[value.Name] != value.Description {
					t.Errorf("%s.%s: %s described as %q", table.Name, c.Name, value.Name, p.EnumDescriptions[value.Name])
				}
			}
		}
	}
}

func TestEveryAnsweredCellFitsTheSpec(t *testing.T) {
	spec := served(t)
	s := sample(t)
	for _, table := range Tables {
		code, out, body := ask(t, s, "text/plain", importing, "(from "+table.Name+")")
		if code != http.StatusOK {
			t.Fatalf("%s: %d %s", table.Name, code, body)
		}
		for name, rows := range out.Resources {
			properties := spec.Components.Schemas[name].Properties
			for id, row := range rows {
				for column, value := range row {
					p, ok := properties[column]
					if !ok {
						t.Errorf("%s %s: %s is not in the spec", name, id, column)
						continue
					}
					if p.Enum != nil && !slices.Contains(p.Enum, value) {
						t.Errorf("%s %s: %s %q is not one of %v", name, id, column, value, p.Enum)
					}
					if p.Pattern != "" && !regexp.MustCompile(p.Pattern).MatchString(value) {
						t.Errorf("%s %s: %s %q does not match %s", name, id, column, value, p.Pattern)
					}
				}
			}
		}
	}
}
