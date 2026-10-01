package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"heliosian/internal/data"
)

const consentHeader = `Timestamp,Email Address,Communication Opt-In Status,You have my permission to share the folllowing:` + "\n"

func consentForm(t *testing.T, rows ...string) *data.Dir {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, consentApp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, consentApp, consentTab+".csv"), []byte(consentHeader+strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &data.Dir{Root: root}
}

func consentOf(s *Store, person string) string {
	p, _ := s.Model().Table("PERSON").Get(person)
	return p["consent"] + " " + p["address_consent"] + " " + p["phone_consent"]
}

func familyConsent(s *Store) string {
	g, _ := s.Model().Table("GROUP").Get("grp00000000020")
	return g["consent"] + " " + g["address_consent"] + " " + g["phone_consent"]
}

func TestConsent(t *testing.T) {
	s, queue := sampleWithQueue(t)
	run := func(form *data.Dir) {
		t.Helper()
		c := &Consent{s: s, queue: queue, source: form}
		c.read()
		c.apply()
		writes, err := s.Model().consentWrites(c.rows)
		if err != nil || len(writes) != 0 {
			t.Fatalf("after applying, consent still plans %v: %v", writes, err)
		}
	}

	run(consentForm(t))
	for person, want := range map[string]string{student: "withheld withheld withheld", parent: "withheld withheld withheld", staff: "listed shared shared"} {
		if got := consentOf(s, person); got != want {
			t.Errorf("with no responses, %s is %q, want %q", person, got, want)
		}
	}
	if got := familyConsent(s); got != "withheld withheld withheld" {
		t.Errorf("with no responses, the family is %q", got)
	}

	run(consentForm(t,
		`8/18/2026 9:12:04,Rowan@Example.com,I agree to have family names and emails in the Helios Community Apps,Home Address (if provided on Veracross)`,
		`8/18/2026 9:30:00,nobody@example.org,I agree to have family names and emails in the Helios Community Apps,`,
		`8/19/2026 9:00:00,maya.lindqvist@example.org,I agree to have family names and emails in the Helios Community Apps,`))
	for person, want := range map[string]string{student: "listed shared withheld", parent: "listed shared withheld", staff: "listed withheld withheld"} {
		if got := consentOf(s, person); got != want {
			t.Errorf("after the family's opt-in through another email, %s is %q, want %q", person, got, want)
		}
	}
	if got := familyConsent(s); got != "listed shared withheld" {
		t.Errorf("after the family's opt-in, the family is %q", got)
	}

	run(consentForm(t,
		`8/18/2026 9:12:04,rowan@example.com,I agree to have family names and emails in the Helios Community Apps,Home Address (if provided on Veracross)`,
		`8/20/2026 9:12:04,rowan.ashdown@example.org,"Please remove all family names and emails from the Helios Community Apps. I understand that we will be unable to access the Helios Who directory, the volunteer portal and Spring Celebration fun(d)raiser events.",`))
	if got := consentOf(s, student); got != "withheld withheld withheld" {
		t.Errorf("after the family's later opt-out, the student is %q", got)
	}
	if got := familyConsent(s); got != "withheld withheld withheld" {
		t.Errorf("after the family's later opt-out, the family is %q", got)
	}
	if p, _ := s.Model().Table("PERSON").Get(guest); p["consent"] != "" {
		t.Errorf("a guest was given consent %q", p["consent"])
	}
}

func TestConsentRefusesAnUnknownAnswer(t *testing.T) {
	s := sample(t)
	_, err := s.Model().consentWrites([]map[string]string{{
		consentTimestamp: "8/18/2026 9:12:04", consentEmail: "rowan@example.com", consentStatus: "Sure, why not",
	}})
	if err == nil || !strings.Contains(err.Error(), "opt-in answer") {
		t.Fatalf("an unknown answer: %v", err)
	}
}
