package db

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"heliosian/internal/blob"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

func (m *Model) cardPicture(ctx context.Context, bucket *blob.Bucket, app string, g store.Row, name string) []byte {
	if name == "" {
		return nil
	}
	picture, _, err := m.contentBytes(ctx, bucket, name)
	if err != nil && !errors.Is(err, blob.ErrNotFound) {
		slog.ErrorContext(ctx, app+": read a share card's picture", "group", g["id"], "name", name, "error", err)
	}
	return picture
}

func serveGroupCard(w http.ResponseWriter, r *http.Request, style *sharecard.Style, card sharecard.Card, name string, whole bool, load func() ([]byte, error), parts ...string) {
	parts = append(parts, name, cardWhole(whole))
	if style.Answered(w, r, parts...) {
		return
	}
	picture, err := load()
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	card.Picture, card.Whole = picture, whole
	if name != "" && len(picture) == 0 {
		style.ServeOnce(w, r, card, parts...)
		return
	}
	style.Serve(w, r, card, parts...)
}

func cardWhole(whole bool) string {
	if whole {
		return "whole"
	}
	return "cropped"
}

func whenCardLines(day, hours, location string) []sharecard.Line {
	return []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}, {Icon: "pin", Text: location}}
}

func eventBlurb(g store.Row) string {
	parts := []string{}
	for _, s := range []string{whenText(g), g["location"], blurb(g["description"])} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " — ")
}
