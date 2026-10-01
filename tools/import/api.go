package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

type client struct {
	base string
	key  string
}

type answer struct {
	Now       string                    `json:"now"`
	Result    []string                  `json:"result"`
	Resources map[string]map[string]row `json:"resources"`
}

func (c client) send(method, path, kind string, body io.Reader, out any) error {
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", kind)
	req.Header.Set("Authorization", "Bearer "+c.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(got)))
	}
	return json.Unmarshal(got, out)
}

func (c client) q(method string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return c.send(method, "/api/q", "application/json", bytes.NewReader(raw), out)
}

func (c client) table(name string, where ...any) (table, string, error) {
	q := map[string]any{"from": name}
	if len(where) > 0 {
		q["where"] = where
	}
	var a answer
	if err := c.q("QUERY", q, &a); err != nil {
		return table{}, "", err
	}
	t := table{rows: map[string]row{}}
	for _, id := range a.Result {
		t.add(id, a.Resources[name][id])
	}
	return t, a.Now, nil
}

func in(path string, values ...any) map[string]any {
	return map[string]any{"in": append([]any{map[string]any{"path": path}}, values...)}
}

func (c client) read() (*state, error) {
	st := &state{}
	var err error
	if st.people, st.now, err = c.table("PERSON"); err != nil {
		return nil, err
	}
	if st.emails, _, err = c.table("PERSON_EMAIL"); err != nil {
		return nil, err
	}
	if st.photos, _, err = c.table("PERSON_PHOTO"); err != nil {
		return nil, err
	}
	if st.groups, _, err = c.table("GROUP", in("kind", "family", "role", "classroom", "crew", "grade", "band")); err != nil {
		return nil, err
	}
	if st.members, _, err = c.table("MEMBER", in("group.kind", "family", "role")); err != nil {
		return nil, err
	}
	return st, nil
}

func (c client) write(batch []write) ([]string, error) {
	var out struct {
		Result []string `json:"result"`
	}
	if err := c.q(http.MethodPost, map[string]any{"batch": batch}, &out); err != nil {
		return nil, err
	}
	return out.Result, nil
}

func (c client) addPhoto(p portrait) error {
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	if err := form.WriteField("person", p.person); err != nil {
		return err
	}
	part, err := form.CreateFormFile("photo", p.name)
	if err != nil {
		return err
	}
	if _, err := part.Write(p.content); err != nil {
		return err
	}
	if err := form.Close(); err != nil {
		return err
	}
	var out struct {
		Result []string `json:"result"`
	}
	return c.send(http.MethodPost, "/api/do/person-photo", form.FormDataContentType(), body, &out)
}
