package db

import (
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/cells"
	"heliosian/internal/serve"
)

type schema = map[string]any

var kindNames = map[Kind]string{
	Text: "text", Enum: "enum", ID: "id", Ref: "ref", Bool: "bool", Int: "int", Money: "money",
	Float: "float", Date: "date", Moment: "moment", Blob: "blob", Order: "order", Email: "email", URL: "url",
}

func component(name string) schema {
	return schema{"$ref": "#/components/schemas/" + name}
}

func prefixOf(table string) string {
	t, ok := Lookup(table)
	if !ok {
		return ""
	}
	id, _ := t.Column("id")
	return id.Prefix
}

func idPattern(prefix string) string {
	return "^" + regexp.QuoteMeta(prefix) + "[" + idAlphabet + "]{" + strconv.Itoa(randomLength) + "}$"
}

func cellSchema(c Column) schema {
	out := schema{"type": "string", "x-kind": kindNames[c.Kind]}
	notes := []string{c.Description}
	switch c.Kind {
	case Enum:
		out["enum"] = append([]string{""}, c.ValueNames()...)
		described := map[string]string{}
		for _, value := range c.Values {
			described[value.Name] = value.Description
		}
		out["x-enumDescriptions"] = described
	case ID:
		out["pattern"] = idPattern(c.Prefix)
		out["readOnly"] = true
		if c.Name == "id" {
			notes = append(notes, "Minted by the server.")
		} else {
			notes = append(notes, "A token minted by the server.")
		}
	case Ref:
		if c.Target == "" {
			notes = append(notes, "The ID of a row in any table.")
			break
		}
		out["x-relation"] = c.Target
		out["pattern"] = "^$|" + idPattern(prefixOf(c.Target))
		notes = append(notes, "A "+c.Target+" ID.")
	case Bool:
		out["enum"] = []string{"", "Yes", "No"}
	case Int:
		out["pattern"] = `^[0-9]*$`
	case Money:
		out["pattern"] = `^$|^[0-9]+(\.[0-9]{1,2})?$`
	case Float:
		notes = append(notes, "A decimal number.")
	case Date:
		out["examples"] = []string{"2026-09-24"}
	case Moment:
		out["examples"] = []string{"2026-09-24", "2026-09-24 16:00", "2026-09-24 16:00:05"}
		notes = append(notes, "A day, or a day and a time to the minute or the second, in the school's time zone.")
	case Blob:
		notes = append(notes, "An object's name in the media bucket.")
	case Order:
		notes = append(notes, "An order key: rows sort by it.")
	case Email:
		out["format"] = "email"
	case URL:
		out["format"] = "uri"
	}
	if c.Required {
		out["x-required"] = true
		notes = append(notes, "Required.")
	}
	if c.Generated {
		out["readOnly"] = true
		out["x-generated"] = true
		notes = append(notes, "Generated from the row's other columns, never stored.")
	}
	if c.Private {
		out["x-private"] = true
		notes = append(notes, "Private: answered to the import alone.")
	}
	out["description"] = strings.Join(notes, " ")
	return out
}

func tableSchema(t Table) schema {
	properties := schema{}
	order := []string{}
	for _, c := range t.Columns {
		properties[c.Name] = cellSchema(c)
		order = append(order, c.Name)
	}
	out := schema{
		"type":         "object",
		"properties":   properties,
		"x-columns":    order,
		"x-sheet":      t.Sheet,
		"x-unique":     slices.Clone(t.Unique),
		"x-generated":  t.Generated,
		"x-appendOnly": t.AppendOnly,
		"description":  t.Description,
	}
	if t.Generated {
		out["description"] = t.Description + " Generated: worked out from other tables, never stored or written."
	}
	return out
}

func tableNames() []string {
	out := []string{}
	for _, t := range Tables {
		out = append(out, t.Name)
	}
	return out
}

func answers(description, name string) schema {
	return schema{"description": description, "content": schema{"application/json": schema{"schema": component(name)}}}
}

func failure(description string) schema {
	return schema{"description": description, "content": schema{"text/plain": schema{"schema": schema{"type": "string"}}}}
}

func refusals(out schema) schema {
	out["401"] = failure("A bearer key that is not the import's.")
	out["403"] = failure("The caller is in no person's row, or a write the policies refuse.")
	out["413"] = failure("The body is too large.")
	out["415"] = failure("The body is neither of the types this takes.")
	return out
}

func spec() schema {
	schemas := schema{}
	resources := schema{}
	examples := schema{}
	for _, t := range Tables {
		schemas[t.Name] = tableSchema(t)
		resources[t.Name] = schema{"type": "object", "additionalProperties": component(t.Name)}
		examples[t.Name] = schema{"summary": t.Name, "value": "(from " + t.Name + " (limit 20))"}
	}
	tables := tableNames()
	cell := schema{"type": []string{"string", "number", "boolean"}, "description": "Text, a number, or true/false for a yes/no; \"\" blanks it."}
	schemas["Answer"] = schema{
		"type":     "object",
		"required": []string{"now", "query", "result", "resources"},
		"properties": schema{
			"now":       schema{"type": "string", "description": "The school's time as the query saw it.", "examples": []string{"2026-09-24 16:00:05"}},
			"query":     schema{"type": "string", "description": "The query in canonical text."},
			"tree":      schema{"$ref": "#/components/schemas/Query", "description": "The query's JSON form, answered when it was sent as text."},
			"result":    schema{"type": "array", "items": schema{"type": "string"}, "description": "The IDs the query answers, in order."},
			"resources": schema{"type": "object", "properties": resources, "description": "Every row reached, the result's and its includes', by table then ID, each as the reader may see it."},
		},
	}
	schemas["Query"] = schema{
		"type":        "object",
		"required":    []string{"from"},
		"description": "A query as a JSON tree, one object per bracket of the language.",
		"properties": schema{
			"from":    schema{"type": "string", "enum": tables},
			"as":      schema{"type": "string", "description": "A name for the row, for paths starting @name."},
			"where":   schema{"type": "array", "items": schema{"type": "object"}, "description": "Conditions, anded."},
			"order":   schema{"type": "array", "items": schema{"type": "object", "properties": schema{"path": schema{"type": "string"}, "dir": schema{"type": "string", "enum": []string{"asc", "desc"}}}}},
			"limit":   schema{"type": "integer", "minimum": 1},
			"include": schema{"type": "array", "items": schema{"type": "string"}, "description": "Paths whose rows are answered alongside."},
		},
	}
	schemas["Batch"] = schema{
		"type":     "object",
		"required": []string{"batch"},
		"properties": schema{"batch": schema{"type": "array", "items": schema{"oneOf": []schema{
			{"type": "object", "title": "insert", "required": []string{"insert", "row"}, "properties": schema{
				"insert": schema{"type": "string", "enum": tables},
				"as":     schema{"type": "string", "description": "Names the new row for later writes in the batch, as @name."},
				"row":    schema{"type": "object", "additionalProperties": cell},
			}},
			{"type": "object", "title": "set", "required": []string{"set", "cells"}, "properties": schema{
				"set":   schema{"type": "string", "description": "The row's ID, or @name."},
				"cells": schema{"type": "object", "additionalProperties": cell},
			}},
			{"type": "object", "title": "delete", "required": []string{"delete"}, "properties": schema{
				"delete": schema{"type": "string", "description": "The row's ID, or @name."},
			}},
		}}}},
	}
	schemas["Written"] = schema{"type": "object", "properties": schema{"result": schema{"type": "array", "items": schema{"type": "string"}, "description": "The ID each write touched."}}}
	schemas["Stored"] = schema{"type": "object", "properties": schema{
		"result": schema{"type": "array", "items": schema{"type": "string"}},
		"hash":   schema{"type": "string", "description": "The SHA-256 of the stored file."},
	}}
	schemas["Fetched"] = schema{"type": "object", "properties": schema{
		"result": schema{"type": "array", "items": schema{"type": "string"}},
		"hash":   schema{"type": "string", "description": "The SHA-256 of the content, when the body was kept."},
		"fetch":  schema{"type": "string", "enum": []string{"gone", "sign_in", "refused"}, "description": "What fetch was set to, when it was."},
		"why":    schema{"type": "string", "description": "Why fetch was set."},
	}}
	schemas["SearchHit"] = schema{"type": "object", "required": []string{"id", "name", "summary"}, "properties": schema{
		"id":      schema{"type": "string"},
		"name":    schema{"type": "string", "description": "The row's name; a document's is the name of the nearest document above it with a kind, such as its email's subject."},
		"href":    schema{"type": "string", "description": "The page the row has on the Helios apps or the web, when it has one."},
		"summary": schema{"type": "string"},
	}}
	crop := schema{"type": "integer", "minimum": 0}
	paths := schema{
		"/api/q": schema{
			"query": schema{
				"tags":        []string{"read"},
				"summary":     "Run a query as the signed-in person",
				"operationId": "query",
				"requestBody": schema{"required": true, "content": schema{
					"text/plain":       schema{"schema": schema{"type": "string"}, "examples": examples},
					"application/json": schema{"schema": component("Query")},
				}},
				"responses": refusals(schema{
					"200": answers("The IDs the query answers and every row it reached.", "Answer"),
					"400": failure("The query does not parse or check against the schema."),
				}),
			},
			"post": schema{
				"tags":        []string{"write"},
				"summary":     "Apply a batch of writes, all or none",
				"operationId": "write",
				"requestBody": schema{"required": true, "content": schema{"application/json": schema{"schema": component("Batch")}}},
				"responses": refusals(schema{
					"200": answers("The ID each write touched.", "Written"),
					"400": failure("A write that does not check, naming it."),
				}),
			},
		},
		doPrefix + "photo": schema{
			"post": schema{
				"tags":    []string{"do"},
				"summary": "Add a photo of a person or a group",
				"requestBody": schema{"required": true, "content": schema{"multipart/form-data": schema{"schema": schema{
					"type":     "object",
					"required": []string{"photo"},
					"properties": schema{
						"person": schema{"type": "string", "pattern": idPattern(PersonPrefix)}, "group": schema{"type": "string", "pattern": idPattern(GroupPrefix)},
						"photo":     schema{"type": "string", "contentMediaType": "application/octet-stream"},
						"crop_left": crop, "crop_top": crop, "crop_width": crop, "crop_height": crop,
					},
				}}}},
				"responses": refusals(schema{"200": answers("The new PHOTO and the original's hash.", "Stored"), "400": failure("Not an image, or a bad crop box.")}),
			},
		},
		doPrefix + "file": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Add a file the community received or shared",
				"description": "The file's type is its part's Content-Type, or read from its bytes when that is absent or application/octet-stream. Mail (message/rfc822) is read from its headers alone and takes no other field; anything else needs url and published and becomes a root of kind file, or calendar with kind=calendar.",
				"requestBody": schema{"required": true, "content": schema{"multipart/form-data": schema{"schema": schema{
					"type":     "object",
					"required": []string{"file"},
					"properties": schema{
						"file":      schema{"type": "string", "contentMediaType": "application/octet-stream"},
						"kind":      schema{"type": "string", "enum": []string{"calendar"}},
						"name":      schema{"type": "string"},
						"url":       schema{"type": "string", "format": "uri"},
						"published": schema{"type": "string", "description": "When the file last changed, in school time, as " + cells.StampFormat},
					},
				}}}},
				"responses": refusals(schema{"200": answers("The root DOCUMENT holding the file and its hash.", "Stored"), "400": failure("Mail that does not parse or has no readable Date, or another file with no url or published moment.")}),
			},
		},
		doPrefix + "fetched": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Fill a linked document still to fetch",
				"description": "Takes what a fetch got for a linked document whose content is blank: its body, which becomes the document's content if it is kept, or sets fetch to refused if not; or a stop, which sets fetch to it.",
				"requestBody": schema{"required": true, "content": schema{"multipart/form-data": schema{"schema": schema{
					"type":     "object",
					"required": []string{"document"},
					"properties": schema{
						"document": schema{"type": "string", "pattern": idPattern(DocumentPrefix)},
						"body":     schema{"type": "string", "contentMediaType": "application/octet-stream"},
						"stop":     schema{"type": "string", "enum": []string{"gone", "sign_in", "refused"}},
					},
				}}}},
				"responses": refusals(schema{"200": answers("The DOCUMENT, and the content's hash, or the fetch it was set to and why.", "Fetched"), "400": failure("Not a linked document still to fetch, or neither a body nor a stop.")}),
			},
		},
		doPrefix + "search": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Search groups, people and documents",
				"description": "Answers, for each of GROUP, PERSON and DOCUMENT, the rows the caller may read best first by words - the rows whose keywords hold the most of the words, the rarer words weighing more - and by meaning - the rows whose embedded chunks lie closest to the words'. Only entries of the current version are searched. A document stands for its near-identical copies, which are left out.",
				"requestBody": schema{"required": true, "content": schema{"application/json": schema{"schema": schema{
					"type":       "object",
					"required":   []string{"words"},
					"properties": schema{"words": schema{"type": "string"}},
				}}}},
				"responses": refusals(schema{
					"200": schema{"description": "The hits by table.", "content": schema{"application/json": schema{"schema": schema{
						"type": "object",
						"additionalProperties": schema{
							"type":     "object",
							"required": []string{"words", "meaning"},
							"properties": schema{
								"words":   schema{"type": "array", "items": schema{"$ref": "#/components/schemas/SearchHit"}},
								"meaning": schema{"type": "array", "items": schema{"$ref": "#/components/schemas/SearchHit"}},
							},
						},
					}}}},
					"400": failure("No words."),
				}),
			},
		},
		doPrefix + "search/make": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Make a row's search entry",
				"description": "The import key alone. Queues the search entry of one group, person or document extract for the makers, who ask Vertex and Claude for it off the request path and store it under the current version. Nothing else makes entries.",
				"requestBody": schema{"required": true, "content": schema{"application/json": schema{"schema": schema{
					"type":       "object",
					"required":   []string{"id"},
					"properties": schema{"id": schema{"type": "string"}},
				}}}},
				"responses": refusals(schema{
					"200": schema{"description": "Queued; the entry's object.", "content": schema{"application/json": schema{"schema": schema{
						"type":       "object",
						"required":   []string{"object"},
						"properties": schema{"object": schema{"type": "string"}},
					}}}},
					"404": failure("No search row for the ID."),
				}),
			},
		},
		doPrefix + "compose": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Write a query from a description",
				"description": "On the admin host alone. Asks Claude for a query in the language answering the words, and answers what it understood them to ask for and the query, blank when it found none. Nothing is run.",
				"requestBody": schema{"required": true, "content": schema{"application/json": schema{"schema": schema{
					"type":       "object",
					"required":   []string{"words"},
					"properties": schema{"words": schema{"type": "string"}},
				}}}},
				"responses": refusals(schema{
					"200": schema{"description": "What Claude understood, and the query.", "content": schema{"application/json": schema{"schema": schema{
						"type":       "object",
						"required":   []string{"understanding", "query"},
						"properties": schema{"understanding": schema{"type": "string"}, "query": schema{"type": "string"}},
					}}}},
					"400": failure("No words, or Claude refused or could not answer."),
					"429": failure("Too many Claude calls from this person this hour."),
				}),
			},
		},
	}
	return schema{
		"openapi": "3.2.0",
		"info":    schema{"title": "Heliosian data model", "version": "1"},
		"tags": []schema{
			{"name": "read", "description": "The query language, as text or a JSON tree."},
			{"name": "write", "description": "Batches of inserts, sets and deletes."},
			{"name": "do", "description": "Calls that are more than writing rows."},
		},
		"security":   []schema{{}, {"import": []string{}}},
		"paths":      paths,
		"components": schema{"schemas": schemas, "securitySchemes": schema{"import": schema{"type": "http", "scheme": "bearer", "description": "IMPORT_KEY, to run as the import."}}},
	}
}

func openapi(w http.ResponseWriter, r *http.Request) {
	serve.Write(w, r, http.StatusOK, spec())
}
