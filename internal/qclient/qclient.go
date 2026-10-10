package qclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	Production = "https://who.heliosian.com"
	ImportMode = "import"
)

type Client struct {
	Base  string
	Token string
	Mode  string
	Spoof string
}

type Answer struct {
	Now       string                                  `json:"now"`
	Query     string                                  `json:"query"`
	Result    []string                                `json:"result"`
	Resources map[string]map[string]map[string]string `json:"resources"`
	Trace     string                                  `json:"-"`
}

func (c *Client) Send(method, path, kind string, body io.Reader, out any) error {
	_, err := c.send(method, path, kind, body, out)
	return err
}

func (c *Client) send(method, path, kind string, body io.Reader, out any) (http.Header, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	header, err := c.once(method, path, kind, raw, out)
	var status *StatusError
	if !errors.As(err, &status) || status.Code != http.StatusUnauthorized || c.Base != Production {
		return header, err
	}
	if err := c.signIn(); err != nil {
		return nil, err
	}
	return c.once(method, path, kind, raw, out)
}

func (c *Client) once(method, path, kind string, raw []byte, out any) (http.Header, error) {
	req, err := http.NewRequest(method, c.Base+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", kind)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if c.Mode != "" {
		req.Header.Set("Helios-Mode", c.Mode)
	}
	if c.Spoof != "" {
		req.Header.Set("Helios-Spoof", c.Spoof)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Code: resp.StatusCode, What: fmt.Sprintf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(got)))}
	}
	return resp.Header, json.Unmarshal(got, out)
}

type StatusError struct {
	Code int
	What string
}

func (e *StatusError) Error() string {
	return e.What
}

func (c *Client) Query(tree any) (Answer, error) {
	raw, err := json.Marshal(tree)
	if err != nil {
		return Answer{}, err
	}
	return c.query("application/json", bytes.NewReader(raw))
}

func (c *Client) QueryText(src string) (Answer, error) {
	return c.query("text/plain", strings.NewReader(src))
}

func (c *Client) query(kind string, body io.Reader) (Answer, error) {
	var a Answer
	header, err := c.send("QUERY", "/api/q", kind, body, &a)
	if err != nil {
		return Answer{}, err
	}
	a.Trace = header.Get("Trace")
	return a, nil
}

func (c *Client) Write(batch any) ([]string, error) {
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
