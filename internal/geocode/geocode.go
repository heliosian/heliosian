package geocode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var client = &http.Client{Timeout: 5 * time.Second}

type Point struct {
	Lat float64
	Lng float64
}

type Client struct {
	key string
}

func New(key string) *Client {
	return &Client{key: key}
}

func (c *Client) Lookup(address string) (Point, error) {
	req, err := http.NewRequest("POST", "https://geocode.googleapis.com/v4/geocode/address", strings.NewReader(url.Values{"addressQuery": {address}}.Encode()))
	if err != nil {
		return Point{}, err
	}
	req.Header.Set("X-HTTP-Method-Override", "GET")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Goog-Api-Key", c.key)
	resp, err := client.Do(req)
	if err != nil {
		return Point{}, err
	}
	defer resp.Body.Close()
	var parsed struct {
		Results []struct {
			Location struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"location"`
		} `json:"results"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Point{}, err
	}
	if parsed.Error.Message != "" {
		return Point{}, fmt.Errorf("geocode: %s", parsed.Error.Message)
	}
	if len(parsed.Results) == 0 {
		return Point{}, fmt.Errorf("geocode: no results")
	}
	return Point{Lat: parsed.Results[0].Location.Latitude, Lng: parsed.Results[0].Location.Longitude}, nil
}

type Suggestion struct {
	Text string `json:"text"`
}

func (c *Client) Suggest(input string) ([]Suggestion, error) {
	body, _ := json.Marshal(map[string]any{
		"input":               input,
		"includedRegionCodes": []string{"us"},
		"locationBias":        map[string]any{"circle": map[string]any{"center": map[string]float64{"latitude": 37.44, "longitude": -122.14}, "radius": 80000}},
	})
	req, err := http.NewRequest("POST", "https://places.googleapis.com/v1/places:autocomplete", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.key)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var parsed struct {
		Suggestions []struct {
			PlacePrediction struct {
				Text struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"placePrediction"`
		} `json:"suggestions"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Error.Message != "" {
		return nil, fmt.Errorf("places: %s", parsed.Error.Message)
	}
	out := []Suggestion{}
	for _, s := range parsed.Suggestions {
		if s.PlacePrediction.Text.Text != "" {
			out = append(out, Suggestion{Text: s.PlacePrediction.Text.Text})
		}
	}
	return out, nil
}
