package ask

import (
	"encoding/json"
	"strings"
	"time"

	"heliosian/internal/model"
)

type linkCard struct {
	URL   string `json:"url"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Image string `json:"image,omitempty"`
	Badge string `json:"badge,omitempty"`
	Color string `json:"color,omitempty"`
}

var cardKinds = map[string]struct{ kind, name, image string }{
	"people":      {"person", "fullName", "heroPhotoUrl"},
	"families":    {"family", "name", "photoUrl"},
	"classrooms":  {"classroom", "name", "imageUrl"},
	"events":      {"event", "title", ""},
	"email-lists": {"group", "title", ""},
	"activities":  {"activity", "title", "imageUrl"},
	"parties":     {"party", "title", "imageUrl"},
}

func (t *turn) linkCard(address string) (linkCard, bool) {
	r, ok := t.found.of(address)
	if !ok {
		return linkCard{}, false
	}
	shape, ok := cardKinds[r.typ]
	if !ok {
		return linkCard{}, false
	}
	paths := map[string]string{"it": one(r.typ, r.id, params())}
	if r.typ == "classrooms" {
		paths["settings"] = collection("when-settings", params())
	}
	env, err := t.read(paths)
	if err != nil {
		return linkCard{}, false
	}
	obj := env.Resources[r.typ][r.id]
	card := linkCard{URL: address, Kind: shape.kind, Name: text(obj, shape.name)}
	if shape.image != "" {
		card.Image = text(obj, shape.image)
		if strings.HasPrefix(card.Image, "/") {
			card.Image = appURL(text(obj, "app"), card.Image)
		}
	}
	if start := text(obj, "start"); r.typ == "events" && len(start) >= len(model.DateFormat) {
		day, err := time.ParseInLocation(model.DateFormat, start[:len(model.DateFormat)], model.Location)
		if err != nil {
			panic(err)
		}
		card.Badge = day.Format("Mon Jan 2")
	}
	if r.typ == "classrooms" {
		for _, settings := range env.Resources["when-settings"] {
			colors := map[string]string{}
			if err := json.Unmarshal(settings["colors"], &colors); err != nil {
				panic(err)
			}
			card.Color = colors[card.Name]
		}
	}
	return card, true
}
