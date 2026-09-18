package artifacts

import (
	"context"
	"strings"
	"testing"
)

const samples = "../../sampledata/artifacts"

func sampleIssue(t *testing.T) *Document {
	t.Helper()
	message, err := ReadMessage(samples + "/2026-09-11-newsletter-sep-11.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Build(message, NewResolver(), Fake{}.Model())
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestMessageBecomesMarkdown(t *testing.T) {
	doc := sampleIssue(t)
	if doc.Title != "Helios Weekly Newsletter 2026 Sep 11" || doc.Date != "2026-09-11" || doc.Author != "Helios School" {
		t.Fatalf("headers: %+v", doc)
	}
	if doc.Kind != KindNewsletter || doc.Channel != "newsletter" || doc.Source != "mail:sample-newsletter-2026-09-11@example.org" {
		t.Fatalf("provenance: %+v", doc)
	}
	for _, want := range []string{
		"# IN THIS NEWSLETTER\n\n- **A Note from Ben**\n- **Announcements & Reminders**",
		"*Question of the week:* What are our Jays building in the maker space this week? **Scroll to the end for the answer!**",
		"## Picture Day: Tuesday, 9/22",
		"[Sign up here.](https://team.heliosian.com/v/international-night) A booth entails",
		"[contact Karn](mailto:karn.knight@example.org).",
		"- Saturday, 9/19: [Fondue & Fort Night](https://celebrate.heliosian.com/): fancy fondue",
		"\n---\n",
		"Sunnyvale, CA 94086\n[www.heliosschool.org](https://www.heliosschool.org)",
	} {
		if !strings.Contains(doc.Markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, doc.Markdown)
		}
	}
	for _, unwanted := range []string{"pixel.gif", "margin: 0", "<", "  "} {
		if strings.Contains(doc.Markdown, unwanted) {
			t.Errorf("markdown holds %q:\n%s", unwanted, doc.Markdown)
		}
	}
}

// A message with no HTML part is its plain text, with the hard wrapping a
// mail client put in joined back into paragraphs.
func TestPlainTextMessageKeepsItsParagraphs(t *testing.T) {
	message, err := ReadMessage(samples + "/2026-08-30-chat-nut-free.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Build(message, NewResolver(), Fake{}.Model())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Kind != KindList || doc.Channel != "chat" {
		t.Fatalf("provenance: %+v", doc)
	}
	for _, want := range []string{
		"A reminder before the bake sale on Friday: Helios is a nut free campus, which covers peanuts and tree nuts,",
		"- no nuts, and no \"may contain\" labels\n- write the ingredients on the plate",
	} {
		if !strings.Contains(doc.Markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, doc.Markdown)
		}
	}
}

func TestChunksFollowTheHeadings(t *testing.T) {
	doc := sampleIssue(t)
	sections := []string{}
	for _, c := range doc.Chunks {
		sections = append(sections, c.Section)
	}
	want := []string{"", "IN THIS NEWSLETTER", "A NOTE FROM BEN", "ANNOUNCEMENTS & REMINDERS › Virtual Math Back to School Afternoon: Thursday, 9/17", "ANNOUNCEMENTS & REMINDERS › Picture Day: Tuesday, 9/22", "HCA NEWSLETTER", "LIBRARY CORNER"}
	if strings.Join(sections, "|") != strings.Join(want, "|") {
		t.Fatalf("sections: %q", sections)
	}
	head := "Helios Weekly Newsletter 2026 Sep 11, September 11, 2026 (newsletter) › HCA NEWSLETTER\n\n"
	if !strings.HasPrefix(doc.embedText(doc.Chunks[5]), head) {
		t.Fatalf("embed text: %q", doc.embedText(doc.Chunks[5]))
	}
}

func TestMixedCaseHeadingsNestUnderCapitals(t *testing.T) {
	chunks := Chunks("# HCA NEWSLETTER\n\nJoin us.\n\n# Mark These Dates\n\n- Saturday\n\n# LIBRARY CORNER\n\nBooks.\n\n## Volunteers\n\nSign up.")
	sections := []string{}
	for _, c := range chunks {
		sections = append(sections, c.Section)
	}
	want := "HCA NEWSLETTER|HCA NEWSLETTER › Mark These Dates|LIBRARY CORNER|LIBRARY CORNER › Volunteers"
	if strings.Join(sections, "|") != want {
		t.Fatalf("sections: %q", sections)
	}
}

func TestLongSectionsAreCutAtParagraphs(t *testing.T) {
	paragraph := strings.Repeat("word ", 500)
	markdown := "# Long\n\n" + strings.TrimSpace(paragraph) + "\n\n" + strings.TrimSpace(paragraph) + "\n\n" + strings.TrimSpace(paragraph)
	chunks := Chunks(markdown)
	if len(chunks) != 2 || chunks[0].Section != "Long" || len(chunks[0].Text) > maxChunkChars || !strings.HasSuffix(chunks[0].Text, "word") {
		t.Fatalf("chunks: %d, first %d characters", len(chunks), len(chunks[0].Text))
	}
}

func TestObjectNamesFollowTheRenderedDocument(t *testing.T) {
	doc := sampleIssue(t)
	before := doc.Object()
	if !strings.HasPrefix(before, Folder+"/"+doc.Key+"-") || before != Folder+"/"+doc.ObjectFile() {
		t.Fatalf("object: %s", before)
	}
	if err := doc.Embed(context.Background(), Fake{}); err != nil {
		t.Fatal(err)
	}
	if doc.Object() != before {
		t.Fatal("embedding changed the object name")
	}
	doc.Chunks[0].Text += " changed"
	if doc.Object() == before {
		t.Fatal("a changed chunk kept the object name")
	}
}

// A vector rides in the object as base64 float32, so a corpus of tens of
// thousands of chunks is megabytes rather than gigabytes.
func TestVectorsRoundTripAsBase64(t *testing.T) {
	doc := sampleIssue(t)
	if err := doc.Embed(context.Background(), Fake{}); err != nil {
		t.Fatal(err)
	}
	encoded, err := doc.Chunks[0].Vector.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > len(doc.Chunks[0].Vector)*6 {
		t.Fatalf("%d floats took %d bytes", len(doc.Chunks[0].Vector), len(encoded))
	}
	back := Vector{}
	if err := back.UnmarshalJSON(encoded); err != nil {
		t.Fatal(err)
	}
	if len(back) != len(doc.Chunks[0].Vector) {
		t.Fatalf("%d floats came back as %d", len(doc.Chunks[0].Vector), len(back))
	}
	for i := range back {
		if back[i] != doc.Chunks[0].Vector[i] {
			t.Fatalf("float %d: %v became %v", i, doc.Chunks[0].Vector[i], back[i])
		}
	}
}

func TestSearchRanksTheMatchingChunkFirst(t *testing.T) {
	m, err := LoadDir(samples, Fake{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Documents) != 3 || m.Documents[0].Date != "2026-09-11" {
		t.Fatalf("documents: %d, first %s", len(m.Documents), m.Documents[0].Date)
	}
	if oldest, newest := m.Span(); oldest != "2026-08-30" || newest != "2026-09-11" {
		t.Fatalf("span: %s to %s", oldest, newest)
	}
	first := func(query string) Hit {
		t.Helper()
		vectors, err := Fake{}.Embed(context.Background(), []string{query}, true)
		if err != nil {
			t.Fatal(err)
		}
		hits := m.Search(vectors[0], query, 3)
		if len(hits) == 0 {
			t.Fatalf("nothing found for %q", query)
		}
		return hits[0]
	}
	hit := first("picture day order forms")
	if hit.Document.Chunks[hit.Index].Section != "ANNOUNCEMENTS & REMINDERS › Picture Day: Tuesday, 9/22" {
		t.Fatalf("picture day: %+v", hit.Document.Chunks[hit.Index])
	}
	if hit = first("are nuts allowed at the bake sale"); hit.Document.Channel != "chat" {
		t.Fatalf("nuts: %s %q", hit.Document.Channel, hit.Document.Title)
	}
}

// A reply carries the message it answers, so the same words are in the
// corpus more than once and the search should offer them once.
func TestAQuotedPassageIsReturnedOnce(t *testing.T) {
	original := "The bake sale is on Friday in the courtyard and every plate needs its ingredients written on it so families with allergies can read them before their children choose."
	model := Fake{}.Model()
	m := &Model{Documents: []*Document{
		{Key: "a", Title: "Bake sale", Date: "2026-09-01", Model: model, Chunks: []Chunk{{Text: original}}},
		{Key: "b", Title: "Re: Bake sale", Date: "2026-09-02", Model: model, Chunks: []Chunk{{Text: "Thanks for organising!\n\n" + original}}},
		{Key: "c", Title: "Picture day", Date: "2026-09-03", Model: model, Chunks: []Chunk{{Text: "Picture day is Tuesday and order forms go home on Monday."}}},
	}}
	for _, d := range m.Documents {
		if err := d.Embed(context.Background(), Fake{}); err != nil {
			t.Fatal(err)
		}
		if err := d.normalize(); err != nil {
			t.Fatal(err)
		}
	}
	vectors, err := Fake{}.Embed(context.Background(), []string{"bake sale ingredients allergies"}, true)
	if err != nil {
		t.Fatal(err)
	}
	hits := m.Search(vectors[0], "bake sale ingredients allergies", 5)
	keys := []string{}
	for _, hit := range hits {
		keys = append(keys, hit.Document.Key)
	}
	if len(hits) != 2 {
		t.Fatalf("hits: %v", keys)
	}
}

func TestChannelsAndDatesNarrowTheSearch(t *testing.T) {
	m, err := LoadDir(samples, Fake{})
	if err != nil {
		t.Fatal(err)
	}
	if names := m.ChannelNames(); strings.Join(names, ",") != "newsletter,chat" {
		t.Fatalf("channels: %v", names)
	}
	if only := m.InChannel("chat"); len(only.Documents) != 1 || only.Documents[0].Kind != KindList {
		t.Fatalf("chat: %d", len(only.Documents))
	}
	if since := m.Between("2026-09-05", ""); len(since.Documents) != 1 || since.Documents[0].Date != "2026-09-11" {
		t.Fatalf("since: %d", len(since.Documents))
	}
	if until := m.Between("", "2026-09-04"); len(until.Documents) != 2 {
		t.Fatalf("until: %d", len(until.Documents))
	}
	if none := m.Between("2027-01-01", ""); len(none.Documents) != 0 {
		t.Fatalf("next year: %d", len(none.Documents))
	}
}

func TestTrackingLinksAreTheOnlyOnesTouched(t *testing.T) {
	for _, address := range []string{"https://www.heliosschool.org/parent", "mailto:someone@example.org", "https://email.mail1.veracross.com/newsletter"} {
		if tracking(address) {
			t.Errorf("%q reads as a tracking link", address)
		}
	}
	for _, address := range []string{"https://email.mail1.veracross.com/c/eJxMzT1u", "https://heliosns.bmetrack.com/c/l?u=DD88991"} {
		if !tracking(address) {
			t.Errorf("%q does not read as a tracking link", address)
		}
	}
	r := NewResolver()
	if got := r.Resolve("https://example.org/page"); got != "https://example.org/page" {
		t.Fatalf("plain link: %q", got)
	}
}

// A Veracross link carries where it goes inside itself, so it is turned back
// with no request at all - and the address of the person it was sent to,
// which is exactly why it must never be handed to a reader.
func TestVeracrossLinksAreReadWithoutAsking(t *testing.T) {
	const wrapped = "https://email.mail1.veracross.com/c/eJxMzT1u7SAQhuHVQGnB8F9Q3MbbOIIBH9DlGAscrz9CKZJynnmlL3lpLKfZc2Md11oIS4tPwiiUGuOhdIyodU7WhATKpuisi7R6YKCZ4xxAWGU3riCZw0UjxRGMQSLZJ9TGtyePgKPPuWH_0ObLfV-TiH8EdgJ7wbCV3GqfNZyrILDT4dfx_mqtPnkQyX6KiaX3tvXxpo8H-uALW83n7f--F-e1_Lr-eyucXnDlMfv56wCMgfgOAAD__5zzUBI"
	r := NewResolver()
	if got := r.Resolve(wrapped); got != "https://hca.heliosian.com/" {
		t.Fatalf("unwrapped: %q", got)
	}
	if r.Dropped != 0 {
		t.Fatalf("dropped %d", r.Dropped)
	}
	// Nothing about the link survives into the answer.
	if strings.Contains(r.Resolve(wrapped), "veracross") {
		t.Fatal("the tracking address came back")
	}
}

// A tracking link nobody can turn back keeps its words and loses its
// address, rather than failing the whole message.
func TestAnUnreadableTrackingLinkKeepsItsWords(t *testing.T) {
	r := NewResolver()
	markdown, err := Markdown(`<p>Please <a href="https://email.mail1.veracross.com/c/not-a-real-blob">sign up here</a> today.</p>`, r.Resolve)
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

// Mail templates wrap links and bold around whole tables. The words inside
// have to survive, laid out as the blocks they are.
func TestInlineMarkupAroundBlocksKeepsItsWords(t *testing.T) {
	r := NewResolver()
	markdown, err := Markdown(`<a href="https://example.org/"><table><tr><td><p>Read the notice</p></td></tr><tr><td><p>Second row</p></td></tr></table></a><p>After.</p>`, r.Resolve)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Read the notice", "Second row", "After."} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}
	markdown, err = Markdown(`<b><div><p>Bold block</p></div></b><p>Then this.</p>`, r.Resolve)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "Bold block") || !strings.Contains(markdown, "Then this.") {
		t.Fatalf("markdown: %q", markdown)
	}
}

// The older mailer's redirects wear the school's own name and carry the
// reader's address; its unsubscribe and view-in-browser links go nowhere a
// reader should be sent at all.
func TestTheMailersVanityRedirectsAreCaught(t *testing.T) {
	for _, address := range []string{
		"http://r560896.heliosschool.org/c/l?u=F6A18CC&e=163F978&email=ARXc6ueZ8",
		"http://r560896.heliosschool.org/c/su?e=163F978&email=ARXc6ueZ8",
		"https://heliosns.bmetrack.com/c/v?e=145E257",
		"https://anything.example.org/page?email=someone%40example.org",
	} {
		if !tracking(address) {
			t.Errorf("%q does not read as a tracking link", address)
		}
	}
	r := NewResolver()
	for _, address := range []string{
		"http://r560896.heliosschool.org/c/su?e=163F978&email=ARXc6ueZ8",
		"http://r560896.heliosschool.org/c/v?e=163F978&email=ARXc6ueZ8",
	} {
		if got := r.Resolve(address); got != "" {
			t.Errorf("%q resolved to %q rather than being dropped", address, got)
		}
	}
}

// A template that sets its sections in bold capitals gets headings, so the
// chunker can cut the newsletter into its sections.
func TestBoldCapitalsBecomeHeadings(t *testing.T) {
	markdown, err := Markdown(`<p><strong>A NOTE FROM BEN</strong></p><p>Dear Helios Families,</p><p><b><a href="https://example.org/map">HELIOS WORLD COMMUNITY MAP</a></b></p><p>Instructions below.</p><p><strong>Ben</strong></p>`, NewResolver().Resolve)
	if err != nil {
		t.Fatal(err)
	}
	want := "# A NOTE FROM BEN\n\nDear Helios Families,\n\n# HELIOS WORLD COMMUNITY MAP\n\nInstructions below.\n\n**Ben**"
	if markdown != want {
		t.Fatalf("markdown:\n%s", markdown)
	}
	sections := []string{}
	for _, c := range Chunks(markdown) {
		sections = append(sections, c.Section)
	}
	if strings.Join(sections, "|") != "A NOTE FROM BEN|HELIOS WORLD COMMUNITY MAP" {
		t.Fatalf("sections: %q", sections)
	}
}

// What a mailing list adds around a message is the machine's words, and
// would otherwise be searched and shown like anybody's.
func TestListFootersAndNoticesAreLeftOff(t *testing.T) {
	markdown := trim(strings.Join([]string{
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

// A message names its own channel when one was recorded with it, and is
// placed by what it carries when none was - which is how mail arriving
// later is placed. Nothing a committee kept to itself is ever placed.
func TestMessagesArePlacedByWhatTheyCarry(t *testing.T) {
	for _, c := range []struct {
		what          string
		message       Message
		channel, kind string
		belongs       bool
	}{
		{"a class list", Message{ListID: "<jays.parents.heliosschool.org>"}, "jays.parents", KindList, true},
		{"the old domain", Message{ListID: "<hummingbirds.parents.heliosns.org>"}, "hummingbirds.parents", KindList, true},
		{"a list with a description", Message{ListID: "Chat <chat.heliosschool.org>"}, "chat", KindList, true},
		{"the newsletter's mailer", Message{Sender: "m@mail1.veracross.com"}, "newsletter", KindNewsletter, true},
		{"the former mailer", Message{ListID: "<3064358178.560896@benchmarkemail.com>"}, "newsletter", KindNewsletter, true},
		{"a broadcast address", Message{To: []string{"parentsandstaff@heliosschool.org"}}, "parentsandstaff", KindAnnouncement, true},
		{"the board", Message{ListID: "<boardoftrustees.heliosschool.org>"}, "", "", false},
		{"a board committee", Message{ListID: "<board-finance.heliosschool.org>"}, "", "", false},
		{"an HCA team", Message{ListID: "<hca-social-events-team.heliosns.org>"}, "", "", false},
		{"the room parents", Message{ListID: "<room.parents.heliosns.org>"}, "", "", false},
		{"one person", Message{To: []string{"gayle.mcdowell@heliosschool.org"}}, "", "", false},
		{"mail to the association", Message{To: []string{"hca@heliosschool.org"}}, "", "", false},
		{"what it was saved as", Message{Channel: "chat", Kind: KindList, ListID: "<boardoftrustees.heliosschool.org>"}, "chat", KindList, true},
	} {
		channel, kind, ok := c.message.Broadcast()
		if ok != c.belongs || (ok && (channel != c.channel || kind != c.kind)) {
			t.Errorf("%s: %q %q %v, wanted %q %q %v", c.what, channel, kind, ok, c.channel, c.kind, c.belongs)
		}
	}
}

func TestUtmMarksAreLeftOff(t *testing.T) {
	if got := clean("https://sites.google.com/page?authuser=2&utm_source=BenchmarkEmail&utm_medium=email"); got != "https://sites.google.com/page?authuser=2" {
		t.Fatalf("clean: %q", got)
	}
}
