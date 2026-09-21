// Package geocode resolves street addresses to coordinates via the google geocoding api.
package geocode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

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
	resp, err := http.Get("https://maps.googleapis.com/maps/api/geocode/json?address=" +
		url.QueryEscape(address) + "&key=" + url.QueryEscape(c.key))
	if err != nil {
		return Point{}, err
	}
	defer resp.Body.Close()
	var parsed struct {
		Status  string `json:"status"`
		Results []struct {
			Geometry struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Point{}, err
	}
	if parsed.Status != "OK" || len(parsed.Results) == 0 {
		return Point{}, fmt.Errorf("geocode %q: %s", address, parsed.Status)
	}
	return Point{Lat: parsed.Results[0].Geometry.Location.Lat, Lng: parsed.Results[0].Geometry.Location.Lng}, nil
}

// Suggestion is one address Google offers for what someone has typed so
// far - the whole line to put in the box.
type Suggestion struct {
	Text string `json:"text"`
}

// Suggest is Google's Places Autocomplete (New) for a few characters of
// an address, kept to the United States and leaning to the Bay Area, so
// a host typing a venue or a street gets whole addresses to pick from.
// The key is the server's, never the page's, so the page is given the
// suggestions through the app's own route.
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
	resp, err := http.DefaultClient.Do(req)
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
