package db

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font/opentype"

	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

type InvitationArt struct {
	pics  *Pictures
	fonts *themeFonts
}

func NewInvitationArt(pics *Pictures) *InvitationArt {
	return &InvitationArt{pics: pics, fonts: newThemeFonts(pics.bucket)}
}

func (a *InvitationArt) invitationCard(w http.ResponseWriter, r *http.Request, m *Model, g store.Row, l look, hosted bool) {
	card, err := cardDesign(m, l)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	backdrop := coverPicture(m, l.theme, l.backdropPicture, designValue(card, "page.backdrop"))
	cardArt := coverPicture(m, l.theme, l.cardPicture, designValue(card, "card.art"))
	day, hours := themedDayAndHours(card, g)
	hosts := ""
	if names := cmp.Or(g["hosted_by"], joinNames(m.namesOf(m.hostsOf(g)))); hosted && names != "" {
		hosts = "Hosted by " + names
	}
	design, _ := json.Marshal(card)
	parts := []string{g["id"], string(design), backdrop, cardArt, g["name"], day, hours, g["place"], g["location"], hosts}
	if sharecard.InvitationAnswered(w, r, parts...) {
		return
	}
	v := sharecard.Invitation{
		Kicker: designValue(card, "text.kicker.words"), KickerUpper: designValue(card, "text.kicker.case") == "uppercase",
		Title: g["name"], Day: day, Hours: hours, Place: g["place"], Address: g["location"], Hosts: hosts,
		Wordless: l.cardPicture != "" && designAt(card, "card.words") == false, SideBySide: designValue(card, "card.details") == "side-by-side",
	}
	for _, c := range []struct {
		into *color.RGBA
		path string
	}{{&v.Backdrop, "page.backdrop"}, {&v.Paper, "card.background"}, {&v.Accent, "card.accent"}} {
		if *c.into, err = designColor(card, c.path); err != nil {
			serve.Error(w, r, err)
			return
		}
	}
	for _, c := range []struct {
		into *color.RGBA
		role string
	}{{&v.KickerInk, "kicker"}, {&v.TitleInk, "title"}, {&v.DayInk, "day"}, {&v.PlaceInk, "place"}, {&v.HostsInk, "hosts"}} {
		hex, err := roleColor(card, c.role)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		*c.into = hexRGBA(hex)
	}
	if v.BackdropPicture, err = a.decoded(r.Context(), m, backdrop); err != nil {
		serve.Error(w, r, err)
		return
	}
	if v.CardPicture, err = a.decoded(r.Context(), m, cardArt); err != nil {
		serve.Error(w, r, err)
		return
	}
	bold, medium, err := sharecard.SiteFonts()
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	fonts := []struct {
		into *(*opentype.Font)
		role string
		site *opentype.Font
	}{{&v.DisplayFont, "title", bold}, {&v.BodyFont, "day", medium}, {&v.KickerFont, "kicker", medium}, {&v.PlaceFont, "place", medium}}
	failed := make([]error, len(fonts))
	var wg sync.WaitGroup
	for i, f := range fonts {
		*f.into = f.site
		face, loaded := themeFace(card, f.role)
		if !loaded {
			continue
		}
		wg.Go(func() {
			*f.into, failed[i] = a.fonts.font(r.Context(), face)
		})
	}
	wg.Wait()
	if err := errors.Join(failed...); err != nil {
		serve.Error(w, r, err)
		return
	}
	sharecard.ServeInvitation(w, r, v, parts...)
}

func hexRGBA(hex string) color.RGBA {
	n, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	return color.RGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 0xff}
}

func coverPicture(m *Model, theme, picture, layers string) string {
	if picture != "" {
		return picture
	}
	for _, layer := range strings.Split(layers, "),") {
		if !strings.Contains(layer, "cover") {
			continue
		}
		found := cssURL.FindStringSubmatch(layer + ")")
		if found == nil {
			continue
		}
		if picture, ok := m.Table("THEME_PICTURE").Find(theme, found[1]+found[2]+found[3]); ok && picture["picture"] != "" {
			return picture["picture"]
		}
	}
	return ""
}

func (a *InvitationArt) decoded(ctx context.Context, m *Model, content string) (image.Image, error) {
	if content == "" {
		return nil, nil
	}
	data, _, err := m.contentBytes(ctx, a.pics.bucket, content)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}
