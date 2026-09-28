package when

import (
	"log/slog"
	"net/http"

	"heliosian/internal/serve"
)

type overrideBody struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Location    string   `json:"location"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Keywords    []string `json:"keywords"`
	Note        *string  `json:"note"`
	Address     string   `json:"address"`
}

func (a app) setOverride(r *http.Request, body overrideBody) (serve.None, error) {
	actor := a.actor(r)
	ops, id, empty, err := a.overrideOps(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: override set", "actor", actor.Email, "event", id, "cleared", empty)
	return serve.None{}, nil
}

type overrideImageBody struct {
	ID    string `json:"id"`
	Image string `json:"image"`
}

func (a app) setOverrideImage(r *http.Request, body overrideImageBody) (serve.None, error) {
	actor := a.actor(r)
	ops, e, image, err := a.overrideImageOps(actor, body.ID, body.Image)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: override image", "actor", actor.Email, "event", e.ID, "image", image)
	return serve.None{}, nil
}
