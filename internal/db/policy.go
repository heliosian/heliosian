package db

import (
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/store"
)

const policySource = `
;; Definitions

; the viewer is an accepted lead of @g or of a group above it
(define (leads @g)
  (exists MEMBER (in group (ancestors @g)) (= person @viewer) (= role "lead") (= status "yes")))

; the members of every family @p leads
(define (household @p)
  (select MEMBER.person
    (in group (select MEMBER.group (= person @p) (= role "lead") (= group.kind "family")))
    (= role "member")))

; every group @p has a membership row in
(define (groups_of @p)
  (select MEMBER.group (= person @p)))

; the viewer is in effect a member of the app's admins group
(define (admin_of app)
  (exists EFFECTIVE_MEMBER (in group (select APP.admins (= key app))) (= person @viewer)))

; the viewer is in effect a member of super-admins
(define (super_admin)
  (exists EFFECTIVE_MEMBER (= group.slug "super-admins") (= person @viewer)))

; @g is not closed, and the viewer leads it or its visibility lets them see it
(define (visible @g)
  (and (!= @g.status "closed")
       (or (leads @g)
           (and (not (in @g.status "pending" "hidden"))
                (or (in @g.visibility "everyone" "unlisted")
                    (and (= @g.visibility "members")
                         (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer)))
                    (and (= @g.visibility "group")
                         (exists EFFECTIVE_MEMBER (= group @g.visible_to) (= person @viewer))))))))

; the viewer may see @g, and @g lets them see its members
(define (sees_members @g)
  (and (visible @g)
       (or (leads @g)
           (blank @g.members_visible)
           (= @g.members_visible "everyone")
           (and (= @g.members_visible "members")
                (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer))))))

; @p is the viewer, a guest in one of the viewer's groups, or anyone else not hidden and active; the consent step has already removed everyone unconsented
(define (person_visible @p)
  (or (= @p @viewer)
      (and (= @p.source "guest")
           (exists MEMBER (in group (groups_of @viewer)) (= person @p)))
      (and (!= @p.source "guest") (not @p.hidden) (blank @p.deactivated))))

; the viewer is @p or a lead of @p's family
(define (self_or_household @p)
  (or (= @p @viewer) (in @p (household @viewer))))

; the viewer leads @g, or is in it and it is not hidden
(define (sees_mail @g)
  (or (leads @g)
      (and (!= @g.status "hidden")
           (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer)))))

; @d was sent to no list, or to a list whose mail the viewer sees
(define (document_visible @d)
  (or (not (exists DOCUMENT_GROUP (= document @d) (= relation "sent_to") (= group.kind "list")))
      (exists DOCUMENT_GROUP (= document @d) (= relation "sent_to") (= group.kind "list") (sees_mail group))))

;; Everyone

; people the viewer may see
(read PERSON (person_visible @row))
; whether the form shares a person's address, to them and their family's leads
(read PERSON.address_consent (self_or_household @row))
; whether the form shares a person's phone, to them and their family's leads
(read PERSON.phone_consent (self_or_household @row))
; a person or a lead of their family overrides their long name
(set PERSON.name_long_override (self_or_household @old))
; a person or a lead of their family overrides their short name
(set PERSON.name_short_override (self_or_household @old))
; a person or a lead of their family overrides their sort name
(set PERSON.name_sort_override (self_or_household @old))
; the emails of people the viewer may see
(read PERSON_EMAIL (person_visible person))
; the photos of people the viewer may see
(read PERSON_PHOTO (person_visible person))
; the viewer's own app settings
(read PERSON_SETTING (= person @viewer))
; the birthday years of people the viewer may see
(read BIRTHDAY_YEAR (person_visible person))
; the viewer's own saved calendars
(read SAVED_VIEW (= person @viewer))
; the viewer's own bug reports and ideas
(read REPORT (= reporter @viewer))

; groups the viewer may see
(read GROUP (visible @row))
; whether a family's adults all share their address, to its members
(read GROUP.address_consent (exists MEMBER (= group @row) (= person @viewer)))
; whether a family's adults all share their phone, to its members
(read GROUP.phone_consent (exists MEMBER (= group @row) (= person @viewer)))
; where the calendar groups the viewer may see came from
(read GROUP_SOURCE (visible group))
; the categories of groups the viewer may see
(read GROUP_CATEGORY (visible group))
; the rules of groups the viewer leads
(read RULE (leads group))
; the viewer's own, household's and guests' memberships, those in groups they lead, visible groups' leads, and members where shown
(read MEMBER
  (or (self_or_household person)
      (= guest_of @viewer)
      (leads group)
      (and (= role "lead") (visible group))
      (and (= role "member") (sees_members group))))
; what a membership cost, to the member, their host and the group's leads
(read MEMBER.price (or (= person @viewer) (= guest_of @viewer) (leads group)))
; a membership's payment reference, to the member, their host and the group's leads
(read MEMBER.purchase_id (or (= person @viewer) (= guest_of @viewer) (leads group)))
; a person, their family's lead or their host answers an invitation; the group's leads set any status
(set MEMBER.status
  (or (and (or (self_or_household @old.person)
               (= @old.guest_of @viewer))
           (in @new.status "yes" "maybe" "no" "cancelled"))
      (leads @old.group)))
; the viewer's own and household's effective memberships, and members of groups that show them
(read EFFECTIVE_MEMBER
  (or (self_or_household person)
      (sees_members group)))

; documents sent to no list, or to a list whose mail the viewer sees
(read DOCUMENT (document_visible @row))
; links between a document and a group, where the viewer may see both
(read DOCUMENT_GROUP (and (document_visible document) (visible group)))

; mail the viewer sent or received, and mail to groups they lead
(read MESSAGE
  (or (= from_person @viewer)
      (exists RECIPIENT (= message @row) (= person @viewer))
      (leads group)))
; the viewer's own deliveries, deliveries of mail they sent, and of mail to groups they lead
(read RECIPIENT
  (or (= person @viewer)
      (= message.from_person @viewer)
      (leads message.group)))
; a delivery's link token, to its recipient alone
(read RECIPIENT.token (= person @viewer))

; app settings
(read SETTING true)
; categories open to everyone or to a group the viewer is in
(read CATEGORY
  (or (blank visible_to)
      (exists EFFECTIVE_MEMBER (= group @row.visible_to) (= person @viewer))))
; the birthday charities
(read CHARITY true)
; apps open to everyone or to a group the viewer is in
(read APP
  (or (blank visible_to)
      (exists EFFECTIVE_MEMBER (= group @row.visible_to) (= person @viewer))))
; front-page widgets open to everyone or to a group the viewer is in
(read WIDGET
  (or (blank visible_to)
      (exists EFFECTIVE_MEMBER (= group @row.visible_to) (= person @viewer))))
; the coordinates of a family address the viewer may see; a withheld address is blank, so it matches nothing
(read GEOCODE
  (exists GROUP @f (= kind "family") (= address @row.address) (visible @f)))
; the invite list services
(read INVITE_SERVICE true)
; the invite list templates
(read INVITE_TEMPLATE true)
; shared greetings and the viewer's own
(read GREETING (or (blank owner) (= owner @viewer)))

;; Who? admins

; every person, hidden or deactivated too
(read PERSON (admin_of "who"))
; override anyone's long name
(set PERSON.name_long_override (admin_of "who"))
; override anyone's short name
(set PERSON.name_short_override (admin_of "who"))
; override anyone's sort name
(set PERSON.name_sort_override (admin_of "who"))
; every directory group, hidden or pending
(read GROUP (and (admin_of "who") (in kind "family" "tag" "classroom" "grade" "band" "crew" "department" "role")))
; every directory group's memberships
(read MEMBER (and (admin_of "who") (in group.kind "family" "tag" "classroom" "grade" "band" "crew" "department" "role")))
; every directory group's effective members
(read EFFECTIVE_MEMBER (and (admin_of "who") (in group.kind "family" "tag" "classroom" "grade" "band" "crew" "department" "role")))
; the rules that pick each tag's members
(read RULE (and (admin_of "who") (in group.kind "tag")))

;; When admins

; every calendar group: events, series, days, their parts and the day templates
(read GROUP (and (admin_of "when") (in kind "event" "series" "day" "day_part" "day_template")))
; open, hide, cancel or close an event or series
(set GROUP.status (and (admin_of "when") (in @old.kind "event" "series")))
; where every calendar group came from
(read GROUP_SOURCE (and (admin_of "when") (in group.kind "event" "series" "day" "day_part")))
; every event's and series' invitees and hosts
(read MEMBER (and (admin_of "when") (in group.kind "event" "series")))
; make an invitee a host, or a host an invitee
(set MEMBER.role (and (admin_of "when") (in @old.group.kind "event" "series")))
; every event's and series' effective invitees
(read EFFECTIVE_MEMBER (and (admin_of "when") (in group.kind "event" "series")))
; the rules that say who events, series and day parts are for
(read RULE (and (admin_of "when") (in group.kind "event" "series" "day_part")))
; all mail about events and series
(read MESSAGE (and (admin_of "when") (in group.kind "event" "series")))
; every delivery of mail about events and series
(read RECIPIENT (and (admin_of "when") (in message.group.kind "event" "series")))

;; Team admins

; every activity
(read GROUP (and (admin_of "team") (in kind "activity")))
; open, hide, cancel or close an activity
(set GROUP.status (and (admin_of "team") (in @old.kind "activity")))
; every activity's volunteers and leads
(read MEMBER (and (admin_of "team") (in group.kind "activity")))
; make a volunteer a lead, or a lead a volunteer
(set MEMBER.role (and (admin_of "team") (in @old.group.kind "activity")))
; every activity's effective volunteers
(read EFFECTIVE_MEMBER (and (admin_of "team") (in group.kind "activity")))
; the rules that say who activities are for
(read RULE (and (admin_of "team") (in group.kind "activity")))
; all mail about activities
(read MESSAGE (and (admin_of "team") (in group.kind "activity")))
; every delivery of mail about activities
(read RECIPIENT (and (admin_of "team") (in message.group.kind "activity")))

;; Celebrate admins

; every party and celebration
(read GROUP (and (admin_of "celebrate") (in kind "party" "celebration")))
; open, hide, cancel or close a party or celebration
(set GROUP.status (and (admin_of "celebrate") (in @old.kind "party" "celebration")))
; every party's and celebration's ticket holders and hosts
(read MEMBER (and (admin_of "celebrate") (in group.kind "party" "celebration")))
; what every party ticket cost
(read MEMBER.price (and (admin_of "celebrate") (in group.kind "party")))
; every party ticket's payment reference
(read MEMBER.purchase_id (and (admin_of "celebrate") (in group.kind "party")))
; make a ticket holder a host, or a host a ticket holder
(set MEMBER.role (and (admin_of "celebrate") (in @old.group.kind "party")))
; every party's and celebration's effective members
(read EFFECTIVE_MEMBER (and (admin_of "celebrate") (in group.kind "party" "celebration")))
; the rules that say who parties and celebrations are for
(read RULE (and (admin_of "celebrate") (in group.kind "party" "celebration")))
; all mail about parties and celebrations
(read MESSAGE (and (admin_of "celebrate") (in group.kind "party" "celebration")))
; every delivery of mail about parties and celebrations
(read RECIPIENT (and (admin_of "celebrate") (in message.group.kind "party" "celebration")))

;; Loop admins

; every list
(read GROUP (and (admin_of "loop") (in kind "list")))
; open, hide or close a list
(set GROUP.status (and (admin_of "loop") (in @old.kind "list")))
; every list's members and leads
(read MEMBER (and (admin_of "loop") (in group.kind "list")))
; make a member a lead, or a lead a member
(set MEMBER.role (and (admin_of "loop") (in @old.group.kind "list")))
; every list's effective members
(read EFFECTIVE_MEMBER (and (admin_of "loop") (in group.kind "list")))
; the rules that pick each list's members
(read RULE (and (admin_of "loop") (in group.kind "list")))
; all mail to lists
(read MESSAGE (and (admin_of "loop") (in group.kind "list")))
; every delivery of mail to lists
(read RECIPIENT (and (admin_of "loop") (in message.group.kind "list")))
; every post sent to a list
(read DOCUMENT (and (admin_of "loop") (exists DOCUMENT_GROUP (= document @row) (= relation "sent_to") (= group.kind "list"))))
; which lists each post was sent to
(read DOCUMENT_GROUP (and (admin_of "loop") (= group.kind "list")))

;; Home admins

; every audience, admins group and front-page section
(read GROUP (and (admin_of "home") (in kind "audience" "admins" "section")))
; the members of audiences, admins groups and sections
(read MEMBER (and (admin_of "home") (in group.kind "audience" "admins" "section")))
; the effective members of audiences, admins groups and sections
(read EFFECTIVE_MEMBER (and (admin_of "home") (in group.kind "audience" "admins" "section")))
; the rules that pick audiences', admins groups' and sections' members
(read RULE (and (admin_of "home") (in group.kind "audience" "admins" "section")))
; every link category, whoever it is open to
(read CATEGORY (and (admin_of "home") (= scope "link")))
; every app, whoever it is open to
(read APP (admin_of "home"))
; every front-page widget, whoever it is open to
(read WIDGET (admin_of "home"))

;; Super admins

; old IDs and the rows they now name
(read ALIAS (super_admin))
; old paths and where they now go
(read REDIRECT (super_admin))

;; System: import

; every person, withheld too, to match the Veracross export against
(read PERSON (system "import"))
; add a Veracross person new in the export
(insert PERSON (and (system "import") (= @new.source "veracross")))
; a person's name as Veracross has it
(set PERSON.vc_name (system "import"))
; a person's legal name as Veracross has it
(set PERSON.vc_legal_name (system "import"))
; a student's grade as Veracross has it
(set PERSON.vc_grade (system "import"))
; a student's classroom as Veracross has it
(set PERSON.vc_classroom (system "import"))
; a person's crew as Veracross has it
(set PERSON.vc_crew (system "import"))
; a staff member's job title as Veracross has it
(set PERSON.vc_job_title (system "import"))
; a person's phone as Veracross has it
(set PERSON.vc_phone (system "import"))
; a staff member's bio as Veracross has it
(set PERSON.vc_bio (system "import"))
; Veracross's address privacy, shown on the profile to explain it
(set PERSON.vc_address_visibility (system "import"))
; Veracross's phone privacy, shown on the profile to explain it
(set PERSON.vc_phone_visibility (system "import"))
; a person's long name derived from Veracross's
(set PERSON.name_long_import (system "import"))
; a person's short name derived from Veracross's
(set PERSON.name_short_import (system "import"))
; a person's sort name derived from Veracross's
(set PERSON.name_sort_import (system "import"))
; deactivate a Veracross person gone from the export, or bring one back
(set PERSON.deactivated (and (system "import") (= @old.source "veracross")))
; whether the opt-in form lists a person
(set PERSON.consent (system "import"))
; whether the opt-in form shares a person's address
(set PERSON.address_consent (system "import"))
; whether the opt-in form shares a person's phone
(set PERSON.phone_consent (system "import"))
; every email, to match the export's people by
(read PERSON_EMAIL (system "import"))
; add an email new in the export
(insert PERSON_EMAIL (and (system "import") (= @new.source "veracross")))
; change an imported email the export changed
(set PERSON_EMAIL.address (and (system "import") (= @old.source "veracross")))
; remove an imported email gone from the export
(delete PERSON_EMAIL (and (system "import") (= @old.source "veracross")))
; every portrait, to skip those already imported
(read PERSON_PHOTO (system "import"))
; add a portrait from the website
(insert PERSON_PHOTO (system "import"))
; every directory group the import keeps, withheld families too
(read GROUP (and (system "import") (in kind "family" "role" "classroom" "crew" "grade" "band")))
; add a family, role, classroom, crew, grade or band new in the export
(insert GROUP (and (system "import") (in @new.kind "family" "role" "classroom" "crew" "grade" "band")))
; a family's title built from its members' names
(set GROUP.vc_title (and (system "import") (= @old.kind "family")))
; a family's address as Veracross has it
(set GROUP.vc_address (and (system "import") (= @old.kind "family")))
; a family's phone as Veracross has it
(set GROUP.vc_phone (and (system "import") (= @old.kind "family")))
; whether the form lists all of a family's adults
(set GROUP.consent (and (system "import") (= @old.kind "family")))
; whether the form shares all of a family's adults' addresses
(set GROUP.address_consent (and (system "import") (= @old.kind "family")))
; whether the form shares all of a family's adults' phones
(set GROUP.phone_consent (and (system "import") (= @old.kind "family")))
; every family and role membership, to compare with the export
(read MEMBER (and (system "import") (in group.kind "family" "role")))
; add a family or role membership new in the export
(insert MEMBER (and (system "import") (in @new.group.kind "family" "role")))
; the status of a family or role membership
(set MEMBER.status (and (system "import") (in @old.group.kind "family" "role")))
; remove a family or role membership gone from the export
(delete MEMBER (and (system "import") (in @old.group.kind "family" "role")))
`

type policySet struct {
	read   map[string][]cond
	insert map[string][]cond
	set    map[string][]cond
	delete map[string][]cond
}

var policies *policySet

func init() {
	p, err := compilePolicies(policySource)
	if err != nil {
		panic("db: policies: " + err.Error())
	}
	policies = p
}

func compilePolicies(src string) (*policySet, error) {
	forms, err := readForms(src)
	if err != nil {
		return nil, err
	}
	cx := &compiler{policy: true, defines: map[string]*define{}}
	out := &policySet{read: map[string][]cond{}, insert: map[string][]cond{}, set: map[string][]cond{}, delete: map[string][]cond{}}
	for _, form := range forms {
		head := form.head()
		if head == "define" {
			if err := cx.define(form); err != nil {
				return nil, err
			}
			continue
		}
		if len(form.list) != 3 || form.list[1].isList || form.list[1].kind != atomName {
			return nil, form.errorf("a policy is (%s TABLE[.column] condition)", head)
		}
		tableName, column, hasColumn := strings.Cut(form.list[1].text, ".")
		t, err := tableNamed(&sexp{text: tableName, pos: form.list[1].pos})
		if err != nil {
			return nil, err
		}
		if hasColumn {
			if _, err := columnNamed(t, column, form.list[1]); err != nil {
				return nil, err
			}
		}
		var sc *scope
		var into map[string][]cond
		switch {
		case head == "read":
			sc, into = &scope{table: t, name: "row"}, out.read
		case head == "set" && hasColumn:
			sc, into = &scope{table: t, name: "new", outer: &scope{table: t, name: "old"}}, out.set
		case head == "insert" && !hasColumn:
			sc, into = &scope{table: t, name: "new"}, out.insert
		case head == "delete" && !hasColumn:
			sc, into = &scope{table: t, name: "old"}, out.delete
		default:
			return nil, form.errorf("policies are define, read, set TABLE.column, insert TABLE and delete TABLE")
		}
		c, err := cx.cond(form.list[2], sc)
		if err != nil {
			return nil, err
		}
		into[form.list[1].text] = append(into[form.list[1].text], c)
	}
	return out, nil
}

func (cx *compiler) define(form *sexp) error {
	if len(form.list) != 3 || !form.list[1].isList || len(form.list[1].list) == 0 || form.list[1].list[0].isList {
		return form.errorf("a definition is (define (name params…) body)")
	}
	name := form.list[1].list[0].text
	if _, dup := cx.defines[name]; dup {
		return form.errorf("%s is defined twice", name)
	}
	d := &define{body: form.list[2]}
	for _, p := range form.list[1].list[1:] {
		switch {
		case p.isList:
			return p.errorf("a parameter is @name or name")
		case p.kind == atomAt:
			d.params = append(d.params, "@"+p.text)
		case p.kind == atomName:
			d.params = append(d.params, p.text)
		default:
			return p.errorf("a parameter is @name or name")
		}
	}
	cx.defines[name] = d
	return nil
}

func holds(conds []cond, f *frame) bool {
	for _, c := range conds {
		if c.eval(f) {
			return true
		}
	}
	return false
}

func (r *run) readable(t *Table, row store.Row) bool {
	key := t.Name + "\x00" + row["id"]
	if seen, ok := r.rows[key]; ok {
		return seen
	}
	ok := holds(policies.read[t.Name], &frame{table: t, row: row, name: "row", run: r})
	r.rows[key] = ok
	return ok
}

func (r *run) columnReadable(t *Table, row store.Row, column string) bool {
	conds, ok := policies.read[t.Name+"."+column]
	if !ok {
		return true
	}
	key := t.Name + "." + column + "\x00" + row["id"]
	if seen, ok := r.columns[key]; ok {
		return seen
	}
	held := holds(conds, &frame{table: t, row: row, name: "row", run: r})
	r.columns[key] = held
	return held
}

func (r *run) idReadable(id string) bool {
	table, ok := TableOf(id)
	if !ok {
		return false
	}
	if t, _ := Lookup(table); t.Generated {
		return false
	}
	rows := r.table(table)
	row, ok := rows.Get(id)
	if !ok {
		return false
	}
	return r.readable(rows.Table(), row)
}

func (r *run) cell(guarded bool, t *Table, row store.Row, c Column) value {
	raw := row[c.Name]
	if guarded {
		raw = r.guardedCell(t, row, c)
	}
	v := cellValue(c, raw)
	if c.Kind == ID && !v.blank {
		v.s = raw
	}
	return v
}

func (r *run) guardedCell(t *Table, row store.Row, c Column) string {
	raw := row[c.Name]
	if raw == "" || !r.columnReadable(t, row, c.Name) {
		return ""
	}
	switch c.Kind {
	case Ref:
		if !r.idReadable(raw) {
			return ""
		}
	case Refs:
		kept := []string{}
		for _, id := range cells.SplitList(raw) {
			if r.idReadable(id) {
				kept = append(kept, id)
			}
		}
		return cells.JoinList(kept)
	}
	return raw
}

func (r *run) redact(t *Table, row store.Row) store.Row {
	out := store.Row{}
	for _, c := range t.Columns {
		if v := r.guardedCell(t, row, c); v != "" {
			out[c.Name] = v
		}
	}
	return out
}

type Change struct {
	Table string
	Old   store.Row
	New   store.Row
}

func (m *Model) Authorize(env Env, c Change) error {
	t, ok := Lookup(c.Table)
	if !ok || t.Generated {
		return access.Forbidden("no table %s to change", c.Table)
	}
	r := m.newRun(env)
	old := &frame{table: t, row: c.Old, name: "old", run: r}
	switch {
	case c.Old == nil:
		if !holds(policies.insert[t.Name], &frame{table: t, row: c.New, name: "new", run: r}) {
			return access.Forbidden("you may not add to %s", t.Name)
		}
	case c.New == nil:
		if !holds(policies.delete[t.Name], old) {
			return access.Forbidden("you may not remove from %s", t.Name)
		}
	default:
		changed := &frame{table: t, row: c.New, name: "new", outer: old, run: r}
		for _, col := range t.Columns {
			if col.Generated || strings.TrimSpace(c.Old[col.Name]) == strings.TrimSpace(c.New[col.Name]) {
				continue
			}
			if !holds(policies.set[t.Name+"."+col.Name], changed) {
				return access.Forbidden("you may not change %s.%s", t.Name, col.Name)
			}
		}
	}
	return nil
}
