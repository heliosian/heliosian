package api

import (
	"encoding"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/id"
	"heliosian/internal/serve"
)

const (
	docsPage = "web/common/swagger/index.html"
	erdPage  = "web/common/erd/index.html"
)

type schema = map[string]any

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
)

func implements(t, iface reflect.Type) bool {
	return t.Implements(iface) || reflect.PointerTo(t).Implements(iface)
}

func shapeOf(t reflect.Type) schema {
	return shapeIn(t, map[reflect.Type]bool{})
}

func shapeIn(t reflect.Type, open map[reflect.Type]bool) schema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case implements(t, jsonMarshaler):
		return schema{}
	case implements(t, textMarshaler):
		return schema{"type": "string"}
	}
	switch t.Kind() {
	case reflect.String:
		return schema{"type": "string"}
	case reflect.Bool:
		return schema{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return schema{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return schema{"type": "number"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return schema{"type": "string"}
		}
		return schema{"type": "array", "items": shapeIn(t.Elem(), open)}
	case reflect.Map:
		return schema{"type": "object", "additionalProperties": shapeIn(t.Elem(), open)}
	case reflect.Struct:
		if open[t] {
			return schema{"type": "object"}
		}
		open[t] = true
		defer delete(open, t)
		properties := schema{}
		fieldsOf(t, 0, properties, map[string]int{}, open)
		return schema{"type": "object", "properties": properties}
	}
	return schema{}
}

func fieldsOf(t reflect.Type, depth int, into schema, depths map[string]int, open map[reflect.Type]bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		inner := f.Type
		for inner.Kind() == reflect.Pointer {
			inner = inner.Elem()
		}
		if f.Anonymous && name == "" && inner.Kind() == reflect.Struct {
			fieldsOf(inner, depth+1, into, depths, open)
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		if d, ok := depths[name]; ok && d <= depth {
			continue
		}
		depths[name] = depth
		into[name] = shapeIn(f.Type, open)
	}
}

func ref(name string) schema {
	return schema{"$ref": "#/components/schemas/" + name}
}

func (reg *Registry[S]) names() []string {
	out := []string{}
	for name := range reg.types {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func (t *Type[S]) actions() []string {
	out := []string{}
	for name := range t.Actions {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func (t *Type[S]) relations() []string {
	out := []string{}
	for name := range t.Relations {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func (t *Type[S]) schema() schema {
	out := shapeOf(reflect.TypeOf(t.Shape))
	properties := out["properties"].(schema)
	properties["id"] = schema{"type": "string"}
	if len(t.Actions) > 0 {
		can := schema{}
		for _, name := range t.actions() {
			can[name] = schema{"type": "boolean"}
		}
		properties["can"] = schema{"type": "object", "description": "Whether the viewer may take each action now.", "properties": can}
	}
	for _, name := range t.relations() {
		rel := t.Relations[name]
		description := rel.Type + " IDs; present only when included."
		if rel.Many {
			properties[name] = schema{"type": "array", "items": schema{"type": "string"}, "description": description, "x-relation": rel.Type}
			continue
		}
		properties[name] = schema{"type": []string{"string", "null"}, "description": rel.Type + " ID; present only when included.", "x-relation": rel.Type}
	}
	return out
}

func (reg *Registry[S]) reachable(from string) []string {
	seen := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		t, ok := reg.types[queue[0]]
		queue = queue[1:]
		if !ok {
			continue
		}
		for _, rel := range t.Relations {
			if _, ok := reg.types[rel.Type]; ok && !seen[rel.Type] {
				seen[rel.Type] = true
				queue = append(queue, rel.Type)
			}
		}
	}
	out := []string{}
	for name := range seen {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func idKeyed(values schema) schema {
	values["x-additionalPropertiesName"] = "<id>"
	return schema{
		"type":                 "object",
		"minProperties":        1,
		"propertyNames":        schema{"type": "string", "pattern": "^[" + id.Alphabet + "]{" + strconv.Itoa(id.Length) + "}$"},
		"additionalProperties": values,
	}
}

func resourcesOf(names []string) schema {
	properties := schema{}
	for _, name := range names {
		properties[name] = idKeyed(ref(name))
	}
	return schema{"type": "object", "description": "Every resource reached, keyed by type then ID.", "properties": properties}
}

func wrapped(result, reached schema) schema {
	return schema{"type": "object", "properties": schema{
		"now":       schema{"type": "string", "description": "The server's clock in the school's zone."},
		"result":    result,
		"resources": reached,
	}}
}

func answer(description string, body schema) schema {
	return schema{"description": description, "content": schema{"application/json": schema{"schema": body}}}
}

func noContent() schema {
	return schema{"204": schema{"description": "Done."}}
}

func withBody(operation schema, input reflect.Type) schema {
	if input == reflect.TypeFor[serve.None]() {
		return operation
	}
	operation["requestBody"] = schema{"required": false, "content": schema{"application/json": schema{"schema": shapeOf(input)}}}
	return operation
}

func param(in, name, description string) schema {
	out := schema{"in": in, "name": name, "description": description, "schema": schema{"type": "string"}}
	if in == "path" {
		out["required"] = true
		return out
	}
	out["allowEmptyValue"] = true
	return out
}

func (t *Type[S]) include() schema {
	description := "Comma-separated dotted relation paths."
	if len(t.Relations) > 0 {
		description += " Relations: " + strings.Join(t.relations(), ", ") + "."
	}
	return param("query", "include", description)
}

func (reg *Registry[S]) spec() schema {
	schemas := schema{}
	paths := schema{}
	tags := []schema{{"name": "api"}}
	for _, name := range reg.names() {
		t := reg.types[name]
		schemas[name] = t.schema()
		reached := resourcesOf(reg.reachable(name))
		tags = append(tags, schema{"name": name})
		tag := []string{name}
		id := param("path", "id", "An ID or an alias of this type.")

		list := []schema{t.include()}
		filters := []string{}
		for filter := range t.Filters {
			filters = append(filters, filter)
		}
		slices.Sort(filters)
		for _, filter := range filters {
			list = append(list, param("query", filter, "Declared filter."))
		}
		mine := param("query", "mine", "Keep what me.mine says is the viewer's.")
		mine["schema"] = schema{"type": "boolean", "enum": []bool{true}}
		list = append(list, mine)
		if len(t.Actions) > 0 {
			can := param("query", "can", "Keep what the viewer may do this action to.")
			can["schema"] = schema{"type": "string", "enum": t.actions()}
			list = append(list, can)
		}
		collection := schema{"get": schema{
			"tags": tag, "summary": "List " + name, "parameters": list,
			"responses": schema{"200": answer("The collection.", wrapped(schema{"type": "array", "items": schema{"type": "string"}}, reached))},
		}}
		if t.Create.do != nil {
			collection["post"] = withBody(schema{
				"tags": tag, "summary": "Create one of " + name,
				"responses": schema{"200": answer("The new ID.", schema{"type": "object", "properties": schema{"id": schema{"type": "string"}}})},
			}, t.Create.input)
		}
		paths["/api/"+name] = collection

		one := schema{"get": schema{
			"tags": tag, "summary": "Get one of " + name, "parameters": []schema{id, t.include()},
			"responses": schema{"200": answer("The resource.", wrapped(schema{"type": "string"}, reached))},
		}}
		for _, action := range t.actions() {
			if action == "delete" {
				one["delete"] = schema{"tags": tag, "summary": "Delete one of " + name, "parameters": []schema{id}, "responses": noContent()}
				continue
			}
			responses := noContent()
			if t.Actions[action].makes {
				responses = schema{"200": answer("The new ID.", schema{"type": "object", "properties": schema{"id": schema{"type": "string"}}})}
			}
			paths["/api/"+name+"/{id}/"+action] = schema{"post": withBody(schema{
				"tags": tag, "summary": action, "parameters": []schema{id}, "responses": responses,
			}, t.Actions[action].input)}
		}
		paths["/api/"+name+"/{id}"] = one
	}
	schemas["resources"] = resourcesOf(reg.names())
	general := []string{"api"}
	paths["/api/me"] = schema{"get": schema{
		"tags": general, "summary": "The viewer's address and allowances",
		"responses": schema{"200": answer("The viewer.", schema{"type": "object", "properties": schema{
			"email": schema{"type": "string"}, "allowances": schema{"type": "array", "items": schema{"type": "string"}},
		}})},
	}}
	paths["/api/r/{id}"] = schema{"get": schema{
		"tags": general, "summary": "Any resource by ID, whatever its type", "parameters": []schema{param("path", "id", "An ID; aliases don't resolve here."), param("query", "include", "Comma-separated dotted relation paths.")},
		"responses": schema{"200": answer("The resource.", wrapped(schema{"type": "string"}, ref("resources")))},
	}}
	paths["/api/query"] = schema{"post": schema{
		"tags": general, "summary": "Several reads from one snapshot",
		"requestBody": schema{"required": true, "content": schema{"application/json": schema{"schema": schema{
			"type": "object", "additionalProperties": schema{"type": "object", "properties": schema{"path": schema{"type": "string"}}},
			"example": schema{"people": schema{"path": "/api/people?listed&include=children"}},
		}}}},
		"responses": schema{"200": answer("Each read's result by name, one set of resources.", wrapped(schema{"type": "object"}, ref("resources")))},
	}}
	paths["/api/act"] = schema{"post": schema{
		"tags": general, "summary": "Several writes as one change, all or nothing",
		"requestBody": schema{"required": true, "content": schema{"application/json": schema{"schema": schema{
			"type": "array", "items": schema{"type": "object", "properties": schema{
				"method": schema{"type": "string", "enum": []string{"POST", "DELETE"}}, "path": schema{"type": "string"}, "body": schema{"type": "object"},
			}},
		}}}},
		"responses": schema{"200": answer("Each write's result.", schema{"type": "object", "properties": schema{
			"results": schema{"type": "array", "items": schema{"type": "object", "properties": schema{"id": schema{"type": "string"}}}},
		}})},
	}}
	return schema{
		"openapi":    "3.1.0",
		"info":       schema{"title": "Heliosian", "version": "1"},
		"tags":       tags,
		"paths":      paths,
		"components": schema{"schemas": schemas},
	}
}

func (reg *Registry[S]) openapi(*http.Request, serve.None) (schema, error) {
	return reg.spec(), nil
}

func docs(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, docsPage)
}

func erd(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, erdPage)
}
