package when

import (
	"log/slog"
	"net/http"
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
	Note        string   `json:"note"`
	Address     string   `json:"address"`
}

func (a app) setOverride(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body overrideBody
	if !decode(w, r, &body) {
		return
	}
	ops, id, empty, err := a.overrideOps(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: override set", "actor", actor.Email, "event", id, "cleared", empty)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) setOverrideImage(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		ID    string `json:"id"`
		Image string `json:"image"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, e, image, err := a.overrideImageOps(actor, body.ID, body.Image)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: override image", "actor", actor.Email, "event", e.ID, "image", image)
	w.WriteHeader(http.StatusNoContent)
}
