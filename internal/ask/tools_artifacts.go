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
	Kind      string  `json:"kind"`
	Channel   string  `json:"channel,omitempty"`
	Author    string  `json:"author,omitempty"`
	Section   string  `json:"section,omitempty"`
	Text      string  `json:"text"`
	Score     float64 `json:"score"`
}

var searchDocuments = tool{
	name:        "search_documents",
	description: "Search the community's own mail on file - the school's weekly newsletters, the announcements that went to every family, and what each classroom's parent list carried - for the passages that best answer a question: what was announced, asked for, explained, decided or celebrated, and when. Each passage names the message it came from, its date and how long ago that was, the channel it went out on and who wrote it. This is the community's history, so it is the place to look for anything the apps do not hold: how something was done before, what a tradition is, what was said about a topic. A newer message supersedes an older one; always say which message and date an answer came from, and prefer what the apps say for anything happening now.",
	words:       "Searching documents",
	properties: map[string]any{
		"query":   str("What to look for, in plain words: a topic, an event, a name, a request."),
		"channel": str("Only this channel, as a passage names it: parentsandstaff, chat, newsletter, jays.parents and the like."),
		"since":   str("Only messages on or after this date, as 2024-08-01."),
		"until":   str("Only messages on or before this date, as 2025-06-30."),
		"limit":   integer("How many passages to return, 8 unless said, 20 at most."),
	},
	required: []string{"query"},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			Query, Channel, Since, Until string
			Limit                        int
		}](input)
		if err != nil {
			return nil, err
		}
		query := strings.TrimSpace(in.Query)
		if query == "" {
			return nil, fmt.Errorf("say what to look for")
		}
		model := v.artifacts
		oldest, newest := model.Span()
		out := map[string]any{
			"today": v.now.Format("2006-01-02"), "documents": len(model.Documents),
			"oldest": oldest, "newest": newest, "passages": []passage{},
		}
		if len(model.Documents) == 0 {
			return out, nil
		}
		if channel := strings.TrimSpace(in.Channel); channel != "" {
			if model = model.InChannel(channel); len(model.Documents) == 0 {
				return nil, fmt.Errorf("no messages came through a channel called %q; the channels are %s", channel, strings.Join(v.artifacts.ChannelNames(), ", "))
			}
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
			return nil, fmt.Errorf("no messages on file fall in that range; the oldest is %s and the newest %s", oldest, newest)
		}
		vectors, err := v.embedder.Embed(v.ctx, []string{query}, true)
		if err != nil {
			return nil, fmt.Errorf("the search is not answering right now: %w", err)
		}
		passages := []passage{}
		for _, hit := range model.Search(vectors[0], query, limitOf(in.Limit, 8, 20)) {
			d, c := hit.Document, hit.Document.Chunks[hit.Index]
			passages = append(passages, passage{
				Key: d.Key, Title: d.Title, Date: d.Date, Published: v.timing(d.Date, ""), Kind: d.Kind,
				Channel: d.Channel, Author: d.Author, Section: c.Section, Text: c.Text,
				Score: math.Round(hit.Score*1000) / 1000,
			})
		}
		out["searched"] = len(model.Documents)
		out["passages"] = passages
		return out, nil
	},
}

var readDocument = tool{
	name:        "read_document",
	description: "One message in full by its key, as search_documents gave it: the whole thing as markdown, with its title, date, author and channel. Read one when a passage is not enough - the rest of a newsletter, or the whole of an announcement.",
	words:       "Reading a document",
	properties: map[string]any{
		"key": str("The message's key, from search_documents."),
	},
	required: []string{"key"},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Key string }](input)
		if err != nil {
			return nil, err
		}
		d := v.artifacts.Document(strings.TrimSpace(in.Key))
		if d == nil {
			return nil, fmt.Errorf("no message on file has that key")
		}
		return map[string]any{
			"key": d.Key, "title": d.Title, "date": d.Date, "published": v.timing(d.Date, ""),
			"author": d.Author, "kind": d.Kind, "channel": d.Channel, "markdown": d.Markdown,
		}, nil
	},
}
