package db

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

const (
	consentApp  = "preferences"
	consentTab  = "Sheet1"
	consentPoll = time.Minute

	consentTimestamp  = "Timestamp"
	consentEmail      = "Email Address"
	consentStatus     = "Communication Opt-In Status"
	consentPermission = "You have my permission to share the folllowing:"
	consentTimeFormat = "1/2/2006 15:04:05"

	optInAnswer  = "I agree to have family names and emails in the Helios Community Apps"
	optOutAnswer = "Please remove all family names and emails from the Helios Community Apps. " +
		"I understand that we will be unable to access the Helios Who directory, " +
		"the volunteer portal and Spring Celebration fun(d)raiser events."
	shareAddressAnswer = "Home Address (if provided on Veracross)"
	sharePhoneAnswer   = "Adult Phone Number (if provided on Veracross)"
)

type response struct {
	when    time.Time
	out     bool
	address bool
	phone   bool
}

func parseResponse(row map[string]string) (string, response, error) {
	email := strings.ToLower(strings.TrimSpace(row[consentEmail]))
	if !address.MatchString(email) {
		return "", response{}, fmt.Errorf("a response has the email %q", row[consentEmail])
	}
	when, err := time.Parse(consentTimeFormat, row[consentTimestamp])
	if err != nil {
		return "", response{}, fmt.Errorf("%s's response has the timestamp %q", email, row[consentTimestamp])
	}
	r := response{when: when}
	switch row[consentStatus] {
	case optInAnswer:
	case optOutAnswer:
		r.out = true
	default:
		return "", response{}, fmt.Errorf("%s's response has the opt-in answer %q", email, row[consentStatus])
	}
	if cell := row[consentPermission]; cell != "" {
		for _, item := range strings.Split(cell, ", ") {
			switch {
			case item == shareAddressAnswer && !r.address:
				r.address = true
			case item == sharePhoneAnswer && !r.phone:
				r.phone = true
			default:
				return "", response{}, fmt.Errorf("%s's response grants %q", email, item)
			}
		}
	}
	return email, r, nil
}

func latest(into map[string]response, key string, r response) {
	if current, ok := into[key]; !ok || r.when.After(current.when) {
		into[key] = r
	}
}

func sharedOr(shared bool) string {
	if shared {
		return "shared"
	}
	return "withheld"
}

func (m *Model) consentWrites(responses []map[string]string) ([]write, error) {
	groups := m.Table("GROUP")
	familiesOf := map[string][]string{}
	staff := map[string]bool{}
	for _, row := range m.Table("MEMBER").All() {
		g, _ := groups.Get(row["group"])
		if g["kind"] == "family" && !slices.Contains(familiesOf[row["person"]], row["group"]) {
			familiesOf[row["person"]] = append(familiesOf[row["person"]], row["group"])
		}
		if g["kind"] == "role" && g["slug"] == "staff" {
			staff[row["person"]] = true
		}
	}
	up := map[string]string{}
	var find func(string) string
	find = func(f string) string {
		if up[f] == "" || up[f] == f {
			return f
		}
		up[f] = find(up[f])
		return up[f]
	}
	for _, families := range familiesOf {
		for _, f := range families[1:] {
			up[find(f)] = find(families[0])
		}
	}

	byLinked := map[string]response{}
	byPerson := map[string]response{}
	for _, row := range responses {
		email, r, err := parseResponse(row)
		if err != nil {
			return nil, err
		}
		person := m.PersonOf(email)
		if person == "" {
			continue
		}
		if len(familiesOf[person]) == 0 {
			latest(byPerson, person, r)
			continue
		}
		latest(byLinked, find(familiesOf[person][0]), r)
	}

	writes := []write{}
	wants := map[string]map[string]string{}
	for _, p := range m.Table("PERSON").All() {
		if p["source"] == "guest" {
			continue
		}
		governing := []response{}
		answered := true
		for _, f := range familiesOf[p["id"]] {
			r, ok := byLinked[find(f)]
			if !ok {
				answered = false
				continue
			}
			governing = append(governing, r)
		}
		if r, ok := byPerson[p["id"]]; ok {
			governing = append(governing, r)
		}
		if len(governing) == 0 {
			answered = false
		}
		out, shareAddress, sharePhone := false, true, true
		for _, r := range governing {
			out = out || r.out
			shareAddress = shareAddress && r.address
			sharePhone = sharePhone && r.phone
		}
		consent := "listed"
		if out || (!answered && !staff[p["id"]]) {
			consent = "withheld"
			shareAddress, sharePhone = false, false
		}
		want := map[string]string{"consent": consent, "address_consent": sharedOr(shareAddress), "phone_consent": sharedOr(sharePhone)}
		wants[p["id"]] = want
		writes = appendChanges(writes, p, want)
	}
	for _, g := range groups.All() {
		if g["kind"] != "family" {
			continue
		}
		leads := 0
		listed, shareAddress, sharePhone := true, true, true
		for _, row := range m.Table("MEMBER").Referencing("group", g["id"]) {
			if row["role"] != "lead" {
				continue
			}
			leads++
			w := wants[row["person"]]
			listed = listed && w["consent"] == "listed"
			shareAddress = shareAddress && w["address_consent"] == "shared"
			sharePhone = sharePhone && w["phone_consent"] == "shared"
		}
		if leads == 0 {
			listed, shareAddress, sharePhone = false, false, false
		}
		consent := "listed"
		if !listed {
			consent = "withheld"
		}
		writes = appendChanges(writes, g, map[string]string{"consent": consent, "address_consent": sharedOr(shareAddress), "phone_consent": sharedOr(sharePhone)})
	}
	return writes, nil
}

func appendChanges(writes []write, row store.Row, want map[string]string) []write {
	cells := map[string]any{}
	for _, column := range slices.Sorted(maps.Keys(want)) {
		if !strings.EqualFold(row[column], want[column]) {
			cells[column] = want[column]
		}
	}
	if len(cells) == 0 {
		return writes
	}
	return append(writes, write{Set: row["id"], Cells: cells})
}

type Consent struct {
	s      *Store
	queue  *store.Queue
	source data.Source
	wake   chan struct{}
	rows   []map[string]string
}

func StartConsent(s *Store, queue *store.Queue, source data.Source) {
	c := &Consent{s: s, queue: queue, source: source, wake: make(chan struct{}, 1)}
	go c.loop()
	queue.OnSwap(c.poke)
}

func (c *Consent) poke() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Consent) loop() {
	tick := time.NewTicker(consentPoll)
	c.read()
	for {
		c.apply()
		select {
		case <-tick.C:
			c.read()
		case <-c.wake:
		}
	}
}

func (c *Consent) read() {
	_, rows, err := c.source.Table(consentApp, consentTab)
	if err != nil {
		slog.Error("read the consent form's responses", "error", err)
		return
	}
	c.rows = rows
}

func (c *Consent) apply() {
	if c.rows == nil {
		return
	}
	writes, err := c.s.Model().consentWrites(c.rows)
	if err != nil {
		slog.Error("work out consent from the form", "error", err)
		return
	}
	if len(writes) == 0 {
		return
	}
	env := Env{System: importReader, Now: time.Now()}
	if _, err := Write(context.Background(), c.s, c.queue, access.System(importReader), env, Batch{Batch: writes}); err != nil {
		slog.Error("write consent", "people", len(writes), "error", err)
		return
	}
	slog.Info("wrote consent", "people", len(writes), "responses", len(c.rows))
}
