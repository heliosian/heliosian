package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	invitationsJob = "invitations"
	notifyHostsKey = "notify_hosts"
	maxGuestWords  = 5000
)

var guestListKinds = []string{"event", "party", "activity"}

type InvitationConfig struct {
	Sender     *mail.Mailgun
	ReplyTo    string
	SigningKey string
	Origin     func(app string) string
}

type Invitations struct {
	s       *Store
	pics    *Pictures
	config  InvitationConfig
	in      *inbox
	kick    wake
	started atomic.Bool
}

func RegisterInvitations(mux, api *http.ServeMux, s *Store, pics *Pictures, tokens auth.Tokens, now func() time.Time, config InvitationConfig) *Invitations {
	iv := &Invitations{s: s, pics: pics, config: config, in: s.Model().services.inbox(inviteInbox), kick: newWake()}
	asked := func(path string, ask func() any, run func(ctx context.Context, m *Model, env Env, actor access.Actor, body any) (any, error)) {
		api.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			m := s.Model()
			env, actor, ok := caller(w, r, m, tokens, now())
			if !ok {
				return
			}
			body := ask()
			if !serve.Decode(w, r, body) {
				return
			}
			out, err := run(r.Context(), m, env, actor, body)
			if err != nil {
				serve.Error(w, r, err)
				return
			}
			serve.Write(w, r, http.StatusOK, out)
		})
	}
	asked("/api/do/invite", func() any { return &inviteAsk{} }, func(ctx context.Context, m *Model, env Env, actor access.Actor, body any) (any, error) {
		return iv.invite(ctx, m, env, actor, *body.(*inviteAsk))
	})
	asked("/api/do/guest-mail", func() any { return &guestMailAsk{} }, func(ctx context.Context, m *Model, env Env, actor access.Actor, body any) (any, error) {
		return iv.guestMail(ctx, m, env, actor, *body.(*guestMailAsk))
	})
	asked("/api/do/add-guests", func() any { return &addGuestsAsk{} }, func(ctx context.Context, m *Model, env Env, actor access.Actor, body any) (any, error) {
		return iv.addGuests(ctx, m, env, actor, *body.(*addGuestsAsk))
	})
	mux.HandleFunc("GET /open/ext/{token}", iv.extView)
	mux.HandleFunc("GET /open/ext/{token}/flyer", iv.extFlyer)
	mux.HandleFunc("POST /open/ext/{token}", iv.extAnswer)
	mux.HandleFunc("POST /open/ext/{token}/guest", iv.extGuest)
	mux.HandleFunc("DELETE /open/ext/{token}/guest", iv.extRemoveGuest)
	mux.HandleFunc("POST /hooks/replies/mime", iv.replies)
	go iv.run()
	return iv
}

func (iv *Invitations) Start() {
	iv.started.Store(true)
	iv.s.OnTick(iv.kick.poke)
	iv.kick.poke()
}

func (iv *Invitations) write(ctx context.Context, actor access.Actor, edits []Edit) ([]string, error) {
	if len(edits) == 0 {
		return nil, nil
	}
	return Write(ctx, iv.s, actor, Env{System: invitationsJob, Now: time.Now()}, Batch{Batch: edits})
}

func (iv *Invitations) writeInvitations(ctx context.Context, actor access.Actor, edits []Edit, group, from string, more int) ([]string, error) {
	return writeChecked(ctx, iv.s, actor, Env{System: invitationsJob, Now: time.Now()}, Batch{Batch: edits}, func(m *Model) error {
		return m.mayInvite(group, from, more)
	})
}

func (m *Model) passes(ctx context.Context, env Env, table, id, cond string) bool {
	q, err := Parse(fmt.Sprintf("(from %s @row (where (= id %q) %s))", table, id, cond))
	if err != nil {
		slog.ErrorContext(ctx, "invitations: a check does not parse", "table", table, "condition", cond, "error", err)
		return false
	}
	result := m.Run(ctx, q, env)
	return result.Err == nil && len(result.IDs) == 1
}

func (m *Model) guestList(ctx context.Context, env Env, id string) (store.Row, error) {
	g, ok := m.Table("GROUP").Get(id)
	if !ok || !slices.Contains(guestListKinds, g["kind"]) || !m.passes(ctx, env, "GROUP", id, "(visible @row)") {
		return nil, access.Missing("no event, party or activity %s", id)
	}
	return g, nil
}

func (m *Model) copiesOf(group string) map[string]store.Row {
	out := map[string]store.Row{}
	for _, msg := range m.Table("MESSAGE").Referencing("group", group) {
		if msg["direction"] != "out" || (msg["kind"] != "invitation" && msg["kind"] != "skip") {
			continue
		}
		for _, r := range m.Table("RECIPIENT").Referencing("message", msg["id"]) {
			out[r["person"]] = r
		}
	}
	return out
}

func (m *Model) pendingOf(group string) []string {
	copies := m.copiesOf(group)
	out := []string{}
	for _, row := range m.effectiveRows(group) {
		if _, sent := copies[row["person"]]; !sent && !slices.Contains(out, row["person"]) {
			out = append(out, row["person"])
		}
	}
	return out
}

func (m *Model) answerOf(g store.Row, person string) string {
	for _, a := range []struct{ column, word string }{{"rsvp_yes", "yes"}, {"rsvp_no", "no"}} {
		if g[a.column] == "" {
			continue
		}
		if row, ok := m.Table("MEMBER").Find(g[a.column], person); ok && memberAs(row) == "yes" {
			return a.word
		}
	}
	return ""
}

func (m *Model) householdOf(person string) []string {
	out := []string{person}
	for _, f := range m.familiesOf(person) {
		for _, p := range m.familyPeople(f) {
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

func (m *Model) hostsOf(g store.Row) []string {
	out := []string{}
	for _, row := range m.chain(g) {
		for _, p := range m.membersOf(row["managed_by"]) {
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

func (m *Model) onList(g store.Row, person string) bool {
	if _, sent := m.copiesOf(g["id"])[person]; sent {
		return true
	}
	return m.effectivelyIn(g["id"], person)
}

func (m *Model) ended(g store.Row, now time.Time) bool {
	_, until, _, ok := mail.Span(g["start"], g["end"], School)
	return ok && until.Before(now)
}

func (m *Model) answerEdits(g store.Row, person, answer, by string, at time.Time) []Edit {
	edits := []Edit{}
	for _, column := range []string{"rsvp_yes", "rsvp_no"} {
		if g[column] == "" {
			continue
		}
		if row, ok := m.Table("MEMBER").Find(g[column], person); ok {
			edits = append(edits, Edit{Delete: row["id"]})
		}
	}
	column := map[string]string{"yes": "rsvp_yes", "no": "rsvp_no"}[answer]
	if column == "" {
		return edits
	}
	group := g[column]
	if group == "" {
		group = "@" + column
	}
	row := map[string]any{"group": group, "person": person, "member": "yes", "added": stamp(at)}
	if by != "" {
		row["added_by"] = by
	}
	return append(edits, Edit{Insert: "MEMBER", Row: row})
}

const maxGuestInvitations = 10

func (m *Model) invitationsFrom(group, person string) int {
	sent := 0
	for _, msg := range m.Table("MESSAGE").Referencing("group", group) {
		if msg["direction"] == "out" && msg["kind"] == "invitation" && msg["from_person"] == person {
			sent += len(m.Table("RECIPIENT").Referencing("message", msg["id"]))
		}
	}
	return sent
}

func (m *Model) mayInvite(group, from string, more int) error {
	if m.invitationsFrom(group, from)+more > maxGuestInvitations {
		return access.Forbidden("you can have at most %d invitations mailed for one guest list", maxGuestInvitations)
	}
	return nil
}

func invitationEdits(group, from, kind string, people []string, at time.Time) []Edit {
	created := stamp(at)
	msg := map[string]any{"direction": "out", "kind": kind, "group": group, "created": created}
	if from != "" {
		msg["from_person"] = from
	}
	edits := []Edit{{Insert: "MESSAGE", As: "invitation", Row: msg}}
	for _, p := range people {
		row := map[string]any{"message": "@invitation", "person": p, "created": created}
		if kind == "skip" {
			row["sent"] = created
		}
		edits = append(edits, Edit{Insert: "RECIPIENT", Row: row})
	}
	return edits
}

type inviteAsk struct {
	Group  string   `json:"group"`
	People []string `json:"people"`
	Skip   bool     `json:"skip"`
}

type sentAnswer struct {
	Result []string `json:"result"`
	Sent   int      `json:"sent"`
}

func (m *Model) broughtBy(g store.Row, person, viewer string) bool {
	row, ok := m.Table("MEMBER").Find(g["id"], person)
	return ok && (row["added_by"] == viewer || row["guest_of"] == viewer)
}

func (iv *Invitations) invite(ctx context.Context, m *Model, env Env, actor access.Actor, ask inviteAsk) (sentAnswer, error) {
	g, err := m.guestList(ctx, env, ask.Group)
	if err != nil {
		return sentAnswer{}, err
	}
	runs := m.passes(ctx, env, "GROUP", g["id"], "(runs_list @row)")
	pending := m.pendingOf(g["id"])
	people := ask.People
	if len(people) == 0 {
		if !runs {
			return sentAnswer{}, access.Forbidden("only the hosts send the invitation to everyone waiting for it")
		}
		people = pending
	}
	for _, p := range people {
		if !slices.Contains(pending, p) {
			return sentAnswer{}, access.Invalid("%s is not on the guest list waiting for the invitation", p)
		}
		if !runs && !m.broughtBy(g, p, env.Viewer) {
			return sentAnswer{}, access.Forbidden("only the hosts send the invitation to people they did not add")
		}
	}
	if len(people) == 0 {
		return sentAnswer{Result: []string{}}, nil
	}
	if ask.Skip && !runs {
		return sentAnswer{}, access.Forbidden("only the hosts mark the invitation sent without mailing it")
	}
	kind := "invitation"
	if ask.Skip {
		kind = "skip"
	}
	edits := append(answerGroupEdits(g), invitationEdits(g["id"], env.Viewer, kind, people, env.Now)...)
	var ids []string
	if !runs && kind == "invitation" {
		ids, err = iv.writeInvitations(ctx, actor, edits, g["id"], env.Viewer, len(people))
	} else {
		ids, err = iv.write(ctx, actor, edits)
	}
	if err != nil {
		return sentAnswer{}, err
	}
	slog.InfoContext(ctx, "invitations: sent", "viewer", env.Viewer, "group", g["id"], "kind", kind, "people", len(people))
	return sentAnswer{Result: ids[len(ids)-len(people)-1 : len(ids)-len(people)], Sent: len(people)}, nil
}

type guestMailAsk struct {
	Group   string   `json:"group"`
	Kind    string   `json:"kind"`
	To      []string `json:"to"`
	People  []string `json:"people"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	Notify  bool     `json:"notify"`
}

var guestMailKinds = map[string]string{"reminder": "reminder", "message": "post", "update": "update", "cancel": "update"}

func (iv *Invitations) storeText(ctx context.Context, m *Model, text string) (string, []Edit, error) {
	raw := []byte(text)
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	if row, ok := m.Table("CONTENT").Find(hash, "text/plain"); ok {
		return row["id"], nil, nil
	}
	name := "content/" + hash
	if _, err := keepBytes(ctx, m, iv.pics.bucket, "text/plain", raw); err != nil {
		return "", nil, fmt.Errorf("store the words: %w", err)
	}
	return "@words", []Edit{{Insert: "CONTENT", As: "words", Row: map[string]any{"hash": hash, "blob": name, "mime": "text/plain", "size": strconv.Itoa(len(raw))}}}, nil
}

func (iv *Invitations) guestMail(ctx context.Context, m *Model, env Env, actor access.Actor, ask guestMailAsk) (sentAnswer, error) {
	g, err := m.guestList(ctx, env, ask.Group)
	if err != nil {
		return sentAnswer{}, err
	}
	if !m.passes(ctx, env, "GROUP", g["id"], "(runs_list @row)") {
		return sentAnswer{}, access.Forbidden("only the hosts mail the guest list")
	}
	kind, ok := guestMailKinds[ask.Kind]
	if !ok {
		return sentAnswer{}, access.Invalid("kind is reminder, message, update or cancel, not %q", ask.Kind)
	}
	text := strings.TrimSpace(ask.Text)
	if len(text) > maxGuestWords {
		return sentAnswer{}, access.Invalid("the words run over %d characters", maxGuestWords)
	}
	if (ask.Kind == "reminder" || ask.Kind == "message") && (strings.TrimSpace(ask.Subject) == "" || text == "") {
		return sentAnswer{}, access.Invalid("a subject and some words, please")
	}
	edits := []Edit{}
	if ask.Kind == "cancel" {
		edits = append(edits, Edit{Set: g["id"], Cells: map[string]any{"status": "cancelled"}})
	}
	people := []string{}
	for _, p := range slices.Sorted(maps.Keys(m.copiesOf(g["id"]))) {
		answer := m.answerOf(g, p)
		word := answer
		if word == "" {
			word = "none"
		}
		switch {
		case ask.Kind == "cancel":
		case ask.Kind == "update" && answer == "no":
			continue
		case ask.Kind == "update":
		case !slices.Contains(ask.To, word) && !slices.Contains(ask.People, p):
			continue
		}
		people = append(people, p)
	}
	if ask.Kind == "cancel" && !ask.Notify {
		people = nil
	}
	if len(people) > 0 {
		created := stamp(env.Now)
		msg := map[string]any{"direction": "out", "kind": kind, "group": g["id"], "from_person": env.Viewer, "created": created}
		if s := strings.TrimSpace(ask.Subject); s != "" {
			msg["subject"] = s
		}
		if text != "" {
			ref, stored, err := iv.storeText(ctx, m, text)
			if err != nil {
				return sentAnswer{}, err
			}
			edits = append(edits, stored...)
			msg["content"] = ref
		}
		edits = append(edits, Edit{Insert: "MESSAGE", As: "mail", Row: msg})
		for _, p := range people {
			edits = append(edits, Edit{Insert: "RECIPIENT", Row: map[string]any{"message": "@mail", "person": p, "created": created}})
		}
	}
	if _, err := iv.write(ctx, actor, edits); err != nil {
		return sentAnswer{}, err
	}
	slog.InfoContext(ctx, "invitations: guest list mailed", "viewer", env.Viewer, "group", g["id"], "kind", ask.Kind, "people", len(people))
	return sentAnswer{Result: []string{}, Sent: len(people)}, nil
}

type guestAsk struct {
	Person string `json:"person"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}

type addGuestsAsk struct {
	Group   string     `json:"group"`
	People  []guestAsk `json:"people"`
	GuestOf string     `json:"guest_of"`
	Family  bool       `json:"family"`
	Answer  string     `json:"answer"`
	Send    bool       `json:"send"`
}

const othersJoin = `(and (= @row.join "others") (or (blank @row.eligible) (exists EFFECTIVE_MEMBER (= group @row.eligible) (= person @viewer))))`

func (iv *Invitations) addGuests(ctx context.Context, m *Model, env Env, actor access.Actor, ask addGuestsAsk) (sentAnswer, error) {
	g, err := m.guestList(ctx, env, ask.Group)
	if err != nil {
		return sentAnswer{}, err
	}
	runs := m.passes(ctx, env, "GROUP", g["id"], "(runs_list @row)")
	if !runs && !m.passes(ctx, env, "GROUP", g["id"], othersJoin) {
		return sentAnswer{}, access.Forbidden("only the hosts add people to this guest list")
	}
	if ask.GuestOf != "" && !runs && !m.passes(ctx, env, "PERSON", ask.GuestOf, "(self_or_household @row)") {
		return sentAnswer{}, access.Forbidden("a guest is brought by you or someone in your family")
	}
	if ask.Answer != "" && ask.Answer != "yes" {
		return sentAnswer{}, access.Invalid("a guest is answered for as yes or not at all, not %q", ask.Answer)
	}
	if len(ask.People) == 0 {
		return sentAnswer{}, access.Invalid("name someone to add")
	}
	edits := answerGroupEdits(g)
	people := []string{}
	outside := []string{}
	addressed := map[string]bool{}
	for i, guest := range ask.People {
		if guest.Person != "" {
			if !m.passes(ctx, env, "PERSON", guest.Person, "(person_visible @row)") {
				return sentAnswer{}, access.Missing("no person %s", guest.Person)
			}
			people = append(people, guest.Person)
			addressed[guest.Person] = m.addressOf(guest.Person) != ""
			continue
		}
		address := strings.ToLower(strings.TrimSpace(guest.Email))
		name := strings.Join(strings.Fields(guest.Name), " ")
		if address != "" && (mail.AddressOf(address) != address || !strings.Contains(address, "@")) {
			return sentAnswer{}, access.Invalid("%q is not an address", guest.Email)
		}
		if address == "" && name == "" {
			return sentAnswer{}, access.Invalid("someone from outside needs a name or an address")
		}
		if p := m.personAt(address); address != "" && p != "" {
			people = append(people, p)
			addressed[p] = true
			continue
		}
		as := fmt.Sprintf("guest%d", i)
		row := map[string]any{"source": "guest"}
		if name != "" {
			row["name_long_override"] = name
		}
		edits = append(edits, Edit{Insert: "PERSON", As: as, Row: row})
		if address != "" {
			edits = append(edits, Edit{Insert: "PERSON_EMAIL", Row: map[string]any{"person": "@" + as, "address": address, "primary": true, "source": "guest"}})
		}
		people = append(people, "@"+as)
		outside = append(outside, "@"+as)
		addressed["@"+as] = address != ""
	}
	if ask.Family && len(outside) > 1 {
		edits = append(edits,
			Edit{Insert: "GROUP", As: "family", Row: map[string]any{"kind": "family", "status": "open", "consent": "listed", "address_consent": "withheld", "phone_consent": "withheld", "posting": "none", "replying": "none"}},
			Edit{Set: "@family", Cells: map[string]any{"managed_by": "@family", "visible_to": "@family", "members_visible_to": "@family"}},
		)
		for _, p := range outside {
			edits = append(edits, Edit{Insert: "MEMBER", Row: map[string]any{"group": "@family", "person": p, "member": "yes"}})
		}
	}
	at := stamp(env.Now)
	added := []string{}
	for _, p := range people {
		if !strings.HasPrefix(p, "@") {
			if row, ok := m.Table("MEMBER").Find(g["id"], p); ok {
				if memberAs(row) == "excluded" && runs {
					edits = append(edits, Edit{Set: row["id"], Cells: map[string]any{"member": "yes"}})
					added = append(added, p)
				}
				continue
			}
			if m.effectivelyIn(g["id"], p) {
				continue
			}
		}
		row := map[string]any{"group": g["id"], "person": p, "member": "yes", "added_by": env.Viewer, "added": at}
		if ask.GuestOf != "" && ask.GuestOf != p {
			row["guest_of"] = ask.GuestOf
		}
		edits = append(edits, Edit{Insert: "MEMBER", Row: row})
		added = append(added, p)
	}
	if ask.Answer == "yes" {
		for _, p := range people {
			edits = append(edits, m.answerEdits(g, p, "yes", env.Viewer, env.Now)...)
		}
	}
	sending := []string{}
	if !runs || ask.Send {
		copies := m.copiesOf(g["id"])
		for _, p := range added {
			if _, sent := copies[p]; !sent && addressed[p] {
				sending = append(sending, p)
			}
		}
	}
	if len(sending) > 0 {
		edits = append(edits, invitationEdits(g["id"], env.Viewer, "invitation", sending, env.Now)...)
	}
	if !runs && len(sending) > 0 {
		_, err = iv.writeInvitations(ctx, actor, edits, g["id"], env.Viewer, len(sending))
	} else {
		_, err = iv.write(ctx, actor, edits)
	}
	if err != nil {
		return sentAnswer{}, err
	}
	slog.InfoContext(ctx, "invitations: guests added", "viewer", env.Viewer, "group", g["id"], "added", len(added), "sent", len(sending), "family", ask.Family && len(outside) > 1)
	return sentAnswer{Result: []string{}, Sent: len(sending)}, nil
}

func (m *Model) byToken(table, token string) (store.Row, bool) {
	if row, ok := m.Table(table).Find(token); ok {
		return row, true
	}
	if a, ok := m.Table("ALIAS").Find(strings.ToLower(token)); ok {
		return m.Table(table).Get(a["target"])
	}
	return nil, false
}

func notifies(m *Model, person, group string) bool {
	row, ok := m.Table("PERSON_SETTING").Find(person, "when", notifyHostsKey)
	return ok && slices.Contains(cells.SplitList(row["value"]), group)
}
