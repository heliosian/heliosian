package model

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/id"
)

const (
	kindDocument = "document"
	kindPassage  = "document-passage"
	searchDepth  = 100
)

type documentPassage struct {
	doc   *Document
	chunk int
}

type documentResource struct {
	Title     string `json:"title"`
	Date      string `json:"date"`
	Kind      string `json:"kind"`
	URL       string `json:"url,omitempty"`
	EmailList string `json:"emailList,omitempty"`
	Markdown  string `json:"markdown"`
}

type passageResource struct {
	Section string `json:"section,omitempty"`
	Text    string `json:"text"`
}

func (m *Documents) index(key []byte) {
	m.ids, m.idOf, m.passages, m.passageIDs = map[string]*Document{}, map[*Document]string{}, map[string]documentPassage{}, map[documentPassage]string{}
	for _, d := range m.Documents {
		docID := id.Of(key, kindDocument, d.Key)
		m.ids[docID], m.idOf[d] = d, docID
		for i := range d.Chunks {
			p := documentPassage{doc: d, chunk: i}
			passageID := id.Of(key, kindPassage, d.Key+"\x00"+strconv.Itoa(i))
			m.passages[passageID], m.passageIDs[p] = p, passageID
		}
	}
}

func (m *Model) readsDocument(q api.Query, d *Document) bool {
	if d.Kind != DocumentKindGroup {
		return true
	}
	s := m.scope
	s.mailOnce.Do(func() {
		s.mailReadable = map[string]bool{}
		sources := m.listSources()
		for _, g := range m.EmailLists.Groups {
			if g.MailReadableBy(q.Actor.Email, sources) {
				s.mailReadable[g.Name] = true
			}
		}
	})
	return s.mailReadable[d.Channel]
}

func (m *Model) readableDocument(q api.Query, key string) *Document {
	d := m.Documents.ids[key]
	if d == nil || !m.readsDocument(q, d) {
		return nil
	}
	return d
}

func (m *Model) emailListWords(d *Document) string {
	if d.Kind != DocumentKindGroup {
		return ""
	}
	g := m.EmailLists.Named(d.Channel)
	if g == nil {
		return d.Channel
	}
	return g.Title + " (" + g.Address() + ")"
}

func documentDated(name string, keep func(date, value string) bool, of func(m *Model, key string) *Document) api.Filter[*Model] {
	return func(m *Model, _ api.Query, value string) (func(string) bool, error) {
		value, err := dateParam(name, strings.TrimSpace(value))
		if err != nil {
			return nil, err
		}
		return func(key string) bool {
			d := of(m, key)
			return d != nil && keep(d.Date, value)
		}, nil
	}
}

func datedFilters(of func(m *Model, key string) *Document) map[string]api.Filter[*Model] {
	return map[string]api.Filter[*Model]{
		"since": documentDated("since", func(date, value string) bool { return date >= value }, of),
		"until": documentDated("until", func(date, value string) bool { return date <= value }, of),
	}
}

func documentOf(m *Model, key string) *Document {
	return m.Documents.ids[key]
}

func passageDocument(m *Model, key string) *Document {
	return m.Documents.passages[key].doc
}

func DocumentResources(filer *DocumentFiler) []api.Type[*Model] {
	documents := api.Type[*Model]{
		Name:  "documents",
		Shape: documentResource{},
		Has:   func(m *Model, key string) bool { return m.Documents.ids[key] != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			d := m.readableDocument(q, key)
			if d == nil {
				return nil, false
			}
			return documentResource{Title: d.Title, Date: d.Date, Kind: d.Kind, URL: d.URL(), EmailList: m.emailListWords(d), Markdown: d.Markdown}, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, d := range m.Documents.Documents {
				if m.readsDocument(q, d) {
					out = append(out, m.Documents.idOf[d])
				}
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, d := range m.Documents.Documents {
				out[d.Key] = m.Documents.idOf[d]
			}
			return out
		},
		Filters: datedFilters(documentOf),
	}
	passages := api.Type[*Model]{
		Name:  "document-passages",
		Shape: passageResource{},
		Has:   func(m *Model, key string) bool { _, ok := m.Documents.passages[key]; return ok },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			p, ok := m.Documents.passages[key]
			if !ok || !m.readsDocument(q, p.doc) {
				return nil, false
			}
			c := p.doc.Chunks[p.chunk]
			return passageResource{Section: c.Section, Text: c.Text}, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, d := range m.Documents.Documents {
				if !m.readsDocument(q, d) {
					continue
				}
				for i := range d.Chunks {
					out = append(out, m.Documents.passageIDs[documentPassage{doc: d, chunk: i}])
				}
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"document": {Type: "documents", List: func(m *Model, q api.Query, key string) []string {
				p, ok := m.Documents.passages[key]
				if !ok || !m.readsDocument(q, p.doc) {
					return nil
				}
				return []string{m.Documents.idOf[p.doc]}
			}},
		},
		Filters: datedFilters(passageDocument),
		Rankers: map[string]api.Ranker[*Model]{
			"search": func(m *Model, q api.Query, value string, keep func(string) bool) ([]api.Hit, error) {
				value = strings.TrimSpace(value)
				if value == "" {
					return nil, access.Invalid("say what to search for")
				}
				vectors, err := filer.embedder.Embed(q.Context, []string{value}, true)
				if err != nil {
					return nil, fmt.Errorf("embed the search: %w", err)
				}
				docs := m.Documents
				hits := []api.Hit{}
				for _, hit := range docs.Search(vectors[0], value, searchDepth, func(d *Document, chunk int) bool {
					return m.readsDocument(q, d) && keep(docs.passageIDs[documentPassage{doc: d, chunk: chunk}])
				}) {
					hits = append(hits, api.Hit{ID: docs.passageIDs[documentPassage{doc: hit.Document, chunk: hit.Index}], Score: math.Round(hit.Score*1000) / 1000})
				}
				return hits, nil
			},
		},
	}
	return []api.Type[*Model]{documents, passages}
}
