package tomarkdown

import (
	"strings"
	"testing"
)

func TestTrackingLinksAreTheOnlyOnesTouched(t *testing.T) {
	for _, address := range []string{"https://www.heliosschool.org/parent", "mailto:someone@example.org", "https://email.mail1.veracross.com/newsletter"} {
		if trackingLink(address) {
			t.Errorf("%q reads as a tracking link", address)
		}
	}
	for _, address := range []string{"https://email.mail1.veracross.com/c/eJxMzT1u"} {
		if !trackingLink(address) {
			t.Errorf("%q does not read as a tracking link", address)
		}
	}
	r := &LinkResolver{}
	if got := r.Resolve("https://example.org/page"); got != "https://example.org/page" {
		t.Fatalf("plain link: %q", got)
	}
}

func TestVeracrossLinksAreReadWithoutAsking(t *testing.T) {
	const wrapped = "https://email.mail1.veracross.com/c/eJxMzT1u7SAQhuHVQGnB8F9Q3MbbOIIBH9DlGAscrz9CKZJynnmlL3lpLKfZc2Md11oIS4tPwiiUGuOhdIyodU7WhATKpuisi7R6YKCZ4xxAWGU3riCZw0UjxRGMQSLZJ9TGtyePgKPPuWH_0ObLfV-TiH8EdgJ7wbCV3GqfNZyrILDT4dfx_mqtPnkQyX6KiaX3tvXxpo8H-uALW83n7f--F-e1_Lr-eyucXnDlMfv56wCMgfgOAAD__5zzUBI"
	r := &LinkResolver{}
	if got := r.Resolve(wrapped); got != "https://hca.heliosian.com/" {
		t.Fatalf("unwrapped: %q", got)
	}
	if r.Dropped != 0 {
		t.Fatalf("dropped %d", r.Dropped)
	}
	if strings.Contains(r.Resolve(wrapped), "veracross") {
		t.Fatal("the tracking address came back")
	}
}

func TestAnUnreadableTrackingLinkKeepsItsWords(t *testing.T) {
	r := &LinkResolver{}
	markdown, err := HTML(`<p>Please <a href="https://email.mail1.veracross.com/c/not-a-real-blob">sign up here</a> today.</p>`, r.Resolve)
	if err != nil {
		t.Fatal(err)
	}
	if markdown != "Please sign up here today." {
		t.Fatalf("markdown: %q", markdown)
	}
	if r.Dropped != 1 {
		t.Fatalf("dropped %d", r.Dropped)
	}
}

func TestInlineMarkupAroundBlocksKeepsItsWords(t *testing.T) {
	r := &LinkResolver{}
	markdown, err := HTML(`<a href="https://example.org/"><table><tr><td><p>Read the notice</p></td></tr><tr><td><p>Second row</p></td></tr></table></a><p>After.</p>`, r.Resolve)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Read the notice", "Second row", "After."} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}
	markdown, err = HTML(`<b><div><p>Bold block</p></div></b><p>Then this.</p>`, r.Resolve)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "Bold block") || !strings.Contains(markdown, "Then this.") {
		t.Fatalf("markdown: %q", markdown)
	}
}

func TestAPageIsItsMainAlone(t *testing.T) {
	markdown, err := HTML(`<html><body><nav><a href="/about">About</a></nav><main><h1>Aftercare</h1><p>Aftercare runs until six.</p></main><footer><p>Copyright Helios School</p></footer></body></html>`, (&LinkResolver{}).Resolve)
	if err != nil {
		t.Fatal(err)
	}
	if markdown != "# Aftercare\n\nAftercare runs until six." {
		t.Fatalf("markdown: %q", markdown)
	}
}

func TestNonBreakingSpacesAreSpaces(t *testing.T) {
	markdown, err := HTML("<p>Pick\u00a0up\u00a0at 3</p>", (&LinkResolver{}).Resolve)
	if err != nil {
		t.Fatal(err)
	}
	if markdown != "Pick up at 3" {
		t.Fatalf("markdown: %q", markdown)
	}
}

func TestOtherLinksAreKeptAsWritten(t *testing.T) {
	r := &LinkResolver{}
	for _, address := range []string{
		"https://anything.example.org/page?email=someone%40example.org",
		"http://10.0.0.1/c/l?email=x",
		"https://heliosns.bmetrack.com/c/l?u=DD88991",
		"https://email.mail1.veracross.com/newsletter?email=x",
	} {
		if got := r.Resolve(address); got != address {
			t.Errorf("%q became %q", address, got)
		}
	}
	if r.Dropped != 0 {
		t.Fatalf("dropped %d", r.Dropped)
	}
}

func TestListFootersAndNoticesAreLeftOff(t *testing.T) {
	markdown := Trim(strings.Join([]string{
		"Dear Friends,",
		"Please check the ingredients.",
		"Kristine",
		"This message (including any attachments) may contain confidential information intended for a specific individual.",
		"--",
		"You received this message because you are subscribed to the Google Groups \"chat\" group.\nTo unsubscribe from this group and stop receiving emails from it, send an email to [chat+unsubscribe@heliosns.org](mailto:chat+unsubscribe@heliosns.org).",
		"To view this discussion on the web visit https://groups.google.com/a/x/d/msgid/y",
	}, "\n\n"))
	if markdown != "Dear Friends,\n\nPlease check the ingredients.\n\nKristine" {
		t.Fatalf("trimmed:\n%s", markdown)
	}
	if strings.Contains(markdown, "unsubscribe") {
		t.Fatal("an unsubscribe address survived")
	}
}

func TestUtmMarksAreLeftOff(t *testing.T) {
	if got := cleanLink("https://sites.google.com/page?authuser=2&utm_source=BenchmarkEmail&utm_medium=email"); got != "https://sites.google.com/page?authuser=2" {
		t.Fatalf("clean: %q", got)
	}
}
