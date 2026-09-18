package ask

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type passage struct {
	Key       string  `json:"key"`
	Title     string  `json:"title"`
	Date      string  `json:"date"`
	Published string  `json:"published"`
	URL       string  `json:"url,omitempty"`
	Group     string  `json:"group,omitempty"`
	Section   string  `json:"section,omitempty"`
	Text      string  `json:"text"`
	Score     float64 `json:"score"`
}

var searchDocuments = tool{
	name:        "search_documents",
	description: "Search the community's documents for the passages that best answer a question: what was announced, asked for, explained, decided or celebrated, how the school describes itself and its program, and when. They include the mail of the Helios Loop groups the viewer is on or manages. Each passage names its document, its date and how long ago that was, the document's url when it has one, and for a group's mail the group it went to. This is the community's memory, so it is the place to look for anything the apps do not hold: how something was done before, what a tradition is, what was said about a topic. A newer document supersedes an older one. For anything happening now the apps are usually the better word, but a recent document can override them: a last-minute reminder, a changed time or place, a cancellation.",
	words:       "Searching documents",
	properties: map[string]any{
		"query": str("What to look for, in plain words: a topic, an event, a name, a request."),
		"since": str("Only documents dated on or after this date, as 2024-08-01."),
		"until": str("Only documents dated on or before this date, as 2025-06-30."),
		"limit": integer("How many passages to return, 8 unless said, 20 at most."),
	},
	required: []string{"query"},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			Query, Since, Until string
			Limit               int
		}](input)
		if err != nil {
			return nil, err
		}
		query := strings.TrimSpace(in.Query)
		if query == "" {
			return nil, fmt.Errorf("say what to look for")
		}
		model := v.documents()
		oldest, newest := model.Span()
		out := map[string]any{
			"today": v.now.Format("2006-01-02"), "documents": len(model.Documents),
			"oldest": oldest, "newest": newest, "passages": []passage{},
		}
		if len(model.Documents) == 0 {
			return out, nil
		}
		since, until := strings.TrimSpace(in.Since), strings.TrimSpace(in.Until)
		for _, cell := range []string{since, until} {
			if cell != "" {
				if _, err := date(cell); err != nil {
					return nil, err
				}
			}
		}
		if model = model.Between(since, until); len(model.Documents) == 0 {
			return nil, fmt.Errorf("no documents fall in that range; the oldest is %s and the newest %s", oldest, newest)
		}
		vectors, err := v.embedder.Embed(v.ctx, []string{query}, true)
		if err != nil {
			return nil, fmt.Errorf("the search is not answering right now: %w", err)
		}
		passages := []passage{}
		for _, hit := range model.Search(vectors[0], query, limitOf(in.Limit, 8, 20)) {
			d, c := hit.Document, hit.Document.Chunks[hit.Index]
			passages = append(passages, passage{
				Key: d.Key, Title: d.Title, Date: d.Date, Published: v.timing(d.Date, ""), URL: d.URL(), Group: v.groupOf(d),
				Section: c.Section, Text: c.Text, Score: math.Round(hit.Score*1000) / 1000,
			})
		}
		out["searched"] = len(model.Documents)
		out["passages"] = passages
		return out, nil
	},
}

var readDocument = tool{
	name:        "read_document",
	description: "One document in full by its key, as search_documents gave it: the whole thing as markdown, with its title, date and url when it has one. Read one when a passage is not enough.",
	words:       "Reading a document",
	properties: map[string]any{
		"key": str("The document's key, from search_documents."),
	},
	required: []string{"key"},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Key string }](input)
		if err != nil {
			return nil, err
		}
		d := v.documents().Document(strings.TrimSpace(in.Key))
		if d == nil {
			return nil, fmt.Errorf("no document has that key")
		}
		out := map[string]any{
			"key": d.Key, "title": d.Title, "date": d.Date, "published": v.timing(d.Date, ""), "markdown": d.Markdown,
		}
		if url := d.URL(); url != "" {
			out["url"] = url
		}
		if group := v.groupOf(d); group != "" {
			out["group"] = group
		}
		return out, nil
	},
}
