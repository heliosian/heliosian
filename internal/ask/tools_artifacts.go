package ask

import (
	"encoding/json"
	"fmt"
	"strconv"
)

var searchDocuments = tool{
	name:        "search_documents",
	description: "Search the community's documents for the passages that best answer a question: what was announced, asked for, explained, decided or celebrated, how the school describes itself and its program, and when. They include the mail of the Helios Loop email lists the viewer is on or manages. Each passage names its document with the document's id for read_document, its date, its url when it has one, and for an email list's mail the email list it went to. This is the community's memory, so it is the place to look for anything the apps do not hold: how something was done before, what a tradition is, what was said about a topic. A newer document supersedes an older one. For anything happening now the apps are usually the better word, but a recent document can override them: a last-minute reminder, a changed time or place, a cancellation.",
	words:       "Searching documents",
	properties: map[string]any{
		"query": str("What to look for, in plain words: a topic, an event, a name, a request."),
		"since": str("Only documents dated on or after this date, as 2024-08-01."),
		"until": str("Only documents dated on or before this date, as 2025-06-30."),
		"limit": integer("How many passages to return, 8 unless said, 20 at most."),
	},
	required: []string{"query"},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			Query, Since, Until string
			Limit               int
		}](input)
		if err != nil {
			return nil, err
		}
		if in.Query == "" {
			return nil, fmt.Errorf("say what to look for")
		}
		v := params("search", in.Query, "since", in.Since, "until", in.Until, "include", "document", "limit", strconv.Itoa(limitOf(in.Limit, 8, 20)))
		spec := "section,text,document.id,document.title,document.date,document.url,document.emailList"
		return t.ask(query{name: "passages", path: collection("document-passages", v), fields: fields(spec), scoreAs: "score"})
	},
}

var readDocument = tool{
	name:        "read_document",
	description: "One document in full by its id, as search_documents or the list of recent documents gave it: the whole thing as markdown, with its title, date and url when it has one. Read one when a passage is not enough.",
	words:       "Reading a document",
	properties: map[string]any{
		"id": str("The document's id."),
	},
	required: []string{"id"},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ ID string }](input)
		if err != nil {
			return nil, err
		}
		if in.ID == "" {
			return nil, fmt.Errorf("say which document")
		}
		return t.ask(query{name: "document", path: one("documents", in.ID, params()), fields: fields("title,date,url,emailList,markdown")})
	},
}
