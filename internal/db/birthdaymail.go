package db

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const (
	birthdayJob       = "birthday mail"
	birthdaySheet     = "birthdayshared"
	BirthdaySharedTab = "Newsletter"
	lateAfterDays     = 2
)

var (
	birthdayBrand         = mail.Brand{Name: "Helios Staff Birthdays", Color: "#0e4d54", Tagline: "staff birthday donations"}
	remindHours           = [2]int{7, 21}
	BirthdaySharedColumns = []string{"Staff Name", "Staff Email", "Staff Birthday", "Target Newsletter Date", "Contacted On", "Charity Selected On", "Charity Name", "Charity Link", "Charity Blurb", "Note", "Preference"}
)

type BirthdayMailConfig struct {
	Sender *mail.Mailgun
	Origin func(app string) string
	Shared data.Writer
}

type BirthdayMailer struct {
	s       *Store
	config  BirthdayMailConfig
	in      *inbox
	kick    wake
	started atomic.Bool
}

func NewBirthdayMailer(s *Store, config BirthdayMailConfig) *BirthdayMailer {
	bm := &BirthdayMailer{s: s, config: config, in: s.Model().services.inbox(birthdayInbox), kick: newWake()}
	go bm.run()
	return bm
}

func (bm *BirthdayMailer) Start() {
	bm.started.Store(true)
	bm.s.OnTick(bm.kick.poke)
	bm.kick.poke()
}

func (bm *BirthdayMailer) run() {
	for {
		select {
		case <-bm.in.wake:
			bm.in.take()
		case <-bm.kick:
		}
		if bm.started.Load() {
			bm.pass(context.Background(), time.Now().In(School))
		}
	}
}

func (bm *BirthdayMailer) pass(ctx context.Context, now time.Time) {
	bm.invites(ctx, now)
	if h := now.Hour(); h >= remindHours[0] && h < remindHours[1] {
		bm.reminders(ctx, now)
	}
	bm.sendQueued(ctx)
	bm.weekly(ctx, now)
}

type staffBirthday struct {
	row, record, person store.Row
}

func (b staffBirthday) participation() string {
	return b.person["birthday_participation"]
}

func (b staffBirthday) done() bool {
	return b.record != nil && (b.record["published"] != "" || (b.record["charity"] != "" && b.participation() == "no_newsletter"))
}

func (b staffBirthday) assignee() string {
	if b.record == nil || b.participation() == "skip" || b.done() {
		return ""
	}
	return b.record["assigned_to"]
}

func (m *Model) birthdaysOf(year int) []staffBirthday {
	t, _ := Lookup("BIRTHDAY")
	out := []staffBirthday{}
	for _, row := range m.generated(t).rows {
		if row["year"] != strconv.Itoa(year) {
			continue
		}
		b := staffBirthday{row: row}
		b.person, _ = m.Table("PERSON").Get(row["person"])
		if row["record"] != "" {
			b.record, _ = m.Table("BIRTHDAY_YEAR").Get(row["record"])
		}
		out = append(out, b)
	}
	return out
}

func (m *Model) birthdayYearOf(now time.Time) int {
	start, err := time.Parse("01-02", m.Setting(birthdayApp, "Year Start"))
	if err != nil {
		panic("db: the birthday Year Start setting is not a month and day: " + err.Error())
	}
	return birthdaySchoolYear(now, start)
}

func (m *Model) birthdayMessages(record, kind string) []store.Row {
	out := []store.Row{}
	for _, msg := range m.Table("MESSAGE").Referencing("about", record) {
		if msg["direction"] == "out" && msg["kind"] == kind {
			out = append(out, msg)
		}
	}
	slices.SortFunc(out, func(a, b store.Row) int { return strings.Compare(a["created"], b["created"]) })
	return out
}

func (m *Model) birthdayInvitedTo(record string) string {
	sent := m.birthdayMessages(record, "calendar")
	if len(sent) == 0 {
		return ""
	}
	return m.recipientOf(sent[len(sent)-1]["id"])
}

func (m *Model) recipientOf(message string) string {
	for _, r := range m.Table("RECIPIENT").Referencing("message", message) {
		return r["person"]
	}
	return ""
}

func birthdayCopy(kind, record, person, subject string, now time.Time) []Edit {
	created := stamp(now)
	return []Edit{
		{Insert: "MESSAGE", As: "m", Row: map[string]any{"direction": "out", "kind": kind, "about": record, "subject": subject, "created": created}},
		{Insert: "RECIPIENT", Row: map[string]any{"message": "@m", "person": person, "created": created}},
	}
}

func (bm *BirthdayMailer) write(ctx context.Context, edits []Edit) error {
	if len(edits) == 0 {
		return nil
	}
	_, err := Write(ctx, bm.s, access.System(birthdayJob), Env{System: birthdayJob, Now: time.Now()}, Batch{Batch: edits})
	return err
}

func (bm *BirthdayMailer) invites(ctx context.Context, now time.Time) {
	m := bm.s.Model()
	for _, b := range m.birthdaysOf(m.birthdayYearOf(now)) {
		to := b.assignee()
		if to == "" || (b.record["invite_day"] == b.row["ask_by"] && m.birthdayInvitedTo(b.record["id"]) == to) {
			continue
		}
		edits := append(birthdayCopy("calendar", b.record["id"], to, m.birthdaySubject(b), now), Edit{Set: b.record["id"], Cells: map[string]any{"invite_day": b.row["ask_by"]}})
		if err := bm.write(ctx, edits); err != nil {
			slog.ErrorContext(ctx, "birthday mail: invite not recorded", "birthday", b.row["id"], "error", err)
		}
	}
}

type birthdayStep struct {
	kind string
	day  string
}

func birthdaySteps(b staffBirthday) []birthdayStep {
	steps := []birthdayStep{}
	if b.record["contacted"] == "" {
		ask, _ := dayOf(b.row["ask_by"])
		steps = append(steps, birthdayStep{"ask", b.row["ask_by"]}, birthdayStep{"late", ask.AddDate(0, 0, lateAfterDays).Format(time.DateOnly)})
	}
	if b.record["charity"] == "" {
		steps = append(steps, birthdayStep{"donation", b.row["due_by"]})
	}
	slices.SortStableFunc(steps, func(a, b birthdayStep) int { return strings.Compare(a.day, b.day) })
	return steps
}

func stepOn(b staffBirthday, day string) (birthdayStep, bool) {
	var due birthdayStep
	found := false
	for _, s := range birthdaySteps(b) {
		if s.day <= day {
			due, found = s, true
		}
	}
	return due, found
}

func (m *Model) issueDay(issue string) string {
	g, _ := m.Table("GROUP").Get(issue)
	day, _ := dayOf(g["start"])
	return day.Format(time.DateOnly)
}

func (bm *BirthdayMailer) reminders(ctx context.Context, now time.Time) {
	m := bm.s.Model()
	today := now.Format(time.DateOnly)
	for _, b := range m.birthdaysOf(m.birthdayYearOf(now)) {
		to := b.assignee()
		if to == "" || today > m.issueDay(b.row["newsletter"]) {
			continue
		}
		due, ok := stepOn(b, today)
		if !ok || slices.ContainsFunc(m.birthdayMessages(b.record["id"], "reminder"), func(r store.Row) bool { return dayPart(r["created"]) >= due.day }) {
			continue
		}
		if err := bm.write(ctx, birthdayCopy("reminder", b.record["id"], to, "Re: "+m.birthdaySubject(b), now)); err != nil {
			slog.ErrorContext(ctx, "birthday mail: reminder not recorded", "birthday", b.row["id"], "step", due.kind, "error", err)
		}
	}
}

func (bm *BirthdayMailer) sendQueued(ctx context.Context) {
	m := bm.s.Model()
	edits := []Edit{}
	for _, msg := range m.Table("MESSAGE").All() {
		if msg["direction"] != "out" || (msg["kind"] != "calendar" && msg["kind"] != "reminder") || !strings.HasPrefix(msg["about"], BirthdayYearPrefix) {
			continue
		}
		for _, r := range m.Table("RECIPIENT").Referencing("message", msg["id"]) {
			if r["sent"] != "" || r["failed"] != "" {
				continue
			}
			out, ok := bm.compose(m, msg, r["person"])
			if !ok {
				edits = append(edits, Edit{Set: r["id"], Cells: map[string]any{"failed": stamp(time.Now()), "detail": "the birthday is done or no longer theirs"}})
				continue
			}
			address := m.addressOf(r["person"])
			if address == "" {
				edits = append(edits, Edit{Set: r["id"], Cells: map[string]any{"sent": stamp(time.Now()), "detail": "no address to send to"}})
				continue
			}
			out.To = []string{address}
			sender := bm.config.Sender.From()
			if out.Headers["Message-Id"] == "" {
				out.Headers["Message-Id"] = mail.MessageID(r["id"], sender)
			}
			if err := bm.config.Sender.SendCopy(ctx, sender, []string{address}, []byte(mail.Compose(sender, out)), r["id"]); err != nil {
				slog.ErrorContext(ctx, "birthday mail: not sent", "recipient", r["id"], "error", err)
				continue
			}
			slog.InfoContext(ctx, "birthday mail: sent", "recipient", r["id"], "kind", msg["kind"], "about", msg["about"])
			edits = append(edits, Edit{Set: r["id"], Cells: map[string]any{"sent": stamp(time.Now())}})
		}
	}
	if err := bm.write(ctx, edits); err != nil {
		slog.ErrorContext(ctx, "birthday mail: sends not recorded", "copies", len(edits), "error", err)
	}
}

func (m *Model) birthdayOfRecord(record string) (staffBirthday, bool) {
	y, ok := m.Table("BIRTHDAY_YEAR").Get(record)
	if !ok {
		return staffBirthday{}, false
	}
	year, _ := strconv.Atoi(y["year"])
	for _, b := range m.birthdaysOf(year) {
		if b.row["record"] == record {
			return b, true
		}
	}
	return staffBirthday{}, false
}

func (m *Model) birthdaySubject(b staffBirthday) string {
	return fmt.Sprintf("Ask %s about their birthday charity", m.nameOf(b.row["person"]))
}

func (m *Model) birthdayThread(b staffBirthday) string {
	return fmt.Sprintf("<birthday-%s-%s-%d@heliosian.com>", m.addressOf(b.row["person"]), b.row["year"], atoi(b.row["year"])+1)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func (bm *BirthdayMailer) staffLink(m *Model, b staffBirthday) string {
	key := b.person["slug"]
	if key == "" {
		key = b.person["id"]
	}
	return bm.config.Origin("birthday") + "/staff/" + url.PathEscape(key)
}

func (m *Model) lastYearOf(b staffBirthday) store.Row {
	for _, y := range m.Table("BIRTHDAY_YEAR").Referencing("person", b.row["person"]) {
		if y["year"] == strconv.Itoa(atoi(b.row["year"])-1) && y["charity"] != "" {
			return y
		}
	}
	return nil
}

func (m *Model) charityName(id string) string {
	c, _ := m.Table("CHARITY").Get(id)
	return c["name"]
}

func longDay(cell string) string {
	t, _ := dayOf(cell)
	return t.Format("Monday, January 2, 2006")
}

func mediumDay(cell string) string {
	t, _ := dayOf(cell)
	return t.Format("January 2, 2006")
}

func (bm *BirthdayMailer) compose(m *Model, msg store.Row, to string) (mail.Message, bool) {
	b, ok := m.birthdayOfRecord(msg["about"])
	if !ok || b.assignee() != to {
		return mail.Message{}, false
	}
	if msg["kind"] == "calendar" {
		return bm.invite(m, b, to), true
	}
	step, ok := stepOn(b, dayPart(msg["created"]))
	if !ok {
		return mail.Message{}, false
	}
	return bm.reminder(m, b, step, to), true
}

func (bm *BirthdayMailer) invite(m *Model, b staffBirthday, to string) mail.Message {
	name := m.nameOf(b.row["person"])
	link := bm.staffLink(m, b)
	rows := [][2]string{{"Birthday", longDay(b.row["day"])}, {"Ask by", longDay(b.row["ask_by"])}, {"Newsletter", longDay(m.issueDay(b.row["newsletter"]))}}
	if last := m.lastYearOf(b); last != nil {
		words := m.charityName(last["charity"])
		if last["note"] != "" {
			words += " — " + last["note"]
		}
		rows = append(rows, [2]string{"Last year", words})
	}
	if b.participation() == "no_newsletter" {
		rows = append(rows, [2]string{"Note", "Asked to stay out of the newsletter"})
	}
	invites := m.birthdayMessages(b.record["id"], "calendar")
	moved := len(invites) > 1 && m.recipientOf(invites[len(invites)-2]["id"]) == to
	subject := m.birthdaySubject(b)
	l := mail.Letter{
		Brand: birthdayBrand, Base: bm.config.Origin("birthday"), Title: name, Path: link,
		Heading:  fmt.Sprintf("%s's birthday is yours", name),
		Intro:    fmt.Sprintf("%s's birthday is yours this year. The day to ask them is %s: reach out by then and record the charity they choose.", name, mediumDay(b.row["ask_by"])),
		Rows:     rows,
		Button:   "Open their birthday page",
		Footnote: "The invite attached puts the day to ask by on your calendar.",
	}
	thread := m.birthdayThread(b)
	headers := map[string]string{"Message-Id": thread}
	if moved {
		subject = "Re: " + subject
		headers = map[string]string{"In-Reply-To": thread, "References": thread}
		l.Heading = "The day to ask moved"
		l.Intro = fmt.Sprintf("The day to ask %s about their birthday charity moved, so here is the invite again: it is now %s.", name, mediumDay(b.row["ask_by"]))
	}
	description := fmt.Sprintf("Reach out to %s about the charity they would like the HCA to give to for their birthday, and record it on their page.\n\n", name)
	for _, row := range rows {
		description += fmt.Sprintf("%s: %s\n", row[0], row[1])
	}
	day, _ := dayOf(b.row["ask_by"])
	e := mail.Event{
		UID: fmt.Sprintf("birthday-%s-%s-%d@heliosian.com", m.addressOf(b.row["person"]), b.row["year"], atoi(b.row["year"])+1), Start: day, End: day.AddDate(0, 0, 1), AllDay: true,
		Summary: subject, Description: description + "\n" + link, URL: link,
		Organizer: mail.Person{Name: birthdayBrand.Name, Email: mail.AddressOf(bm.config.Sender.From())},
		Attendees: []mail.Person{{Email: m.addressOf(to)}}, Transparent: true, Alarm: 15 * time.Hour,
	}
	out := l.Message(subject, nil, nil, nil)
	out.Headers = headers
	out.Attachments = []mail.Attachment{mail.Calendar{Product: birthdayBrand.Name, Method: mail.MethodRequest, Stamp: time.Now(), Events: []mail.Event{e}}.Attachment()}
	return out
}

func (bm *BirthdayMailer) reminder(m *Model, b staffBirthday, step birthdayStep, to string) mail.Message {
	name := m.nameOf(b.row["person"])
	link := bm.staffLink(m, b)
	var text, htm strings.Builder
	p := func(t string) {
		text.WriteString(t + "\n\n")
		fmt.Fprintf(&htm, "<p style=\"font:16px/1.5 -apple-system,Segoe UI,Roboto,sans-serif\">%s</p>", html.EscapeString(t))
	}
	button := func(label, href string) {
		fmt.Fprintf(&text, "%s: %s\n\n", label, href)
		fmt.Fprintf(&htm, "<p style=\"margin:16px 0\"><a href=\"%s\" style=\"display:inline-block;padding:10px 18px;border-radius:8px;background:#0e4d54;color:#fff;font:700 15px -apple-system,Segoe UI,Roboto,sans-serif;text-decoration:none\">%s</a></p>", html.EscapeString(href), html.EscapeString(label))
	}
	switch step.kind {
	case "ask", "late":
		if step.kind == "ask" {
			p(fmt.Sprintf("Today is the day to ask %s which charity they would like the HCA to give to for their birthday (%s).", name, monthDay(b.row["day"])))
		} else {
			p(fmt.Sprintf("%s was due to be asked about their birthday charity on %s, and their outreach is not marked done. If you have asked, mark it done on their page; if not, here is the email, ready to send.", name, mediumDay(b.row["ask_by"])))
		}
		l := m.birthdayLetter(b, m.nameOf(to))
		button("Send the email", l.mailto())
		fmt.Fprintf(&text, "To: %s\nCC: %s\nSubject: %s\n\n%s\n\n", l.to, l.cc, l.subject, l.body)
		fmt.Fprintf(&htm, "<table style=\"font:14px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;border-collapse:collapse;color:#33474c\"><tr><td style=\"padding:2px 12px 2px 0;color:#647071\">To</td><td>%s</td></tr><tr><td style=\"padding:2px 12px 2px 0;color:#647071\">CC</td><td>%s</td></tr><tr><td style=\"padding:2px 12px 2px 0;color:#647071\">Subject</td><td>%s</td></tr></table>", html.EscapeString(l.to), html.EscapeString(l.cc), html.EscapeString(l.subject))
		fmt.Fprintf(&htm, "<blockquote style=\"margin:12px 0;padding:12px 16px;border-left:3px solid #cfdcde;background:#f4f8f8;font:15px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;white-space:pre-wrap\">%s</blockquote>", html.EscapeString(l.body))
		p("Once it is sent, mark the outreach done on their page so the team can see it.")
		button("Mark outreach done", link)
	case "donation":
		p(fmt.Sprintf("The %s newsletter is two days out and %s's charity is not recorded yet.", mediumDay(m.issueDay(b.row["newsletter"])), name))
		p("If they answered, record what they chose on their page. There is no need to ask again - whether to reply is their choice, and if they do not, the donation goes to " + m.charityName(m.Setting(birthdayApp, "Default Charity")) + ".")
		button("Record their charity", link)
	}
	thread := m.birthdayThread(b)
	return mail.Message{Subject: "Re: " + m.birthdaySubject(b), Text: strings.TrimSpace(text.String()), HTML: htm.String(), Headers: map[string]string{"In-Reply-To": thread, "References": thread}}
}

type birthdayLetter struct {
	to, cc, subject, body string
}

func (l birthdayLetter) mailto() string {
	q := url.Values{}
	if l.cc != "" {
		q.Set("cc", l.cc)
	}
	q.Set("subject", l.subject)
	q.Set("body", l.body)
	// url.Values writes spaces as +, which mail apps read as pluses.
	return "mailto:" + l.to + "?" + strings.ReplaceAll(q.Encode(), "+", "%20")
}

var (
	trailingSpace = regexp.MustCompile(`[ \t]+(\r?\n|$)`)
	manyBlank     = regexp.MustCompile(`\n{3,}`)
)

func (m *Model) birthdayLetter(b staffBirthday, sender string) birthdayLetter {
	lastYear := ""
	if last := m.lastYearOf(b); last != nil {
		lines := []string{"*Last Year's Charity*", m.charityName(last["charity"])}
		if last["note"] != "" {
			lines = append(lines, last["note"])
		}
		lastYear = strings.Join(lines, "\n")
	}
	fill := strings.NewReplacer(
		"{first name}", m.firstName(b.row["person"]),
		"{name}", m.nameOf(b.row["person"]),
		"{newsletter date}", mediumDay(m.issueDay(b.row["newsletter"])),
		"{birthday}", monthDay(b.row["day"]),
		"{default charity}", m.charityName(m.Setting(birthdayApp, "Default Charity")),
		"{sender}", sender,
		"{last year}", lastYear,
	).Replace
	note := ""
	if b.participation() == "no_newsletter" {
		note = fill(m.Setting(birthdayApp, "No Newsletter Note"))
	}
	body := m.Setting(birthdayApp, "Email Body")
	if strings.Contains(body, "{no newsletter note}") {
		body = strings.ReplaceAll(body, "{no newsletter note}", note)
	} else if note != "" {
		body += "\n\n" + note
	}
	body = trailingSpace.ReplaceAllString(strings.ReplaceAll(fill(body), "\r\n", "\n"), "$1")
	return birthdayLetter{
		to: m.addressOf(b.row["person"]), cc: m.Setting(birthdayApp, "Outreach CC"),
		subject: fill(m.Setting(birthdayApp, "Email Subject")), body: strings.TrimSpace(manyBlank.ReplaceAllString(body, "\n\n")),
	}
}

func monthDay(cell string) string {
	t, _ := dayOf(cell)
	return t.Format("January 2")
}

func lastCopyDay(now time.Time) time.Time {
	at := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 0, 0, School)
	for at.Weekday() != time.Thursday || at.After(now) {
		at = at.AddDate(0, 0, -1)
	}
	return at
}

func (m *Model) weekIssue(now time.Time) string {
	from := lastCopyDay(now)
	to := from.AddDate(0, 0, 7)
	for _, g := range m.Table("GROUP").Referencing("parent", m.Setting(birthdayApp, "Newsletter")) {
		if day := m.issueDay(g["id"]); day > from.Format(time.DateOnly) && day <= to.Format(time.DateOnly) {
			return g["id"]
		}
	}
	return ""
}

func (m *Model) fallbackCharity(b staffBirthday) (string, string) {
	if last := m.lastYearOf(b); last != nil {
		if c, ok := m.Table("CHARITY").Get(last["charity"]); ok && c["allowed"] == "Yes" {
			return last["charity"], last["note"]
		}
	}
	return m.Setting(birthdayApp, "Default Charity"), ""
}

func (bm *BirthdayMailer) weekly(ctx context.Context, now time.Time) {
	m := bm.s.Model()
	issue := m.weekIssue(now)
	if issue == "" {
		return
	}
	rows := []map[string]string{}
	edits := []Edit{}
	at := stamp(now)
	for _, b := range m.birthdaysOf(m.birthdayYearOf(now)) {
		if b.row["newsletter"] != issue || b.participation() == "skip" || (b.record != nil && b.record["published"] != "") {
			continue
		}
		charity, note, selected, contacted := "", "", "", ""
		cells := map[string]any{"published": at}
		if b.record != nil {
			charity, note, selected, contacted = b.record["charity"], b.record["note"], b.record["recorded"], b.record["contacted"]
		}
		if charity == "" {
			charity, note = m.fallbackCharity(b)
			cells["charity"], cells["note"], cells["recorded"] = charity, note, at
		}
		c, _ := m.Table("CHARITY").Get(charity)
		preference := ""
		if b.participation() == "no_newsletter" {
			preference = "No Newsletter"
		}
		rows = append(rows, map[string]string{
			"Staff Name": m.nameOf(b.row["person"]), "Staff Email": m.addressOf(b.row["person"]), "Staff Birthday": b.row["day"],
			"Target Newsletter Date": m.issueDay(issue), "Contacted On": dayPart(contacted), "Charity Selected On": dayPart(selected),
			"Charity Name": c["name"], "Charity Link": c["url"], "Charity Blurb": c["description"], "Note": note, "Preference": preference,
		})
		if b.record == nil {
			cells["person"], cells["year"] = b.row["person"], b.row["year"]
			edits = append(edits, Edit{Insert: "BIRTHDAY_YEAR", Row: cells})
			continue
		}
		edits = append(edits, Edit{Set: b.record["id"], Cells: cells})
	}
	if len(rows) == 0 {
		return
	}
	if err := bm.config.Shared.Insert(birthdaySheet, BirthdaySharedTab, rows); err != nil {
		slog.ErrorContext(ctx, "birthday mail: weekly copy not written to the shared sheet", "issue", issue, "error", err)
		return
	}
	if err := bm.write(ctx, edits); err != nil {
		slog.ErrorContext(ctx, "birthday mail: weekly copy written but not recorded", "issue", issue, "rows", len(rows), "error", err)
		return
	}
	slog.InfoContext(ctx, "birthday mail: weekly copy", "issue", issue, "rows", len(rows))
}

func dayPart(cell string) string {
	if len(cell) < len(time.DateOnly) {
		return cell
	}
	return cell[:len(time.DateOnly)]
}
