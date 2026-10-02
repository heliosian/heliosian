package model

import (
	"net/http"
	"slices"
	"testing"
)

const birthdayAdmin = "mina.park@heliosschool.org"

func birthdayWidgetOf(t *testing.T, mux http.Handler, as string) map[string][]map[string]any {
	t.Helper()
	r := get(t, mux, as, "/api/birthday-settings?include=mine,all")
	s := r.Resources["birthday-settings"][r.ids(t)[0]]
	out := map[string][]map[string]any{}
	for _, name := range []string{"mine", "all"} {
		out[name] = []map[string]any{}
		list, _ := s[name].([]any)
		for _, key := range list {
			out[name] = append(out[name], r.Resources["birthdays"][key.(string)])
		}
	}
	return out
}

func emailsOf(list []map[string]any) []string {
	out := []string{}
	for _, b := range list {
		out = append(out, b["email"].(string))
	}
	return out
}

func TestBirthdayWidget(t *testing.T) {
	_, mux := birthdaysServer(t)
	robin := birthdayWidgetOf(t, mux, parent)
	if got := emailsOf(robin["mine"]); !slices.Equal(got, []string{"bill.ryder@heliosschool.org"}) {
		t.Errorf("a volunteer's own: %v", got)
	}
	if len(robin["all"]) != 0 {
		t.Errorf("a volunteer sees all: %v", emailsOf(robin["all"]))
	}
	jordan := birthdayWidgetOf(t, mux, "jordan.whitfield@heliosschool.org")
	last := ""
	for _, b := range jordan["mine"] {
		next, _ := b["next"].(map[string]any)
		soon := b["newsletterDate"] == "2026-09-11" || b["newsletterDate"] == "2026-09-18"
		if b["me"].(map[string]any)["mine"] != true || !soon || next == nil || next["step"] == LateNewsletter || next["day"].(string) < last {
			t.Errorf("jordan's work out of turn or not theirs: %v", b)
		}
		last, _ = next["day"].(string)
	}
	admin := birthdayWidgetOf(t, mux, birthdayAdmin)
	issues := map[string]bool{}
	for _, b := range admin["all"] {
		issues[b["newsletterDate"].(string)] = true
		if b["stage"] == "" {
			t.Errorf("no status: %v", b)
		}
	}
	delete(issues, "2026-09-11")
	delete(issues, "2026-09-18")
	if len(admin["all"]) == 0 || len(issues) != 0 {
		t.Errorf("all reaches past the next two issues %v: %v", issues, emailsOf(admin["all"]))
	}
	r := get(t, mux, parent, "/api/birthday-settings?include=mine.fallback-charity")
	bill := r.Resources["birthdays"][r.Resources["birthday-settings"][r.ids(t)[0]]["mine"].([]any)[0].(string)]
	if got := r.follow(bill, "fallback-charity", "charities"); got == nil || got["name"] != "Second Harvest of Silicon Valley" || bill["fallbackNote"] != nil {
		t.Errorf("Bill, with no charity last year, falls back to the default: %v %v", got, bill)
	}
	r = get(t, mux, parent, "/api/birthdays/dana.hawkins@heliosschool.org?include=fallback-charity")
	dana := r.Resources["birthdays"][r.id(t)]
	if got := r.follow(dana, "fallback-charity", "charities"); got == nil || got["name"] != "Birthfund" || dana["fallbackNote"] != "Because caring for children starts with caring for their mothers." {
		t.Errorf("Dana falls back to last year's: %v %v", got, dana)
	}
	if stranger := birthdayWidgetOf(t, mux, "sam.whitfield@heliosschool.org"); len(stranger["mine"])+len(stranger["all"]) != 0 {
		t.Errorf("someone off the team: %+v", stranger)
	}
}
