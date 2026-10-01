package db

type Kind int

const (
	Text Kind = iota
	Enum
	ID
	Ref
	Refs
	Bool
	Int
	Money
	Float
	Date
	Moment
	Blob
	Order
	Email
	URL
)

type Column struct {
	Name      string
	Kind      Kind
	Prefix    string
	Target    string
	Values    []string
	Required  bool
	Generated bool
}

type Table struct {
	Name       string
	Sheet      string
	Unique     []string
	Columns    []Column
	AppendOnly bool
	Generated  bool
	Generate   func(row map[string]string)
}

const (
	PeopleSheet    = "datapeople"
	GroupsSheet    = "datagroups"
	DocumentsSheet = "datadocuments"
	MailSheet      = "datamail"
	ConfigSheet    = "dataconfig"
)

var Sheets = []string{PeopleSheet, GroupsSheet, DocumentsSheet, MailSheet, ConfigSheet}

var (
	apps     = []string{"who", "when", "team", "celebrate", "birthday", "loop", "ask", "home"}
	grades   = []string{"K", "1", "2", "3", "4", "5", "6", "7", "8"}
	sources  = []string{"veracross", "manual", "guest"}
	audience = []string{"everyone", "members", "leads"}
)

func ident(prefix string) Column {
	return Column{Name: "id", Kind: ID, Prefix: prefix, Required: true}
}

func token(name, prefix string) Column {
	return Column{Name: name, Kind: ID, Prefix: prefix, Required: true}
}

func ref(name, target string) Column {
	return Column{Name: name, Kind: Ref, Target: target}
}

func refs(name, target string) Column {
	return Column{Name: name, Kind: Refs, Target: target}
}

func enum(name string, values ...string) Column {
	return Column{Name: name, Kind: Enum, Values: values}
}

func col(name string, kind Kind) Column {
	return Column{Name: name, Kind: kind}
}

func generated(name string) Column {
	return Column{Name: name, Kind: Text, Generated: true}
}

func (c Column) required() Column {
	c.Required = true
	return c
}

var Tables = []Table{
	{
		Name:     "PERSON",
		Sheet:    PeopleSheet,
		Generate: personNames,
		Columns: []Column{
			ident(PersonPrefix),
			enum("source", sources...).required(),
			col("vc_name", Text),
			col("vc_legal_name", Text),
			enum("vc_grade", grades...),
			ref("vc_classroom", "GROUP"),
			ref("vc_crew", "GROUP"),
			col("vc_job_title", Text),
			col("vc_phone", Text),
			col("vc_bio", Text),
			enum("vc_address_visibility", "full", "partial", "hidden"),
			enum("vc_phone_visibility", "visible", "mixed", "hidden"),
			col("name_long_import", Text),
			col("name_short_import", Text),
			col("name_sort_import", Text),
			col("name_long_override", Text),
			col("name_short_override", Text),
			col("name_sort_override", Text),
			generated("name_long"),
			generated("name_short"),
			generated("name_sort"),
			generated("name_show"),
			enum("grade", grades...),
			ref("classroom", "GROUP"),
			ref("crew", "GROUP"),
			col("job_title", Text),
			ref("department", "GROUP"),
			col("phone", Text),
			col("facts", Text),
			col("pronouns", Text),
			col("pronunciation", Blob),
			col("facts_updated", Date),
			col("photo_updated", Date),
			col("birthday", Date),
			enum("consent", "listed", "withheld"),
			col("share_address", Bool),
			col("share_phone", Bool),
			col("hidden", Bool),
			col("deactivated", Moment),
			col("signed_out", Moment),
			ref("added_by", "PERSON"),
		},
	},
	{
		Name:   "PERSON_EMAIL",
		Sheet:  PeopleSheet,
		Unique: []string{"address"},
		Columns: []Column{
			ident(PersonEmailPrefix),
			col("address", Email).required(),
			ref("person", "PERSON").required(),
			col("primary", Bool),
			enum("source", sources...).required(),
		},
	},
	{
		Name:   "PERSON_PHOTO",
		Sheet:  PeopleSheet,
		Unique: []string{"person", "photo"},
		Columns: []Column{
			ident(PersonPhotoPrefix),
			ref("person", "PERSON").required(),
			col("photo", Blob).required(),
			col("thumbnail", Blob).required(),
			col("crop", Blob),
			col("order", Order),
		},
	},
	{
		Name:   "PERSON_SETTING",
		Sheet:  PeopleSheet,
		Unique: []string{"person", "app", "key"},
		Columns: []Column{
			ident(PersonSettingPrefix),
			ref("person", "PERSON").required(),
			enum("app", apps...).required(),
			col("key", Text).required(),
			col("value", Text),
		},
	},
	{
		Name:   "BIRTHDAY_YEAR",
		Sheet:  PeopleSheet,
		Unique: []string{"person", "year"},
		Columns: []Column{
			ident(BirthdayYearPrefix),
			ref("person", "PERSON").required(),
			col("year", Int).required(),
			ref("assigned_to", "PERSON"),
			col("contacted_on", Date),
			ref("contacted_by", "PERSON"),
			ref("charity", "CHARITY"),
			enum("participation", "full", "skip", "no_newsletter"),
			col("used_on", Date),
			col("note", Text),
		},
	},
	{
		Name:   "SAVED_VIEW",
		Sheet:  PeopleSheet,
		Unique: []string{"token"},
		Columns: []Column{
			ident(SavedViewPrefix),
			token("token", FeedTokenPrefix),
			ref("person", "PERSON").required(),
			col("name", Text),
			refs("groups", "GROUP"),
			refs("categories", "CATEGORY"),
			col("emoji", Text),
			col("order", Order),
			col("is_default", Bool),
		},
	},
	{
		Name:  "GROUP",
		Sheet: GroupsSheet,
		Columns: []Column{
			ident(GroupPrefix),
			ref("parent", "GROUP"),
			enum("kind", "family", "classroom", "grade", "band", "crew", "department", "role", "event", "series", "activity", "party", "celebration", "day", "day_part", "day_template", "list", "tag", "audience", "admins", "section").required(),
			col("slug", Text),
			col("vc_title", Text),
			col("vc_address", Text),
			col("vc_phone", Text),
			col("title", Text),
			col("subtitle", Text),
			col("description", Text),
			col("image", Blob),
			col("image_crop", Blob),
			col("color", Text),
			col("flyer", Blob),
			col("pronunciation", Blob),
			col("address", Text),
			col("phone", Text),
			enum("status", "pending", "open", "hidden", "done", "cancelled", "closed"),
			enum("visibility", "everyone", "unlisted", "members", "leads", "group"),
			ref("visible_to", "GROUP"),
			enum("members_visible", audience...),
			enum("posting", audience...),
			enum("replying", audience...),
			enum("join", "invite", "direct", "approval", "closed"),
			enum("adding", "anyone", "approval", "leads"),
			col("capacity", Int),
			col("minimum", Int),
			col("price", Money),
			col("unit", Text),
			col("waitlist", Bool),
			ref("eligible", "GROUP"),
			col("parent_required", Bool),
			col("lead_needed", Bool),
			col("priority", Bool),
			col("start", Moment),
			col("end", Moment),
			col("all_day", Bool),
			col("location", Text),
			col("order", Order),
			ref("added_by", "PERSON"),
			col("added", Date),
		},
	},
	{
		Name:  "GROUP_SOURCE",
		Sheet: GroupsSheet,
		Columns: []Column{
			ident(GroupSourcePrefix),
			ref("group", "GROUP").required(),
			col("calendar_event", Text),
			ref("document", "DOCUMENT"),
			col("title", Text),
			col("start", Moment),
			col("end", Moment),
			col("all_day", Bool),
			col("location", Text),
			col("description", Text),
			refs("categories", "CATEGORY"),
			refs("audience", "GROUP"),
			enum("marker", "first_day", "last_day"),
			col("hash", Text),
		},
	},
	{
		Name:   "MEMBER",
		Sheet:  GroupsSheet,
		Unique: []string{"group", "person", "role"},
		Columns: []Column{
			ident(MemberPrefix),
			ref("group", "GROUP").required(),
			ref("person", "PERSON").required(),
			enum("role", "lead", "member", "waitlist").required(),
			enum("status", "invited", "pending", "yes", "maybe", "no", "excluded", "cancelled"),
			col("quantity", Int),
			col("price", Money),
			col("purchase_id", Text),
			ref("guest_of", "PERSON"),
			col("note", Text),
			col("answered", Moment),
			ref("answered_by", "PERSON"),
			enum("via", "page", "mail"),
			col("archived", Bool),
			col("opened", Moment),
			ref("added_by", "PERSON"),
			col("added", Moment),
		},
	},
	{
		Name:  "RULE",
		Sheet: GroupsSheet,
		Columns: []Column{
			ident(RulePrefix),
			ref("group", "GROUP").required(),
			col("order", Order).required(),
			enum("kind", "include", "exclude").required(),
			ref("target", "GROUP"),
			ref("person", "PERSON"),
			col("search", Text),
			col("property", Text),
			col("value", Text),
			col("descend", Bool),
			enum("expand", "self", "parents", "children", "household"),
			ref("within", "GROUP"),
		},
	},
	{
		Name:   "GROUP_CATEGORY",
		Sheet:  GroupsSheet,
		Unique: []string{"group", "category"},
		Columns: []Column{
			ident(GroupCategoryPrefix),
			ref("group", "GROUP").required(),
			ref("category", "CATEGORY").required(),
		},
	},
	{
		Name:  "DOCUMENT",
		Sheet: DocumentsSheet,
		Columns: []Column{
			ident(DocumentPrefix),
			enum("kind", "newsletter", "list", "page", "portal", "post", "link", "calendar").required(),
			col("title", Text),
			col("date", Date),
			ref("author", "PERSON"),
			col("url", URL),
			ref("message", "MESSAGE"),
			col("object", Blob),
			ref("category", "CATEGORY"),
			col("key_points", Text),
			col("indexed", Bool),
			col("order", Order),
		},
	},
	{
		Name:   "DOCUMENT_GROUP",
		Sheet:  DocumentsSheet,
		Unique: []string{"document", "group", "relation"},
		Columns: []Column{
			ident(DocumentGroupPrefix),
			ref("document", "DOCUMENT").required(),
			ref("group", "GROUP").required(),
			enum("relation", "sent_to", "for", "attached").required(),
		},
	},
	{
		Name:  "REPORT",
		Sheet: DocumentsSheet,
		Columns: []Column{
			ident(ReportPrefix),
			ref("reporter", "PERSON").required(),
			col("received", Date),
			enum("app", apps...),
			enum("kind", "bug", "idea"),
			enum("status", "new", "filed", "dismissed").required(),
			col("summary", Text),
			col("details", Text),
			col("page_url", Text),
			col("browser", Text),
			col("errors", Text),
			col("screenshot", Blob),
			col("issue", Text),
			ref("handled_by", "PERSON"),
		},
	},
	{
		Name:  "MESSAGE",
		Sheet: MailSheet,
		Columns: []Column{
			ident(MessagePrefix),
			enum("direction", "in", "out").required(),
			enum("kind", "post", "invitation", "skip", "update", "reminder", "calendar", "notice", "reply", "forward").required(),
			ref("group", "GROUP"),
			ref("about", ""),
			ref("from_person", "PERSON"),
			col("from_address", Email),
			col("subject", Text),
			col("object", Blob),
			col("header_id", Text),
			ref("parent", "MESSAGE"),
			col("created", Moment).required(),
		},
	},
	{
		Name:   "RECIPIENT",
		Sheet:  MailSheet,
		Unique: []string{"token"},
		Columns: []Column{
			ident(RecipientPrefix),
			ref("message", "MESSAGE").required(),
			ref("person", "PERSON").required(),
			token("token", MailTokenPrefix),
			col("provider_id", Text),
			col("created", Moment).required(),
			col("sent", Moment),
			col("delivered", Moment),
			col("failed", Moment),
			col("detail", Text),
		},
	},
	{
		Name:      "EFFECTIVE_MEMBER",
		Generated: true,
		Columns: []Column{
			ident(EffectiveMemberPrefix),
			ref("group", "GROUP"),
			ref("person", "PERSON"),
			enum("status", "invited", "pending", "yes", "maybe", "no"),
			col("reasons", Text),
		},
	},
	{
		Name:      "INBOX",
		Generated: true,
		Columns: []Column{
			col("id", Text),
			ref("person", "PERSON"),
			ref("document", "DOCUMENT"),
		},
	},
	{
		Name:   "SETTING",
		Sheet:  ConfigSheet,
		Unique: []string{"app", "key"},
		Columns: []Column{
			ident(SettingPrefix),
			enum("app", append([]string{"platform"}, apps...)...).required(),
			col("key", Text).required(),
			col("value", Text),
		},
	},
	{
		Name:  "CATEGORY",
		Sheet: ConfigSheet,
		Columns: []Column{
			ident(CategoryPrefix),
			enum("scope", "event", "activity", "party", "link", "day_type", "day_part").required(),
			col("title", Text).required(),
			col("description", Text),
			col("image", Blob),
			col("color", Text),
			enum("style", "cards", "tiles", "events", "apps"),
			col("max", Int),
			col("default", Bool),
			col("order", Order),
			ref("visible_to", "GROUP"),
		},
	},
	{
		Name:  "CHARITY",
		Sheet: ConfigSheet,
		Columns: []Column{
			ident(CharityPrefix),
			col("name", Text).required(),
			col("link", URL),
			col("about", Text),
			col("allowed", Bool),
		},
	},
	{
		Name:   "APP",
		Sheet:  ConfigSheet,
		Unique: []string{"key"},
		Columns: []Column{
			ident(AppPrefix),
			enum("key", apps...).required(),
			col("name", Text),
			col("tagline", Text),
			ref("visible_to", "GROUP"),
			ref("admins", "GROUP"),
			col("order", Order),
		},
	},
	{
		Name:   "WIDGET",
		Sheet:  ConfigSheet,
		Unique: []string{"key"},
		Columns: []Column{
			ident(WidgetPrefix),
			enum("key", "when", "team", "celebrate", "school").required(),
			col("order", Order),
			ref("visible_to", "GROUP"),
		},
	},
	{
		Name:       "GEOCODE",
		Sheet:      ConfigSheet,
		Unique:     []string{"address"},
		AppendOnly: true,
		Columns: []Column{
			ident(GeocodePrefix),
			col("address", Text).required(),
			col("lat", Float),
			col("lng", Float),
		},
	},
	{
		Name:   "ALIAS",
		Sheet:  ConfigSheet,
		Unique: []string{"alias"},
		Columns: []Column{
			ident(AliasPrefix),
			col("alias", Text).required(),
			ref("target", "").required(),
		},
	},
	{
		Name:   "REDIRECT",
		Sheet:  ConfigSheet,
		Unique: []string{"app", "old"},
		Columns: []Column{
			ident(RedirectPrefix),
			enum("app", apps...).required(),
			col("old", Text).required(),
			col("new", Text).required(),
			col("added", Date),
		},
	},
	{
		Name:   "INVITE_SERVICE",
		Sheet:  ConfigSheet,
		Unique: []string{"service"},
		Columns: []Column{
			ident(InviteServicePrefix),
			col("service", Text).required(),
			col("name", Text),
			col("header_row", Bool),
			col("supports_groups", Bool),
			col("description", Text),
		},
	},
	{
		Name:  "INVITE_TEMPLATE",
		Sheet: ConfigSheet,
		Columns: []Column{
			ident(InviteTemplatePrefix),
			ref("service", "INVITE_SERVICE").required(),
			col("order", Order).required(),
			col("column", Text),
			col("template", Text),
		},
	},
	{
		Name:  "GREETING",
		Sheet: ConfigSheet,
		Columns: []Column{
			ident(GreetingPrefix),
			ref("owner", "PERSON"),
			col("name", Text).required(),
			col("format", Text),
			col("grouped", Text),
			col("individual", Text),
		},
	},
}

var byName = func() map[string]*Table {
	out := map[string]*Table{}
	for i := range Tables {
		out[Tables[i].Name] = &Tables[i]
	}
	return out
}()

var prefixTable = func() map[string]string {
	out := map[string]string{}
	for _, t := range Tables {
		if c, ok := t.Column("id"); ok && c.Kind == ID {
			out[c.Prefix] = t.Name
		}
	}
	return out
}()

func Lookup(name string) (*Table, bool) {
	t, ok := byName[name]
	return t, ok
}

func TableOf(s string) (string, bool) {
	prefix, ok := ParseID(s)
	if !ok {
		return "", false
	}
	table, ok := prefixTable[prefix]
	return table, ok
}

func (t Table) Column(name string) (Column, bool) {
	for _, c := range t.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return Column{}, false
}

func (t Table) Stored() []string {
	out := []string{}
	for _, c := range t.Columns {
		if !c.Generated {
			out = append(out, c.Name)
		}
	}
	return out
}
