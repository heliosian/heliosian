package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type client struct {
	url string
	key string
}

type answer struct {
	Now       string                    `json:"now"`
	Result    []string                  `json:"result"`
	Resources map[string]map[string]row `json:"resources"`
}

func (c client) do(method string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, c.url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
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
		return fmt.Errorf("%s %s: %s: %s", method, c.url, resp.Status, strings.TrimSpace(string(got)))
	}
	return json.Unmarshal(got, out)
}

func (c client) table(name string, where ...any) (table, string, error) {
	q := map[string]any{"from": name}
	if len(where) > 0 {
		q["where"] = where
	}
	var a answer
	if err := c.do("QUERY", q, &a); err != nil {
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
	if err := c.do(http.MethodPost, map[string]any{"batch": batch}, &out); err != nil {
		return nil, err
	}
	return out.Result, nil
}
