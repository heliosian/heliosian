package artifacts

import (
	"context"
	"errors"
	"os"
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
	if doc.Kind != KindNewsletter || doc.Channel != "newsletter" || doc.Source != "mail:sample-newsletter-2026-09-11@example.org" || doc.URL() != "" {
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

func TestPageBecomesMarkdown(t *testing.T) {
	saved, err := ReadSaved(samples + "/2026-09-01-page-family-camping.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := saved.(Page); !ok {
		t.Fatalf("read as %T", saved)
	}
	doc, err := saved.Build(NewResolver(), Fake{}.Model())
	if err != nil {
		t.Fatal(err)
	}
	const url = "https://www.heliosschool.org/student-life/family-camping"
	if doc.Title != "Family Camping" || doc.Date != "2026-09-01" || doc.Author != "Helios School" {
		t.Fatalf("headers: %+v", doc)
	}
	if doc.Kind != KindPage || doc.Source != url || doc.URL() != url || doc.Key != Key(url) || saved.Key() != doc.Key {
		t.Fatalf("provenance: %+v", doc)
	}
	for _, want := range []string{
		"Every fall the whole school camps together at [Family Camping Weekend](https://www.heliosschool.org/fs/pages/749), two nights",
		"## What to Bring\n\nA tent, sleeping bags",
		"## Meals\n\nSaturday dinner is a potluck around the campfire, and the eighth graders run the s’mores.",
	} {
		if !strings.Contains(doc.Markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, doc.Markdown)
		}
	}
	for _, unwanted := range []string{"# Family Camping", "About Helios", "Student Life", "In This Section", "Voice", "Sunnyvale", "#fs-panel"} {
		if strings.Contains(doc.Markdown, unwanted) {
			t.Errorf("markdown holds %q:\n%s", unwanted, doc.Markdown)
		}
	}
	if n := strings.Count(doc.Markdown, "What to Bring"); n != 1 {
		t.Errorf("the panel's title appears %d times:\n%s", n, doc.Markdown)
	}
}

func TestExcludedPagesAreRefused(t *testing.T) {
	for _, address := range []string{
		"https://www.heliosschool.org/about/staff-and-faculty",
		"https://www.heliosschool.org/school-calendar",
		"https://www.heliosschool.org/website-instructions",
		"https://www.heliosschool.org/login/",
	} {
		if !Excluded(address) {
			t.Errorf("%s is not excluded", address)
		}
		if _, err := BuildPage(Page{URL: address, HTML: "<main><p>Words.</p></main>"}, Fake{}.Model()); !errors.Is(err, ErrExcluded) {
			t.Errorf("%s: %v", address, err)
		}
	}
	for _, address := range []string{"https://www.heliosschool.org/student-life/camping", "https://www.heliosschool.org"} {
		if Excluded(address) {
			t.Errorf("%s is excluded", address)
		}
	}
}

func TestPortalResourceBecomesMarkdown(t *testing.T) {
	r := Resource{
		URL:     "https://portals.veracross.com/heliosschool/parent/pages/School-Lunch",
		Title:   "School Lunch",
		Fetched: "2026-09-17T19:00:00Z",
		Format:  FormatHTML,
		Body:    `<h2>Ordering</h2><p>Order by Thursday on <a href="https://www.google.com/url?q=https://www.choicelunch.com/&amp;sa=D&amp;usg=x">Choicelunch</a>, and see <a href="/heliosschool/parent/pages/Aftercare-Program">aftercare</a>.</p>`,
	}
	doc, err := r.Build(NewResolver(), Fake{}.Model())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "School Lunch" || doc.Date != "2026-09-17" || doc.Kind != KindPortal || doc.URL() != r.URL || doc.Key != Key(r.URL) {
		t.Fatalf("headers: %+v", doc)
	}
	want := "## Ordering\n\nOrder by Thursday on [Choicelunch](https://www.choicelunch.com/), and see [aftercare](https://portals.veracross.com/heliosschool/parent/pages/Aftercare-Program)."
	if doc.Markdown != want {
		t.Fatalf("markdown:\n%s", doc.Markdown)
	}
}

func TestTextResourceKeepsItsLines(t *testing.T) {
	r := Resource{URL: "https://docs.google.com/presentation/d/x/edit", Title: "SEL", Fetched: "2026-09-17T19:00:00Z", Format: FormatText,
		Body: "Day in the Life\r\nSEL skills   matter.\n\n1\n\nEssential Skills\nAsk for help\n\fRespect others\n"}
	doc, err := r.Build(NewResolver(), Fake{}.Model())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Markdown != "Day in the Life\nSEL skills matter.\n\nEssential Skills\nAsk for help\n\nRespect others" {
		t.Fatalf("markdown:\n%q", doc.Markdown)
	}
	if _, err := (Resource{URL: r.URL, Title: "Empty", Fetched: r.Fetched, Format: FormatText, Body: "\f 2 \n"}).Build(NewResolver(), Fake{}.Model()); !errors.Is(err, ErrNoWords) {
		t.Fatalf("an empty resource: %v", err)
	}
}

func TestSavedFilesAreToldApart(t *testing.T) {
	dir := t.TempDir()
	resource := dir + "/resource.json"
	if err := os.WriteFile(resource, []byte(`{"url":"https://example.org/a","title":"A","fetched":"2026-09-17T19:00:00Z","format":"text","body":"Words."}`), 0o644); err != nil {
		t.Fatal(err)
	}
	saved, err := ReadSaved(resource)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := saved.(Resource); !ok {
		t.Fatalf("read as %T", saved)
	}
	page, err := ReadSaved(samples + "/2026-09-01-page-family-camping.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := page.(Page); !ok {
		t.Fatalf("read as %T", page)
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
	m := sampleModel(t)
	if len(m.Documents) != 4 || m.Documents[0].Date != "2026-09-11" {
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
	if hit = first("what to bring for family camping tents"); hit.Document.Kind != KindPage {
		t.Fatalf("camping: %s %q", hit.Document.Kind, hit.Document.Title)
	}
}

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

func TestDatesNarrowTheSearch(t *testing.T) {
	m := sampleModel(t)
	if since := m.Between("2026-09-05", ""); len(since.Documents) != 1 || since.Documents[0].Date != "2026-09-11" {
		t.Fatalf("since: %d", len(since.Documents))
	}
	if until := m.Between("", "2026-09-04"); len(until.Documents) != 3 {
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

func TestVeracrossLinksAreReadWithoutAsking(t *testing.T) {
	const wrapped = "https://email.mail1.veracross.com/c/eJxMzT1u7SAQhuHVQGnB8F9Q3MbbOIIBH9DlGAscrz9CKZJynnmlL3lpLKfZc2Md11oIS4tPwiiUGuOhdIyodU7WhATKpuisi7R6YKCZ4xxAWGU3riCZw0UjxRGMQSLZJ9TGtyePgKPPuWH_0ObLfV-TiH8EdgJ7wbCV3GqfNZyrILDT4dfx_mqtPnkQyX6KiaX3tvXxpo8H-uALW83n7f--F-e1_Lr-eyucXnDlMfv56wCMgfgOAAD__5zzUBI"
	r := NewResolver()
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
