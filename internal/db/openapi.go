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
	Float: "float", Date: "date", Moment: "moment", Blob: "blob", Order: "order", Email: "email", URL: "url", Words: "words",
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
		notes = append(notes, "An object's name in the media bucket: content/ and the SHA-256 of its bytes.")
	case Order:
		notes = append(notes, "An order key: rows sort by it.")
	case Email:
		out["format"] = "email"
	case URL:
		out["format"] = "uri"
	case Words:
		out["examples"] = []string{`[["hummingbirds","hummingbirds-parents"],["parent-ed"]]`}
		notes = append(notes, "A JSON list of lists of words, each a group's slug or going.")
	}
	if c.Required {
		out["x-required"] = true
		notes = append(notes, "Required.")
	}
	if c.Default != "" {
		out["default"] = c.Default
		notes = append(notes, "An insert that leaves it blank gets "+c.Default+".")
	}
	if c.Generated {
		out["readOnly"] = true
		out["x-generated"] = true
		notes = append(notes, "Generated from the row's other columns, never stored.")
	}
	if c.Private {
		out["x-private"] = true
		notes = append(notes, "Private: answered in import mode alone.")
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
		"x-distinct":   slices.Clone(t.Distinct),
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

func jsonBody(required []string, properties schema) schema {
	return schema{"required": true, "content": schema{"application/json": schema{"schema": schema{
		"type":       "object",
		"required":   required,
		"properties": properties,
	}}}}
}

func object(description string, required []string, properties schema) schema {
	return schema{"description": description, "content": schema{"application/json": schema{"schema": schema{
		"type":       "object",
		"required":   required,
		"properties": properties,
	}}}}
}

func refusals(out schema) schema {
	out["401"] = failure("A bearer token this server did not issue, has expired, or was signed out.")
	out["403"] = failure("The caller is in no person's row, a spoof by someone who is not a super admin, or a write the policies refuse.")
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
			"result":    schema{"type": "array", "items": schema{"type": "string"}, "description": "The IDs the query answers, in order."},
			"resources": schema{"type": "object", "properties": resources, "description": "Every row reached, the result's and its includes', by table then ID, each as the reader may see it."},
		},
	}
	schemas["Query"] = schema{
		"type":        "array",
		"minItems":    2,
		"prefixItems": []schema{{"const": "from"}, {"type": "string", "enum": tables}},
		"description": `The language written as JSON: each bracket a list, its operator's name first, as in ["from", "ALIAS", ["where", ["=", {"n": "alias"}, "spring-fair"]], ["columns", "target"]]. A string is a string value, except where only a name may stand: the table after from, exists, count and select (with its column, as "MEMBER.person"), sum's path and table, and everything in order, include and columns. {"n": "name"} is a name, a path or @name anywhere. A number is a number, and true and false are themselves.`,
	}
	schemas["Queries"] = schema{
		"type":     "object",
		"required": []string{"queries"},
		"properties": schema{"queries": schema{
			"type":                 "object",
			"description":          "Queries by name, each in the language as a string or as JSON, run together as one reader so they share what they work out.",
			"additionalProperties": schema{"oneOf": []schema{{"type": "string"}, component("Query")}},
		}},
	}
	schemas["Answers"] = schema{
		"type":     "object",
		"required": []string{"now", "results", "resources"},
		"properties": schema{
			"now":       schema{"type": "string", "description": "The school's time as the queries saw it."},
			"results":   schema{"type": "object", "additionalProperties": schema{"type": "array", "items": schema{"type": "string"}}, "description": "Each query's IDs, in order, by its name."},
			"resources": schema{"type": "object", "properties": resources, "description": "Every row any of them reached, once, by table then ID, with every column any of them answered."},
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
	schemas["SearchRef"] = schema{"type": "object", "required": []string{"extract", "document", "terminal", "name", "summary"}, "properties": schema{
		"extract":    schema{"type": "string", "description": "The extract that matched."},
		"document":   schema{"type": "string", "description": "The document its text was read from: an attachment, an image, a linked page, an email's body, or the email or file itself."},
		"source":     schema{"type": "string", "description": "How that document stands to its email or file, as the search input says it: attached file, email body, image shown in the email, linked from the email, with its type and any file name, link text and address. Blank when it is the email or file itself."},
		"href":       schema{"type": "string", "description": "That document's address, when it has one."},
		"first_page": schema{"type": "integer", "description": "When that document is a PDF read in runs of pages, the first of its pages the extract was read from."},
		"last_page":  schema{"type": "integer", "description": "When that document is a PDF read in runs of pages, the last of its pages the extract was read from."},
		"terminal":   schema{"type": "string", "description": "The email, shared file, year calendar or wiki page it belongs to."},
		"name":       schema{"type": "string", "description": "That email's or file's name."},
		"summary":    schema{"type": "string", "description": "The extract's summary."},
		"score":      schema{"type": "number", "description": "What it ranked by: 1 when its email's or file's name holds every word; else its closest chunk's cosine similarity to the words, at least the floor."},
	}}
	schemas["SearchHit"] = schema{"type": "object", "required": []string{"id", "name", "summary"}, "properties": schema{
		"id":      schema{"type": "string", "description": "The group or person."},
		"name":    schema{"type": "string"},
		"href":    schema{"type": "string", "description": "The page it has on the Helios apps, when it has one."},
		"summary": schema{"type": "string"},
		"score":   schema{"type": "number", "description": "What it ranked by: 1 when its name holds every word; else its closest chunk's cosine similarity to the words, at least the floor."},
	}}
	schemas["SearchBundle"] = schema{"type": "object", "required": []string{"refs"}, "properties": schema{
		"refs": schema{"type": "array", "items": schema{"$ref": "#/components/schemas/SearchRef"}, "description": "At most five matching extracts that belong together, best first: an email's or file's own, one per source however many extracts it was read in, and near-identical copies from emails or files that started no result of their own."},
	}}
	schemas["Sent"] = schema{"type": "object", "required": []string{"result", "sent"}, "properties": schema{
		"result": schema{"type": "array", "items": schema{"type": "string"}, "description": "The invitation MESSAGE written, for invite; empty otherwise."},
		"sent":   schema{"type": "integer", "description": "How many people mail was queued for."},
	}}
	schemas["Draft"] = schema{"type": "object", "properties": schema{
		"list":      schema{"type": "string", "pattern": idPattern(GroupPrefix), "description": "The email list the draft edits, when it is saved already; the caller must run it."},
		"name":      schema{"type": "string", "description": "The draft's name, for describe-group."},
		"ruleWords": schema{"type": "array", "items": schema{"type": "string"}, "description": "Each rule in words, for describe-group."},
		"rules": schema{"type": "array", "description": "RULE rows without a group, in order.", "items": schema{"type": "object", "properties": schema{
			"exclude": schema{"type": "boolean"}, "target": schema{"type": "string"}, "person": schema{"type": "string"},
			"search": schema{"type": "string"}, "replace_with": schema{"type": "string"}, "within": schema{"type": "string"},
		}}},
		"members": schema{"type": "array", "description": "People put on the draft or kept off it by hand.", "items": schema{"type": "object", "required": []string{"person", "member"}, "properties": schema{
			"person": schema{"type": "string", "pattern": idPattern(PersonPrefix)},
			"member": schema{"type": "string", "enum": []string{"yes", "excluded"}},
		}}},
	}}
	described := func(about string) schema {
		return object("Claude's description of "+about+".", []string{"description"}, schema{"description": schema{"type": "string"}})
	}
	claudeFailures := func(out schema) schema {
		out["429"] = failure("Too many Claude calls from this person this hour.")
		out["502"] = failure("Claude could not answer right now.")
		return refusals(out)
	}
	crop := schema{"type": "integer", "minimum": 0}
	paths := schema{
		"/api/q": schema{
			"query": schema{
				"tags":        []string{"read"},
				"summary":     "Run a query as the signed-in person",
				"operationId": "query",
				"requestBody": schema{"required": true, "content": schema{
					"text/plain":       schema{"schema": schema{"type": "string"}, "examples": examples},
					"application/json": schema{"schema": schema{"oneOf": []schema{component("Query"), component("Queries")}}},
				}},
				"responses": refusals(schema{
					"200": schema{"description": "The IDs the query answers and every row it reached; for several named queries, each one's IDs by name and every row once.", "content": schema{"application/json": schema{"schema": schema{"oneOf": []schema{component("Answer"), component("Answers")}}}}},
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
		"/api/do/photo": schema{
			"post": schema{
				"tags":    []string{"do"},
				"summary": "Add a photo of a person, a group or a wiki page",
				"requestBody": schema{"required": true, "content": schema{"multipart/form-data": schema{"schema": schema{
					"type": "object",
					"properties": schema{
						"person": schema{"type": "string", "pattern": idPattern(PersonPrefix)}, "group": schema{"type": "string", "pattern": idPattern(GroupPrefix)},
						"document":  schema{"type": "string", "pattern": idPattern(DocumentPrefix)},
						"photo":     schema{"type": "string", "contentMediaType": "application/octet-stream"},
						"stock":     schema{"type": "string", "pattern": idPattern(StockPrefix), "description": "A stock picture someone imported, in place of photo."},
						"crop_left": crop, "crop_top": crop, "crop_width": crop, "crop_height": crop,
					},
				}}}},
				"responses": refusals(schema{"200": answers("The new PHOTO and the original's hash.", "Stored"), "400": failure("Not an image, no picture imported for the stock ID, a bad crop box, or not exactly one of person, group and document.")}),
			},
		},
		"/api/do/flyer": schema{
			"post": schema{
				"tags":    []string{"do"},
				"summary": "Set a group's flyer",
				"requestBody": schema{"required": true, "content": schema{"multipart/form-data": schema{"schema": schema{
					"type":     "object",
					"required": []string{"group"},
					"properties": schema{
						"group": schema{"type": "string", "pattern": idPattern(GroupPrefix)},
						"flyer": schema{"type": "string", "contentMediaType": "application/octet-stream", "description": "Kept as sent in flyer_original; the server then makes flyer, a JPEG with every tag dropped, from it."},
						"stock": schema{"type": "string", "pattern": idPattern(StockPrefix), "description": "A stock picture someone imported, in place of flyer."},
					},
				}}}},
				"responses": refusals(schema{"200": answers("The GROUP and the flyer's hash.", "Stored"), "400": failure("Not an image, or no picture imported for the stock ID.")}),
			},
		},
		"/api/do/pronunciation": schema{
			"post": schema{
				"tags":    []string{"do"},
				"summary": "Record how a person's or a family's name is said",
				"requestBody": schema{"required": true, "content": schema{"multipart/form-data": schema{"schema": schema{
					"type":     "object",
					"required": []string{"recording"},
					"properties": schema{
						"person": schema{"type": "string", "pattern": idPattern(PersonPrefix)}, "group": schema{"type": "string", "pattern": idPattern(GroupPrefix)},
						"recording": schema{"type": "string", "contentMediaType": "audio/*", "description": "Typed by its part's Content-Type: WebM, MP4, MP3, Ogg or WAV audio. Kept as sent in pronunciation_original; the server then makes pronunciation, an AAC M4A, from it."},
					},
				}}}},
				"responses": refusals(schema{"200": answers("The PERSON or GROUP and the recording's hash.", "Stored"), "400": failure("Not a supported recording, or neither or both of person and group.")}),
			},
		},
		"/api/do/file": schema{
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
		"/api/do/fetched": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Fill a linked document still to fetch",
				"description": "Takes the body a fetch got for a linked document whose content is blank, which becomes the document's content if it is kept, or sets fetch to refused if not.",
				"requestBody": schema{"required": true, "content": schema{"multipart/form-data": schema{"schema": schema{
					"type":     "object",
					"required": []string{"document", "body"},
					"properties": schema{
						"document": schema{"type": "string", "pattern": idPattern(DocumentPrefix)},
						"body":     schema{"type": "string", "contentMediaType": "application/octet-stream"},
					},
				}}}},
				"responses": refusals(schema{"200": answers("The DOCUMENT, and the content's hash, or the fetch it was set to and why.", "Fetched"), "400": failure("Not a linked document still to fetch, or no body.")}),
			},
		},
		"/api/do/person-at": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Find who holds an address",
				"description": "Answers the person whose address it is, someone in the directory before a guest, whether or not the caller may read them, so a guest named by address is used rather than made again; blank when nobody holds it.",
				"requestBody": schema{"required": true, "content": schema{"application/json": schema{"schema": schema{
					"type":       "object",
					"required":   []string{"address"},
					"properties": schema{"address": schema{"type": "string", "format": "email"}},
				}}}},
				"responses": refusals(schema{
					"200": schema{"description": "The person, or blank.", "content": schema{"application/json": schema{"schema": schema{
						"type":       "object",
						"properties": schema{"person": schema{"type": "string"}},
					}}}},
				}),
			},
		},
		"/api/do/search": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Search groups, people and documents",
				"description": "Answers, for each of GROUP, PERSON and DOCUMENT, the rows the caller may read best first: first the rows whose name holds every word, then the rows an embedded chunk of which lies close enough in meaning to the words. A document result is a bundle of refs, best first: a hit joins the result its own email or file started, else the result holding a near-identical copy of it, else starts one. Equal scores come newest first.",
				"requestBody": schema{"required": true, "content": schema{"application/json": schema{"schema": schema{
					"type":     "object",
					"required": []string{"words"},
					"properties": schema{
						"words": schema{"type": "string"},
						"limits": schema{
							"type":                 "object",
							"description":          "How many results to answer for each table, 0 to leave a table out; a table not named gets 10.",
							"properties":           schema{"GROUP": schema{"type": "integer", "minimum": 0}, "PERSON": schema{"type": "integer", "minimum": 0}, "DOCUMENT": schema{"type": "integer", "minimum": 0}},
							"additionalProperties": false,
						},
					},
				}}}},
				"responses": refusals(schema{
					"200": schema{"description": "The hits by table.", "content": schema{"application/json": schema{"schema": schema{
						"type": "object",
						"properties": schema{
							"GROUP":    schema{"type": "array", "items": component("SearchHit")},
							"PERSON":   schema{"type": "array", "items": component("SearchHit")},
							"DOCUMENT": schema{"type": "array", "items": component("SearchBundle")},
						},
					}}}},
					"400": failure("No words, or a limit naming a table search does not answer, or under 0."),
				}),
			},
		},
		"/api/do/compose": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Write a query from a description",
				"description": "For anyone in the directory, counted against their hourly Claude allowance. Asks Claude for a query in the language answering the words, and answers what it understood them to ask for and the query, blank when it found none. Nothing is run.",
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
		"/api/do/subscription": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Subscribe the caller to an email list's mail, or unsubscribe them",
				"description": "For someone in the group or already unsubscribed from it. Unsubscribing puts the caller in the list's unsubscribed group, made at the first unsubscribe; subscribing takes them out. They stay a member either way. Written as the mailer.",
				"requestBody": jsonBody([]string{"group"}, schema{
					"group":      schema{"type": "string", "pattern": idPattern(GroupPrefix)},
					"subscribed": schema{"type": "boolean", "description": "true to get the list's mail again, false or absent to stop it."},
				}),
				"responses": refusals(schema{
					"200": answers("The group.", "Written"),
					"400": failure("A body that is not JSON."),
					"404": failure("No open group the caller is in or unsubscribed from."),
				}),
			},
		},
		"/api/do/wiki": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Save a wiki page and its side cards",
				"description": "Stores each body as markdown content and writes the page's DOCUMENT, a new one when document is blank, with its side cards as child DOCUMENTs in the order sent. When sides is sent, it is the whole set: a side card with no id is new and one left out is removed; without it, the side cards stay as they are.",
				"requestBody": jsonBody([]string{"name"}, schema{
					"document": schema{"type": "string", "pattern": "^$|" + idPattern(DocumentPrefix), "description": "The page to save over, blank for a new page."},
					"parent":   schema{"type": "string", "pattern": "^$|" + idPattern(DocumentPrefix), "description": "The page it sits under, blank for the top."},
					"slug":     schema{"type": "string"},
					"name":     schema{"type": "string"},
					"body":     schema{"type": "string", "description": "Markdown, at most " + strconv.Itoa(wikiLimit) + " bytes."},
					"sides": schema{"type": "array", "items": schema{"type": "object", "required": []string{"name"}, "properties": schema{
						"id":   schema{"type": "string", "pattern": "^$|" + idPattern(DocumentPrefix)},
						"name": schema{"type": "string"},
						"body": schema{"type": "string"},
					}}},
				}),
				"responses": refusals(schema{
					"200": answers("The page's DOCUMENT and its body's hash.", "Stored"),
					"400": failure("No title, a body too long, a bad slug or parent, or a body that is not JSON."),
				}),
			},
		},
		"/api/do/draft-members": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Work out whom an unsaved email list would hold",
				"description": "Answers the draft's effective members, each with why, and how many people each rule picks out. Writes nothing. Refused unless the caller may see the members of every group a rule names and every person named, and runs list when one is given.",
				"requestBody": schema{"required": true, "content": schema{"application/json": schema{"schema": component("Draft")}}},
				"responses": refusals(schema{
					"200": object("Who the draft holds.", []string{"members", "ruleCounts"}, schema{
						"members": schema{"type": "array", "items": schema{"type": "object", "required": []string{"person", "reasons"}, "properties": schema{
							"person":  schema{"type": "string"},
							"name":    schema{"type": "string", "description": "Their name, when the caller may see them."},
							"reasons": schema{"type": "array", "items": schema{"type": "object", "properties": schema{"rule": schema{"type": "integer", "description": "An index into the rules sent."}, "added": schema{"type": "boolean"}}}},
						}}},
						"ruleCounts": schema{"type": "array", "items": schema{"type": "integer"}, "description": "How many people each rule picks out, in the rules' order."},
					}),
					"400": failure("A member that is neither yes nor excluded, or a body that is not JSON."),
				}),
			},
		},
		"/api/do/describe-group": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Have Claude describe an unsaved email list",
				"description": "Takes the same draft as draft-members, with its name and its rules in words, checked the same way, and answers a description Claude writes from them and from a tally of who the draft holds by role, grade and classroom.",
				"requestBody": schema{"required": true, "content": schema{"application/json": schema{"schema": component("Draft")}}},
				"responses":   claudeFailures(schema{"200": described("the list"), "400": failure("A description too long, or a body that is not JSON.")}),
			},
		},
		"/api/do/describe-activity": schema{
			"post": schema{
				"tags":    []string{"do"},
				"summary": "Have Claude describe a volunteer activity being written",
				"requestBody": jsonBody([]string{}, schema{
					"title": schema{"type": "string"}, "parent": schema{"type": "string"}, "category": schema{"type": "string"},
					"when": schema{"type": "string"}, "notes": schema{"type": "string"},
				}),
				"responses": claudeFailures(schema{"200": described("the activity"), "400": failure("A description too long, or a body that is not JSON.")}),
			},
		},
		"/api/do/describe-charity": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Have Claude look a charity up",
				"description": "For the Birthdays team alone (birthday_team). Answers a sentence on the charity and its donation link.",
				"requestBody": jsonBody([]string{"name"}, schema{"name": schema{"type": "string"}, "url": schema{"type": "string", "description": "The charity's site, when known."}}),
				"responses": claudeFailures(schema{
					"200": object("The charity's sentence and donation link.", []string{"description", "url"}, schema{"description": schema{"type": "string"}, "url": schema{"type": "string"}}),
					"400": failure("No name, or a body that is not JSON."),
					"404": failure("Claude found no such charity."),
				}),
			},
		},
		"/api/do/invite": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Send a guest list's invitation to people waiting for it",
				"description": "For an event, party or activity the caller can see. The hosts (runs_list) send it to anyone waiting, or to everyone waiting when people is empty, and may mark it sent without mailing it (skip); anyone else sends it only to people they brought.",
				"requestBody": jsonBody([]string{"group"}, schema{
					"group":  schema{"type": "string", "pattern": idPattern(GroupPrefix)},
					"people": schema{"type": "array", "items": schema{"type": "string", "pattern": idPattern(PersonPrefix)}},
					"skip":   schema{"type": "boolean"},
				}),
				"responses": refusals(schema{
					"200": answers("The invitation MESSAGE and how many it went to.", "Sent"),
					"400": failure("Someone not waiting for the invitation, or a body that is not JSON."),
					"404": failure("No event, party or activity the caller can see."),
				}),
			},
		},
		"/api/do/guest-mail": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Mail a guest list",
				"description": "For the hosts (runs_list). reminder and message go to the people invited whose answer is in to (yes, no, or none for no answer) or who are named in people; update goes to everyone invited but those who said no; cancel sets the group's status to cancelled and, with notify, mails everyone invited.",
				"requestBody": jsonBody([]string{"group", "kind"}, schema{
					"group":   schema{"type": "string", "pattern": idPattern(GroupPrefix)},
					"kind":    schema{"type": "string", "enum": []string{"reminder", "message", "update", "cancel"}},
					"to":      schema{"type": "array", "items": schema{"type": "string", "enum": []string{"yes", "no", "none"}}},
					"people":  schema{"type": "array", "items": schema{"type": "string", "pattern": idPattern(PersonPrefix)}},
					"subject": schema{"type": "string"},
					"text":    schema{"type": "string", "maxLength": maxGuestWords},
					"notify":  schema{"type": "boolean"},
				}),
				"responses": refusals(schema{
					"200": answers("How many it went to.", "Sent"),
					"400": failure("An unknown kind, words too long, a reminder or message with no subject or words, or a body that is not JSON."),
					"404": failure("No event, party or activity the caller can see."),
				}),
			},
		},
		"/api/do/add-guests": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Add people to a guest list",
				"description": "For the hosts (runs_list), or anyone when the group's join is others and they are eligible. Each person is someone the caller can see, by ID, or someone from outside, by name and address: the person already holding the address, else a new guest. With family, two or more new guests become one family. Anyone added who has an address and no invitation yet is sent one, always for someone not a host and with send for a host.",
				"requestBody": jsonBody([]string{"group", "people"}, schema{
					"group": schema{"type": "string", "pattern": idPattern(GroupPrefix)},
					"people": schema{"type": "array", "items": schema{"type": "object", "properties": schema{
						"person": schema{"type": "string", "pattern": "^$|" + idPattern(PersonPrefix)},
						"name":   schema{"type": "string"},
						"email":  schema{"type": "string", "format": "email"},
					}}},
					"guest_of": schema{"type": "string", "pattern": "^$|" + idPattern(PersonPrefix), "description": "Who brings them: the caller or someone in their household, unless the caller is a host."},
					"family":   schema{"type": "boolean"},
					"answer":   schema{"type": "string", "enum": []string{"", "yes"}, "description": "yes to answer yes for everyone added."},
					"send":     schema{"type": "boolean"},
				}),
				"responses": refusals(schema{
					"200": answers("How many invitations were sent.", "Sent"),
					"400": failure("No one named, an address that is not one, an answer other than yes, or a body that is not JSON."),
					"404": failure("No event, party or activity the caller can see, or a person they can't."),
				}),
			},
		},
		"/api/do/report-draft": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "Draft a GitHub issue from a report",
				"description": "For super admins alone. Writes nothing.",
				"requestBody": jsonBody([]string{"report"}, schema{"report": schema{"type": "string", "pattern": idPattern(prefixOf("REPORT"))}}),
				"responses": refusals(schema{
					"200": object("The draft issue.", []string{"title", "body", "type", "labels", "repo", "filer"}, schema{
						"title": schema{"type": "string"}, "body": schema{"type": "string"}, "type": schema{"type": "string"},
						"labels": schema{"type": "array", "items": schema{"type": "string"}},
						"repo":   schema{"type": "string"},
						"filer":  schema{"type": "boolean", "description": "Whether this server can file on GitHub."},
					}),
					"400": failure("A body that is not JSON."),
					"404": failure("No such report."),
				}),
			},
		},
		"/api/do/file-report": schema{
			"post": schema{
				"tags":        []string{"do"},
				"summary":     "File a report as a GitHub issue",
				"description": "For super admins alone. Files the issue as given and marks the report filed.",
				"requestBody": jsonBody([]string{"report", "title", "type"}, schema{
					"report": schema{"type": "string", "pattern": idPattern(prefixOf("REPORT"))},
					"title":  schema{"type": "string"}, "body": schema{"type": "string"}, "type": schema{"type": "string"},
					"labels": schema{"type": "array", "items": schema{"type": "string"}},
				}),
				"responses": refusals(schema{
					"200": object("The issue filed.", []string{"issue"}, schema{"issue": schema{"type": "string"}}),
					"400": failure("No title or type, too long, an email address left in, or a body that is not JSON."),
					"404": failure("No such report."),
					"409": failure("The report is filed already."),
					"502": failure("GitHub would not take the issue."),
					"503": failure("Filing on GitHub is not set up on this server."),
				}),
			},
		},
	}
	return schema{
		"openapi": "3.2.0",
		"info":    schema{"title": "Heliosian data model", "version": "1"},
		"tags": []schema{
			{"name": "read", "description": "The query language, as text or as JSON."},
			{"name": "write", "description": "Batches of inserts, sets and deletes."},
			{"name": "do", "description": "Calls that are more than writing rows."},
		},
		"security":   []schema{{}, {"token": []string{}}},
		"paths":      paths,
		"components": schema{"schemas": schemas, "securitySchemes": schema{"token": schema{"type": "http", "scheme": "bearer", "description": "An access token from the OAuth server the MCP host runs, to run as the person it names; Helios-Mode and Helios-Spoof as for a session."}}},
	}
}

func openapi(w http.ResponseWriter, r *http.Request) {
	serve.Write(w, r, http.StatusOK, spec())
}
