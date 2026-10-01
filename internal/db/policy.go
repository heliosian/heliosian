package db

import (
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/store"
)

const policySource = `
(define (leads @g)
  (exists MEMBER (in group (ancestors @g)) (= person @viewer) (= role "lead") (= status "yes")))

(define (household @p)
  (select MEMBER.person
    (in group (select MEMBER.group (= person @p) (= role "lead") (= group.kind "family")))
    (= role "member")))

(define (groups_of @p)
  (select MEMBER.group (= person @p)))

(define (admin_of app)
  (exists EFFECTIVE_MEMBER (in group (select APP.admins (= key app))) (= person @viewer)))

(define (super_admin)
  (exists EFFECTIVE_MEMBER (= group.slug "super-admins") (= person @viewer)))

(define (visible @g)
  (and (!= @g.status "closed")
       (or (leads @g)
           (and (not (in @g.status "pending" "hidden"))
                (or (in @g.visibility "everyone" "unlisted")
                    (and (= @g.visibility "members")
                         (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer)))
                    (and (= @g.visibility "group")
                         (exists EFFECTIVE_MEMBER (= group @g.visible_to) (= person @viewer))))))))

(define (sees_members @g)
  (and (visible @g)
       (or (leads @g)
           (blank @g.members_visible)
           (= @g.members_visible "everyone")
           (and (= @g.members_visible "members")
                (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer))))))

(define (person_visible @p)
  (or (= @p @viewer)
      (and (= @p.source "guest")
           (exists MEMBER (in group (groups_of @viewer)) (= person @p)))
      (and (= @p.consent "listed") (not @p.hidden) (blank @p.deactivated))))

(define (shares_address @f)
  (or (leads @f)
      (not (exists MEMBER (= group @f) (= role "lead") (not person.share_address)))))

(define (shares_phone @f)
  (or (leads @f)
      (not (exists MEMBER (= group @f) (= role "lead") (not person.share_phone)))))

(define (sees_mail @g)
  (or (leads @g)
      (and (!= @g.status "hidden")
           (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer)))))

(define (document_visible @d)
  (or (not (exists DOCUMENT_GROUP (= document @d) (= relation "sent_to") (= group.kind "list")))
      (exists DOCUMENT_GROUP (= document @d) (= relation "sent_to") (= group.kind "list") (sees_mail group))))

(read PERSON (person_visible @row))
(read PERSON_EMAIL (person_visible person))
(read PERSON_PHOTO (person_visible person))
(read PERSON_SETTING (= person @viewer))
(read BIRTHDAY_YEAR (person_visible person))
(read SAVED_VIEW (= person @viewer))
(read REPORT (= reporter @viewer))

(read GROUP (visible @row))
(read GROUP_SOURCE (visible group))
(read GROUP_CATEGORY (visible group))
(read RULE (leads group))
(read MEMBER
  (or (= person @viewer)
      (in person (household @viewer))
      (= guest_of @viewer)
      (leads group)
      (and (= role "lead") (visible group))
      (and (= role "member") (sees_members group))))
(read EFFECTIVE_MEMBER
  (or (= person @viewer)
      (in person (household @viewer))
      (sees_members group)))

(read DOCUMENT (document_visible @row))
(read DOCUMENT_GROUP (and (document_visible document) (visible group)))

(read MESSAGE
  (or (= from_person @viewer)
      (exists RECIPIENT (= message @row) (= person @viewer))
      (leads group)))
(read RECIPIENT
  (or (= person @viewer)
      (= message.from_person @viewer)
      (leads message.group)))

(read SETTING true)
(read CATEGORY
  (or (blank visible_to)
      (exists EFFECTIVE_MEMBER (= group @row.visible_to) (= person @viewer))))
(read CHARITY true)
(read APP
  (or (blank visible_to)
      (exists EFFECTIVE_MEMBER (= group @row.visible_to) (= person @viewer))))
(read WIDGET
  (or (blank visible_to)
      (exists EFFECTIVE_MEMBER (= group @row.visible_to) (= person @viewer))))
(read GEOCODE
  (exists GROUP @f (= kind "family") (or (= address @row.address) (= vc_address @row.address))
          (visible @f) (shares_address @f)))
(read ALIAS (super_admin))
(read REDIRECT (super_admin))
(read INVITE_SERVICE true)
(read INVITE_TEMPLATE true)
(read GREETING (or (blank owner) (= owner @viewer)))

(read PERSON.phone (or (= @row @viewer) share_phone))
(read PERSON.vc_phone (or (= @row @viewer) share_phone))
(read GROUP.address (or (!= kind "family") (shares_address @row)))
(read GROUP.vc_address (or (!= kind "family") (shares_address @row)))
(read GROUP.phone (or (!= kind "family") (shares_phone @row)))
(read GROUP.vc_phone (or (!= kind "family") (shares_phone @row)))
(read MEMBER.price (or (= person @viewer) (= guest_of @viewer) (leads group)))
(read MEMBER.purchase_id (or (= person @viewer) (= guest_of @viewer) (leads group)))
(read RECIPIENT.token (= person @viewer))

(set MEMBER.status
  (or (and (or (= @old.person @viewer)
               (in @old.person (household @viewer))
               (= @old.guest_of @viewer))
           (in @new.status "yes" "maybe" "no" "cancelled"))
      (leads @old.group)))
(set PERSON.name_long_override (or (= @old @viewer) (in @old (household @viewer))))
(set PERSON.name_short_override (or (= @old @viewer) (in @old (household @viewer))))
(set PERSON.name_sort_override (or (= @old @viewer) (in @old (household @viewer))))

(read PERSON (admin_of "who"))
(read PERSON.phone (admin_of "who"))
(read PERSON.vc_phone (admin_of "who"))
(read GROUP (and (admin_of "who") (in kind "family" "tag" "classroom" "grade" "band" "crew" "department" "role")))
(read MEMBER (and (admin_of "who") (in group.kind "family" "tag" "classroom" "grade" "band" "crew" "department" "role")))
(read EFFECTIVE_MEMBER (and (admin_of "who") (in group.kind "family" "tag" "classroom" "grade" "band" "crew" "department" "role")))
(read RULE (and (admin_of "who") (in group.kind "tag")))
(set PERSON.name_long_override (admin_of "who"))
(set PERSON.name_short_override (admin_of "who"))
(set PERSON.name_sort_override (admin_of "who"))

(read GROUP (and (admin_of "when") (in kind "event" "series" "day" "day_part" "day_template")))
(read GROUP_SOURCE (and (admin_of "when") (in group.kind "event" "series" "day" "day_part")))
(read MEMBER (and (admin_of "when") (in group.kind "event" "series")))
(read EFFECTIVE_MEMBER (and (admin_of "when") (in group.kind "event" "series")))
(read RULE (and (admin_of "when") (in group.kind "event" "series" "day_part")))
(read MESSAGE (and (admin_of "when") (in group.kind "event" "series")))
(read RECIPIENT (and (admin_of "when") (in message.group.kind "event" "series")))
(set GROUP.status (and (admin_of "when") (in @old.kind "event" "series")))
(set MEMBER.role (and (admin_of "when") (in @old.group.kind "event" "series")))

(read GROUP (and (admin_of "team") (in kind "activity")))
(read MEMBER (and (admin_of "team") (in group.kind "activity")))
(read EFFECTIVE_MEMBER (and (admin_of "team") (in group.kind "activity")))
(read RULE (and (admin_of "team") (in group.kind "activity")))
(read MESSAGE (and (admin_of "team") (in group.kind "activity")))
(read RECIPIENT (and (admin_of "team") (in message.group.kind "activity")))
(set GROUP.status (and (admin_of "team") (in @old.kind "activity")))
(set MEMBER.role (and (admin_of "team") (in @old.group.kind "activity")))

(read GROUP (and (admin_of "celebrate") (in kind "party" "celebration")))
(read MEMBER (and (admin_of "celebrate") (in group.kind "party" "celebration")))
(read EFFECTIVE_MEMBER (and (admin_of "celebrate") (in group.kind "party" "celebration")))
(read RULE (and (admin_of "celebrate") (in group.kind "party" "celebration")))
(read MESSAGE (and (admin_of "celebrate") (in group.kind "party" "celebration")))
(read RECIPIENT (and (admin_of "celebrate") (in message.group.kind "party" "celebration")))
(read MEMBER.price (and (admin_of "celebrate") (in group.kind "party")))
(read MEMBER.purchase_id (and (admin_of "celebrate") (in group.kind "party")))
(set GROUP.status (and (admin_of "celebrate") (in @old.kind "party" "celebration")))
(set MEMBER.role (and (admin_of "celebrate") (in @old.group.kind "party")))

(read GROUP (and (admin_of "loop") (in kind "list")))
(read MEMBER (and (admin_of "loop") (in group.kind "list")))
(read EFFECTIVE_MEMBER (and (admin_of "loop") (in group.kind "list")))
(read RULE (and (admin_of "loop") (in group.kind "list")))
(read MESSAGE (and (admin_of "loop") (in group.kind "list")))
(read RECIPIENT (and (admin_of "loop") (in message.group.kind "list")))
(read DOCUMENT (and (admin_of "loop") (exists DOCUMENT_GROUP (= document @row) (= relation "sent_to") (= group.kind "list"))))
(read DOCUMENT_GROUP (and (admin_of "loop") (= group.kind "list")))
(set GROUP.status (and (admin_of "loop") (in @old.kind "list")))
(set MEMBER.role (and (admin_of "loop") (in @old.group.kind "list")))

(read GROUP (and (admin_of "home") (in kind "audience" "admins" "section")))
(read MEMBER (and (admin_of "home") (in group.kind "audience" "admins" "section")))
(read EFFECTIVE_MEMBER (and (admin_of "home") (in group.kind "audience" "admins" "section")))
(read RULE (and (admin_of "home") (in group.kind "audience" "admins" "section")))
(read CATEGORY (and (admin_of "home") (= scope "link")))
(read APP (admin_of "home"))
(read WIDGET (admin_of "home"))

(read PERSON (system "import"))
(read PERSON.vc_phone (system "import"))
(read PERSON_EMAIL (system "import"))
(read PERSON_PHOTO (system "import"))
(insert PERSON_PHOTO (system "import"))
(read GROUP (and (system "import") (in kind "family" "role" "classroom" "crew" "grade" "band")))
(read GROUP.vc_address (and (system "import") (= kind "family")))
(read GROUP.vc_phone (and (system "import") (= kind "family")))
(read MEMBER (and (system "import") (in group.kind "family" "role")))
(insert PERSON (and (system "import") (= @new.source "veracross")))
(insert PERSON_EMAIL (and (system "import") (= @new.source "veracross")))
(insert GROUP (and (system "import") (in @new.kind "family" "role" "classroom" "crew" "grade" "band")))
(insert MEMBER (and (system "import") (in @new.group.kind "family" "role")))
(set PERSON.vc_name (system "import"))
(set PERSON.vc_legal_name (system "import"))
(set PERSON.vc_grade (system "import"))
(set PERSON.vc_classroom (system "import"))
(set PERSON.vc_crew (system "import"))
(set PERSON.vc_job_title (system "import"))
(set PERSON.vc_phone (system "import"))
(set PERSON.vc_bio (system "import"))
(set PERSON.vc_address_visibility (system "import"))
(set PERSON.vc_phone_visibility (system "import"))
(set PERSON.name_long_import (system "import"))
(set PERSON.name_short_import (system "import"))
(set PERSON.name_sort_import (system "import"))
(set PERSON.deactivated (and (system "import") (= @old.source "veracross")))
(set PERSON_EMAIL.address (and (system "import") (= @old.source "veracross")))
(set GROUP.vc_title (and (system "import") (= @old.kind "family")))
(set GROUP.vc_address (and (system "import") (= @old.kind "family")))
(set GROUP.vc_phone (and (system "import") (= @old.kind "family")))
(set MEMBER.status (and (system "import") (in @old.group.kind "family" "role")))
(delete PERSON_EMAIL (and (system "import") (= @old.source "veracross")))
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
	rows := r.m.Table(table)
	row, ok := rows.Get(id)
	return ok && r.readable(rows.table, row)
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
