package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"heliosian/internal/blob"
)

type classroomUpdate struct {
	Classroom string `json:"classroom"`
	DeckURL   string `json:"deck_url"`
	Title     string `json:"title"`
	TextFile  string `json:"text_file"`
	text      []byte
}

type updateUpload struct {
	Classroom  string   `json:"classroom"`
	URL        string   `json:"url"`
	Name       string   `json:"name"`
	Hash       string   `json:"hash"`
	SentTo     []string `json:"sent_to"`
	Supersedes string   `json:"supersedes,omitempty"`
	text       []byte
}

func readUpdates(dir string) ([]classroomUpdate, error) {
	manifest, err := os.ReadFile(filepath.Join(dir, "classroom-updates.json"))
	if err != nil {
		return nil, err
	}
	updates := []classroomUpdate{}
	if err := json.Unmarshal(manifest, &updates); err != nil {
		return nil, err
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("classroom updates manifest is empty")
	}
	seen := map[string]bool{}
	for i := range updates {
		u := &updates[i]
		if u.Classroom == "" || u.DeckURL == "" || u.Title == "" || u.TextFile == "" {
			return nil, fmt.Errorf("classroom update %d is missing its classroom, deck url, title or text file", i+1)
		}
		if seen[u.DeckURL] {
			return nil, fmt.Errorf("duplicate classroom deck url %s", u.DeckURL)
		}
		seen[u.DeckURL] = true
		if !filepath.IsLocal(u.TextFile) {
			return nil, fmt.Errorf("classroom update text file is not relative to the export: %s", u.TextFile)
		}
		u.text, err = os.ReadFile(filepath.Join(dir, u.TextFile))
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(u.text)) == 0 {
			return nil, fmt.Errorf("classroom update for %s has no text", u.Classroom)
		}
	}
	return updates, nil
}

func planUpdates(updates []classroomUpdate, groups, documents, contents table) ([]updateUpload, error) {
	uploads := []updateUpload{}
	for _, u := range updates {
		classroom := ""
		slug := ""
		for id, g := range groups.rows {
			if g["kind"] == "classroom" && strings.EqualFold(g["name"], u.Classroom) {
				if classroom != "" {
					return nil, fmt.Errorf("multiple classrooms named %s", u.Classroom)
				}
				classroom, slug = id, g["slug"]
			}
		}
		if classroom == "" || slug == "" {
			return nil, fmt.Errorf("no classroom with a slug named %s", u.Classroom)
		}
		parents := ""
		for id, g := range groups.rows {
			if g["kind"] == "group" && g["slug"] == slug+"-parents" {
				parents = id
			}
		}
		if parents == "" {
			return nil, fmt.Errorf("no parents group for classroom %s", u.Classroom)
		}
		current := ""
		for id, d := range documents.rows {
			if d["kind"] == "file" && d["parent"] == "" && d["url"] == u.DeckURL && d["superseded_by"] == "" {
				if current != "" {
					return nil, fmt.Errorf("multiple current documents for deck %s", u.DeckURL)
				}
				current = id
			}
		}
		hash := blob.Hash(u.text)
		if current != "" {
			content := contents.rows[documents.rows[current]["content"]]
			if content["mime"] == "text/markdown" && content["hash"] == hash {
				continue
			}
		}
		uploads = append(uploads, updateUpload{Classroom: u.Classroom, URL: u.DeckURL, Name: u.Title, Hash: hash, SentTo: []string{classroom, parents}, Supersedes: current, text: u.text})
	}
	return uploads, nil
}

func (c client) updatePlan(updates []classroomUpdate, groups table) ([]updateUpload, error) {
	urls := []any{}
	for _, update := range updates {
		urls = append(urls, update.DeckURL)
	}
	tables := []table{}
	for _, query := range [][]any{
		{"from", "DOCUMENT", []any{"where", in("kind", "file"), in("url", urls...)}, []any{"columns", "kind", "parent", "url", "content", "superseded_by"}},
		{"from", "CONTENT", []any{"where", in("mime", "text/markdown"), []any{"in", name("id"), []any{"select", "DOCUMENT.content", in("kind", "file"), in("url", urls...)}}}, []any{"columns", "hash", "mime"}},
	} {
		answer, err := c.Query(query)
		if err != nil {
			return nil, err
		}
		t := table{rows: map[string]row{}}
		for _, id := range answer.Result {
			t.add(id, answer.Resources[query[1].(string)][id])
		}
		tables = append(tables, t)
	}
	return planUpdates(updates, groups, tables[0], tables[1])
}

func (c client) addUpdate(u updateUpload) error {
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	for field, value := range map[string]string{"name": u.Name, "url": u.URL, "supersedes": u.Supersedes} {
		if err := form.WriteField(field, value); err != nil {
			return err
		}
	}
	for _, group := range u.SentTo {
		if err := form.WriteField("sent_to", group); err != nil {
			return err
		}
	}
	part, err := form.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="updates.md"`},
		"Content-Type":        {"text/markdown"},
	})
	if err != nil {
		return err
	}
	if _, err := part.Write(u.text); err != nil {
		return err
	}
	if err := form.Close(); err != nil {
		return err
	}
	var out struct {
		Result []string `json:"result"`
	}
	if err := c.Send(http.MethodPost, "/api/do/file", form.FormDataContentType(), body, &out); err != nil {
		return err
	}
	if len(out.Result) != 1 {
		return fmt.Errorf("uploading %s returned %d documents", u.Classroom, len(out.Result))
	}
	slog.Info("classroom update filed", "classroom", u.Classroom, "document", out.Result[0])
	return nil
}
