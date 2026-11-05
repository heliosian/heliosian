package db

import (
	"context"
	"net/http"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

type removeGuestAsk struct {
	Group  string `json:"group"`
	Person string `json:"person"`
}

func (iv *Invitations) removeGuest(ctx context.Context, _ *Model, env Env, actor access.Actor, ask removeGuestAsk) (sentAnswer, error) {
	var written []string
	_, err := iv.s.Transact(ctx, actor, func(tx *store.Tx[Model]) error {
		m := iv.s.In(tx)
		g, err := m.guestList(ctx, env, ask.Group)
		if err != nil {
			return err
		}
		written, err = stageGuestRemoval(iv.s, tx, env, g, ask.Person)
		return err
	})
	return sentAnswer{Result: written}, err
}

func stageGuestRemoval(s *Store, tx *store.Tx[Model], env Env, g store.Row, person string) ([]string, error) {
	m := s.In(tx)
	row, exists := m.Table("MEMBER").Find(g["id"], person)
	column := "member"
	state := "excluded"
	if g["kind"] == "activity" {
		if memberAs(row) == "yes" {
			return nil, access.Refuse(http.StatusConflict, "cancel the sign-up in hca-team to leave the guest list")
		}
		column = "invited"
	} else {
		if exists {
			if err := m.Authorize(env, Change{Table: "MEMBER", Old: row}); err != nil {
				return nil, err
			}
		}
		runs, err := m.passes(tx.Context(), Env{Viewer: env.Viewer, Now: env.Now}, "GROUP", g["id"], "(runs_list @row)")
		if err != nil {
			return nil, err
		}
		if !runs && memberAs(row) != "excluded" {
			state = "cancelled"
		}
	}
	edit := Edit{Insert: "MEMBER", Row: map[string]any{"group": g["id"], "person": person, column: state, "added_by": env.Viewer, "added": stamp(env.Now)}}
	if exists {
		edit = Edit{Set: row["id"], Cells: map[string]any{column: state}}
		if column == "invited" && row["guest_of"] != "" {
			runs, err := m.passes(tx.Context(), Env{Viewer: env.Viewer, Now: env.Now}, "GROUP", g["id"], "(runs_list @row)")
			if err != nil {
				return nil, err
			}
			if runs || row["guest_of"] != env.Viewer {
				edit.Cells["guest_of"] = ""
			}
		}
	}
	edits := []Edit{}
	if g["kind"] != "activity" {
		edits = m.clearAnswerEdits(g, person)
	}
	edits = append(edits, edit)
	return stageEdits(s, tx, env, edits, "remove guest", m.whole(env))
}
