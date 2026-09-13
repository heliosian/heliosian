// Package geocode resolves street addresses to coordinates via the google geocoding api.
package geocode

import (
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
