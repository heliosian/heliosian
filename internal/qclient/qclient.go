package qclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const Production = "https://who.heliosian.com"

type Client struct {
	Base string
	Key  string
}

type Answer struct {
	Now       string                                  `json:"now"`
	Query     string                                  `json:"query"`
	Result    []string                                `json:"result"`
	Resources map[string]map[string]map[string]string `json:"resources"`
}

func (c Client) Send(method, path, kind string, body io.Reader, out any) error {
	req, err := http.NewRequest(method, c.Base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", kind)
	req.Header.Set("Authorization", "Bearer "+c.Key)
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
		return &StatusError{Code: resp.StatusCode, What: fmt.Sprintf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(got)))}
	}
	return json.Unmarshal(got, out)
}

type StatusError struct {
	Code int
	What string
}

func (e *StatusError) Error() string {
	return e.What
}

func (c Client) Query(tree any) (Answer, error) {
	raw, err := json.Marshal(tree)
	if err != nil {
		return Answer{}, err
	}
	var a Answer
	return a, c.Send("QUERY", "/api/q", "application/json", bytes.NewReader(raw), &a)
}

func (c Client) QueryText(src string) (Answer, error) {
	var a Answer
	return a, c.Send("QUERY", "/api/q", "text/plain", strings.NewReader(src), &a)
}

func (c Client) Write(batch any) ([]string, error) {
	raw, err := json.Marshal(batch)
	if err != nil {
		return nil, err
	}
	var out struct {
		Result []string `json:"result"`
	}
	if err := c.Send(http.MethodPost, "/api/q", "application/json", bytes.NewReader(raw), &out); err != nil {
		return nil, err
	}
	return out.Result, nil
}
