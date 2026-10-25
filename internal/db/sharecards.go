package db

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"heliosian/internal/blob"
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
