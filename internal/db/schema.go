package db

import (
	"fmt"
	"strings"
)

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

type Value struct {
	Name        string
	Description string
}

type Column struct {
	Name        string
	Kind        Kind
	Prefix      string
	Target      string
	Values      []Value
	Required    bool
	Generated   bool
	Private     bool
	Description string
}

type Table struct {
	Name        string
	Sheet       string
	Unique      []string
	Columns     []Column
	AppendOnly  bool
	Generated   bool
	Generate    func(row map[string]string)
	Description string
}

const (
	PeopleSheet    = "datapeople"
	GroupsSheet    = "datagroups"
	DocumentsSheet = "datadocuments"
	MailSheet      = "datamail"
	ConfigSheet    = "dataconfig"
)

var Sheets = []string{PeopleSheet, GroupsSheet, DocumentsSheet, MailSheet, ConfigSheet}

const (
	EverySheet   = "*"
	ChangesTable = "CHANGES"
)

func (t Table) In(sheet string) bool {
	return t.Sheet == sheet || t.Sheet == EverySheet
}

var (
	apps = []Value{
		v("who", "Helios Who?, the directory"),
		v("when", "Helios When, the calendar"),
		v("team", "Helios Team, volunteering"),
		v("celebrate", "Helios Celebrate, the fundraiser parties"),
		v("birthday", "Staff Birthdays"),
		v("loop", "Helios Loop, the email lists"),
		v("ask", "Helios Ask"),
		v("admin", "the admin pages"),
		v("home", "Heliosian, the front page"),
	}
	grades = []Value{
		v("K", "Kindergarten"),
		v("1", "Grade 1"),
		v("2", "Grade 2"),
		v("3", "Grade 3"),
		v("4", "Grade 4"),
		v("5", "Grade 5"),
		v("6", "Grade 6"),
		v("7", "Grade 7"),
		v("8", "Grade 8"),
	}
	sources = []Value{
		v("veracross", "written by the Veracross import"),
		v("manual", "added by hand or by an app"),
		v("guest", "a guest: someone outside the school's records, or the shown twin of someone who isn't"),
	}
	senders = []Value{
		v("everyone", "anyone, from any address, in Helios or not"),
		v("members", "its effective members and managers"),
		v("managers", "its managers alone"),
	}
	consents = []Value{
		v("listed", "consented: shown"),
		v("withheld", "not consented, or no answer: never shown to anyone but the import"),
	}
	shares = []Value{
		v("shared", "shown to whoever sees the row"),
		v("withheld", "never shown, not even to its owner"),
	}
)

func v(name, description string) Value {
	return Value{Name: name, Description: description}
}

func ident(prefix string) Column {
	return Column{Name: "id", Kind: ID, Prefix: prefix, Required: true, Description: "This row's ID."}
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

func enum(name string, values ...Value) Column {
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

func (c Column) private() Column {
	c.Private = true
	return c
}

func (c Column) about(description string) Column {
	c.Description = description
	return c
}

func (c Column) ValueNames() []string {
	out := []string{}
	for _, value := range c.Values {
		out = append(out, value.Name)
	}
	return out
}

var Tables = []Table{
	{
		Name:        "PERSON",
		Sheet:       PeopleSheet,
		Generate:    personGenerated,
		Description: "Anyone the community touches: a student, parent or staff member, or a guest. Whether someone is a student, parent or staff member is their membership of the role groups. Never deleted: deactivated instead, so what names them keeps its record.",
		Columns: []Column{
			ident(PersonPrefix),
			enum("source", sources...).required().about("Where the person came from."),
			col("vc_name", Text).about("Their name as Veracross has it, preferred first name and legal first name in brackets (Juni (Juniper) Ashdown). Written by the import."),
			col("vc_legal_name", Text).about("Their legal name as Veracross has it. Written by the import."),
			enum("vc_grade", grades...).about("A student's grade as Veracross has it. Written by the import; grade overrides it."),
			ref("vc_classroom", "GROUP").about("A student's classroom as Veracross has it. Written by the import; classroom overrides it."),
			ref("vc_crew", "GROUP").about("A student's crew as Veracross has it. Written by the import; crew overrides it."),
			ref("vc_department", "GROUP").about("A staff member's department as Veracross has it. Written by the import; department overrides it."),
			col("vc_job_title", Text).about("A staff member's job title, Veracross's, else the school website's. Written by the import; job_title overrides it."),
			col("vc_phone", Text).private().about("Their phone number as Veracross has it. Written by the import; phone_override overrides it."),
			col("vc_bio", Text).about("A staff member's bio from the school website's staff page. Written by the import."),
			enum("vc_address_visibility",
				v("full", "Veracross shows the address"),
				v("partial", "Veracross shows part of it"),
				v("hidden", "Veracross hides it")).about("Veracross's own privacy setting for the address, already applied to the export; kept only to explain it on the profile."),
			enum("vc_phone_visibility",
				v("visible", "Veracross shows the numbers"),
				v("mixed", "Veracross shows some of them"),
				v("hidden", "Veracross hides them")).about("Veracross's own privacy setting for phone numbers, already applied to the export; kept only to explain it on the profile."),
			col("name_long_import", Text).about("Their full name as the import reads it from vc_name (Juni Ashdown)."),
			col("name_short_import", Text).about("Their short name as the import reads it from vc_name (Juni)."),
			col("name_sort_import", Text).about("Their sorting name as the import reads it from vc_name (Ashdown, Juni)."),
			col("name_long_override", Text).about("Their full name as set in the app, in place of the import's."),
			col("name_short_override", Text).about("Their short name as set in the app, in place of the import's."),
			col("name_sort_override", Text).about("Their sorting name as set in the app, in place of the import's."),
			generated("name_long").about("Their full name: the override, else the import's."),
			generated("name_short").about("Their short name: the override, else the import's."),
			generated("name_sort").about("Their sorting name: the override, else the import's."),
			generated("name_show").about("The name to show: name_long, else Guest for a guest, else [Missing Name]."),
			enum("grade", grades...).about("A student's grade as set in the app, in place of vc_grade."),
			ref("classroom", "GROUP").about("A student's classroom as set in the app, in place of vc_classroom."),
			ref("crew", "GROUP").about("A student's crew as set in the app, in place of vc_crew."),
			col("job_title", Text).about("A staff member's job title as set in the app, in place of vc_job_title."),
			ref("department", "GROUP").about("A staff member's department as set in the app, in place of vc_department."),
			col("phone_override", Text).private().about("Their phone number as set in the app, in place of vc_phone."),
			generated("phone").about("Their phone number: the override, else Veracross's; blank unless phone_consent is shared."),
			col("facts", Text).about("Their about-me words on the profile."),
			col("pronouns", Text).about("Their pronouns, as they write them."),
			col("pronunciation", Blob).about("A recording of how their name is said."),
			col("facts_updated", Date).about("When their about-me words last changed."),
			col("photo_updated", Date).about("When their picture last changed."),
			col("birthday", Date).about("A staff member's birthday, for Staff Birthdays; the year is not used."),
			enum("consent", consents...).private().about("Whether they consented to be in the directory, from the opt-in form; written by the consent import. Hidden from everyone but the import."),
			enum("address_consent", shares...).about("Whether their address is shown, from the opt-in form; written by the consent import."),
			enum("phone_consent", shares...).about("Whether their phone number is shown, from the opt-in form; written by the consent import."),
			col("hidden", Bool).about("Out of the directory and refused at sign-in without being deactivated. Seen by Who?'s admins alone."),
			col("deactivated", Moment).about("When they stopped being part of the community. A deactivated person can't sign in and is in no group's effective members."),
			col("signed_out", Moment).about("When they last signed out everywhere; sessions from before it are refused."),
		},
	},
	{
		Name:        "PERSON_EMAIL",
		Sheet:       PeopleSheet,
		Unique:      []string{"address", "guest"},
		Generate:    emailGenerated,
		Description: "An email address a person uses. Sign-in, replies and inbound mail find the person through it; outbound mail goes to the primary. One person holds an address among students, adults and staff, and one among guests.",
		Columns: []Column{
			ident(PersonEmailPrefix),
			col("address", Email).required().about("The address, in lower case. Never a .noemail placeholder."),
			ref("person", "PERSON").required().about("Whose address it is."),
			col("primary", Bool).about("The address mail goes to. Exactly one per person with any address."),
			enum("source", sources...).required().about("Where the address came from."),
			{Name: "guest", Kind: Bool, Generated: true, Description: "Whether a guest holds it: source is guest."},
		},
	},
	{
		Name:        "PHOTO",
		Sheet:       PeopleSheet,
		Unique:      []string{"person", "group", "photo"},
		Generate:    photoGenerated,
		Description: "A picture of a person or a group, exactly one: a portrait, a family photo, a classroom or grade tile, a category's or activity's picture. The first in order is the one shown. Added through /api/do/photo; the server makes the re-encode, crop and thumbnail after the commit.",
		Columns: []Column{
			ident(PhotoPrefix),
			ref("person", "PERSON").about("The person it is of."),
			ref("group", "GROUP").about("The group it is of."),
			col("photo", Blob).required().private().about("The original as uploaded, which can carry location and face tags."),
			col("reencode", Blob).about("The original turned upright, its tags dropped, as a JPEG at most 2048 pixels on its long side. Made by the server."),
			col("crop_left", Int).about("The crop box's left edge, in pixels of the re-encode."),
			col("crop_top", Int).about("The crop box's top edge, in pixels of the re-encode."),
			col("crop_width", Int).about("The crop box's width, in pixels of the re-encode."),
			col("crop_height", Int).about("The crop box's height, in pixels of the re-encode."),
			col("crop", Blob).about("The re-encode cut to the box. Made by the server."),
			{Name: "image", Kind: Blob, Generated: true, Description: "The picture to show: the crop, else the re-encode."},
			col("thumbnail", Blob).about("A small copy of the image. Made by the server."),
			col("ready", Bool).about("The made pictures match the current original and box. Set by the server; cleared when either changes."),
			col("order", Order).about("Its place among the person's or group's pictures; the first is shown."),
		},
	},
	{
		Name:        "PERSON_SETTING",
		Sheet:       PeopleSheet,
		Unique:      []string{"person", "app", "key"},
		Description: "A person's preference in one app, by key: a Team admin's notice kinds, a host's answer notices.",
		Columns: []Column{
			ident(PersonSettingPrefix),
			ref("person", "PERSON").required().about("Whose preference it is."),
			enum("app", apps...).required().about("The app it belongs to."),
			col("key", Text).required().about("Which preference, as the app names it."),
			col("value", Text).about("The preference's value, as the app reads it."),
		},
	},
	{
		Name:        "BIRTHDAY_YEAR",
		Sheet:       PeopleSheet,
		Unique:      []string{"person", "year"},
		Description: "One staff member's birthday in one school year for Staff Birthdays: who on the team has it, the outreach, the charity chosen and the newsletter that carried it.",
		Columns: []Column{
			ident(BirthdayYearPrefix),
			ref("person", "PERSON").required().about("The staff member whose birthday it is."),
			col("year", Int).required().about("The school year, by the calendar year it starts in."),
			ref("assigned_to", "PERSON").about("Who on the birthday team is looking after it."),
			col("contacted_on", Date).about("When the staff member was asked for their charity."),
			ref("contacted_by", "PERSON").about("Who asked them."),
			ref("charity", "CHARITY").about("The charity given to in their name."),
			enum("participation",
				v("full", "asked, given for and in the newsletter"),
				v("skip", "left out altogether"),
				v("no_newsletter", "asked and given for, never in the newsletter")).about("How they want to take part."),
			col("used_on", Date).about("When the newsletter carried it."),
			col("note", Text).about("The staff member's own words about their choice."),
		},
	},
	{
		Name:        "COLLECTION",
		Sheet:       PeopleSheet,
		Unique:      []string{"token"},
		Description: "A calendar a person saved in When: the classrooms and categories it filters by. Each is also a personal feed address.",
		Columns: []Column{
			ident(CollectionPrefix),
			token("token", FeedTokenPrefix).about("The secret in its feed address. Minted by the server."),
			ref("person", "PERSON").required().about("Whose calendar it is."),
			col("name", Text).about("What they call it."),
			refs("groups", "GROUP").about("The classrooms and categories it shows; none means every one."),
			col("emoji", Text).about("The mark they gave it."),
			col("order", Order).about("Its place among their saved calendars."),
			col("is_default", Bool).about("The calendar When opens to for them."),
		},
	},
	{
		Name:        "GROUP",
		Sheet:       GroupsSheet,
		Generate:    groupGenerated,
		Description: "Any set of people, anything things are filed under, and anything on the calendar. A group sits under its parent; a thing's category is its parent, and a manager of a group manages every group under it.",
		Columns: []Column{
			ident(GroupPrefix),
			ref("parent", "GROUP").about("The group it sits under: its category, the event or activity it is part of, a recurring event for an instance, its celebration, its band or classroom, its day."),
			enum("kind",
				v("family", "a household: its adults manage it, everyone in it is a member"),
				v("classroom", "a school classroom, kept from people's classrooms"),
				v("grade", "a school grade, kept from people's grades"),
				v("band", "a band of grades and classrooms; it takes in everything under it by its own rule"),
				v("crew", "a crew within a classroom"),
				v("department", "a staff department, Veracross's"),
				v("event", "a When event; on the calendar when it has a start, and a recurring event when it has none and its instances under it"),
				v("day", "a school day on its date, under its day-type category, for the classrooms its rules name or all of them"),
				v("day_part", "a part of a school day: Dropoff, School, Pickup or Aftercare"),
				v("activity", "a Team activity; each school year is a root activity holding that year's"),
				v("party", "a Celebrate party"),
				v("celebration", "a Celebrate celebration, holding its parties"),
				v("category", "a heading or filter things are filed under through their parent"),
				v("group", "a group that is only a set of people: role groups, tags, Loop lists, audiences, room parents"),
				v("admins", "an app's admins, named by APP.admins")).required().about("What the group is, where it shows a different way."),
			col("listed", Bool).about("Shown in its app's lists; a group of kind group is also shown in Who? and the pickers."),
			col("mail", Bool).about("Has an email address and is a list in Loop."),
			col("slug", Text).about("Its short name in addresses: a page's friendly address, a list's email address, a grade's grade-k to grade-8."),
			col("vc_address", Text).private().about("A family's address as Veracross has it. Written by the import; address_override overrides it."),
			col("vc_phone", Text).private().about("A family's phone number as Veracross has it. Written by the import; phone_override overrides it."),
			col("title", Text).about("Its name. A family's is written by a trigger from its members' last names."),
			col("subtitle", Text).about("A line under the title."),
			col("description", Text).about("What it is, in words. A category's is what the calendar classifier reads."),
			col("color", Text).about("A classroom's or grade's color."),
			col("flyer", Blob).about("A poster, shown whole."),
			col("pronunciation", Blob).about("A recording of how a family's name is said."),
			col("address_override", Text).private().about("Its address as set in the app, in place of vc_address."),
			col("phone_override", Text).private().about("Its phone number as set in the app, in place of vc_phone."),
			generated("address").about("Its address: the override, else Veracross's; for a family, blank unless address_consent is shared."),
			generated("phone").about("Its phone number: the override, else Veracross's; for a family, blank unless phone_consent is shared."),
			enum("consent", consents...).private().about("A family's consent: listed only when every manager's is. Written by the consent import."),
			enum("address_consent", shares...).about("Whether a family's address is shown: shared only when every manager's is. Written by the consent import."),
			enum("phone_consent", shares...).about("Whether a family's phone number is shown: shared only when every manager's is. Written by the consent import."),
			enum("status",
				v("pending", "awaiting approval; seen by its managers and its app's admins alone"),
				v("open", "live"),
				v("done", "an activity that happened, still shown"),
				v("cancelled", "called off"),
				v("closed", "gone from every list and page, kept for the messages naming it")).about("Where it stands."),
			ref("visible_to", "GROUP").about("The group whose effective members can see it: everyone for all, the group itself for its members, blank for its managers and its app's admins alone (a hidden group). Its managers and its app's admins always can."),
			ref("members_visible_to", "GROUP").about("The group whose effective members can see who is in it, among those who can see it: everyone, the group itself, or blank for its managers alone."),
			enum("posting", senders...).about("Who may post to a Loop list."),
			enum("replying", senders...).about("Who may reply to a Loop list's posts."),
			enum("join",
				v("direct", "sign up or buy right here"),
				v("below", "sign up for something under it, not here"),
				v("none", "full or closed; nothing more is taken")).about("How people join it. A blank activity's is read from the nearest group above that has one."),
			enum("adding",
				v("anyone", "anyone may add, and it goes live"),
				v("approval", "anyone may add, and it waits as pending for an admin"),
				v("managers", "only its managers add")).about("Who may add things under an activity. A blank one's is read from the nearest group above that has one."),
			col("capacity", Int).about("How many places there are; blank for no limit."),
			col("minimum", Int).about("How many it needs to go ahead."),
			col("price", Money).about("A party's price per place, in dollars."),
			col("unit", Text).about("What a party's price is per, in words (person, adult, couple)."),
			col("waitlist", Bool).about("A full party takes a waitlist."),
			ref("eligible", "GROUP").about("The group whose effective members may join; blank for anyone."),
			col("parent_ticket_required", Bool).about("A student's party place needs a parent with one."),
			col("drop_off_allowed", Bool).about("Students may be dropped off at a party."),
			col("lead_needed", Bool).about("An activity still wants a co-chair."),
			col("priority", Bool).about("An activity the community most needs hands for, marked by an admin."),
			col("start", Moment).about("When it starts; set means it is on the calendar. A date alone with all_day."),
			col("end", Moment).about("When it ends; for an all-day one, the last day it runs."),
			col("all_day", Bool).about("It runs whole days, without times."),
			col("timing", Text).about("When it happens, in words, where start can't say it (All Year, September/October)."),
			col("location", Text).about("Where it happens."),
			col("url", URL).about("A page elsewhere that it stands for, so a link is a group like any other."),
			col("default", Bool).about("A When category the calendar's filter starts with on."),
			col("order", Order).about("Its place among the groups beside it."),
			ref("added_by", "PERSON").about("Who added it."),
			col("added", Date).about("When it was added."),
		},
	},
	{
		Name:        "GROUP_SOURCE",
		Sheet:       GroupsSheet,
		Description: "A place a calendar group comes from and what it says there: a Google Calendar event, or an entry of the school's year-calendar PDF. A group lives until its last source goes. Written only by the calendar import.",
		Columns: []Column{
			ident(GroupSourcePrefix),
			ref("group", "GROUP").required().about("The group it brings."),
			col("calendar_event", Text).about("The Google Calendar event's key: its UID, and for an instance of a repeating event its original start."),
			ref("document", "DOCUMENT").about("The year-calendar PDF it was read from."),
			col("title", Text).about("The title as the source has it."),
			col("start", Moment).about("The start as the source has it."),
			col("end", Moment).about("The end as the source has it."),
			col("all_day", Bool).about("Whether the source has it as whole days."),
			col("location", Text).about("The location as the source has it."),
			col("description", Text).about("The description as the source has it, as plain text."),
			enum("marker",
				v("first_day", "the school year's first day"),
				v("last_day", "the school year's last day")).about("A PDF entry that bounds the school year."),
			col("hash", Text).about("A hash of the input it was last classified or read with, so an unchanged entry is never sent to Claude again."),
		},
	},
	{
		Name:        "MEMBER",
		Sheet:       GroupsSheet,
		Unique:      []string{"group", "person"},
		Description: "A person's place in one group: whether they run it, whether they are in it by name, whether they lead it, and their answer about coming. Each is a column of its own, so a host with a ticket who said no is one row.",
		Columns: []Column{
			ident(MemberPrefix),
			ref("group", "GROUP").required().about("The group."),
			ref("person", "PERSON").required().about("The person."),
			col("manager", Bool).about("Runs the group and every group under it: edits it, sees everyone in it, sets anyone's place. The only column here that grants anything."),
			enum("member",
				v("yes", "in the group by name: a ticket on a party, a sign-up on an activity, on the guest list of an event, in it for any other kind"),
				v("cancelled", "gave it up, kept for the books: a ticket given back"),
				v("excluded", "kept out even though a rule would put them in: a Loop unsubscribe, someone taken off a group invite")).about("Whether they are in the group by name. Only yes makes an effective member."),
			col("lead", Bool).about("Leads the group: a co-chair, an event lead. Grants nothing; whether a lead also manages is a manager row's."),
			enum("rsvp",
				v("yes", "coming"),
				v("maybe", "might come"),
				v("no", "not coming")).about("Their answer about coming, invited or not. Never a ticket or a sign-up."),
			col("price", Money).about("What was paid for it, in dollars."),
			col("purchase_id", Text).about("The payment provider's reference, shared by the rows bought together."),
			ref("guest_of", "PERSON").about("Who bought or brought them: the buyer of a ticket bought for someone else, the member who brought a guest."),
			col("note", Text).about("Their note on it."),
			col("answered", Moment).about("When they last answered rsvp."),
			ref("answered_by", "PERSON").about("Who gave the rsvp answer: themselves, a parent, a host."),
			enum("via",
				v("page", "on a page"),
				v("mail", "by replying to the invitation")).about("How the rsvp answer came."),
			col("opened", Moment).about("When they first opened the invitation."),
			ref("added_by", "PERSON").about("Who added them."),
			col("added", Moment).about("When they were added."),
		},
	},
	{
		Name:        "WAITLIST",
		Sheet:       GroupsSheet,
		Unique:      []string{"group", "person"},
		Description: "Someone waiting for a place in a full group, a party's ticket. Wanting two places is two rows, the person and a guest of theirs. Giving a place removes the row and makes them a member.",
		Columns: []Column{
			ident(WaitlistPrefix),
			ref("group", "GROUP").required().about("The group they are waiting for."),
			ref("person", "PERSON").required().about("Who is waiting."),
			ref("guest_of", "PERSON").about("Who put them on it, when it isn't themselves."),
			col("note", Text).about("Their note on it."),
			ref("added_by", "PERSON").about("Who added them."),
			col("added", Moment).required().about("When they were added, to the second; first in, first offered."),
		},
	},
	{
		Name:        "RULE",
		Sheet:       GroupsSheet,
		Description: "A rule that takes people into a group's effective members, or keeps them out. It selects with every selector it fills in at once, then may replace the selection with their relatives and keep only those within another group.",
		Columns: []Column{
			ident(RulePrefix),
			ref("group", "GROUP").required().about("The group it adds to or takes from."),
			col("order", Order).required().about("Its place among the group's rules."),
			col("exclude", Bool).about("Keeps whoever it selects out, rather than taking them in."),
			ref("target", "GROUP").about("Selects this group's effective members."),
			ref("person", "PERSON").about("Selects this person."),
			col("search", Text).about("Selects whoever's name or address holds these words."),
			col("property", Text).about("Selects by a PERSON column holding value; the column must be readable by everyone who sees the person."),
			col("value", Text).about("What property must hold."),
			col("descend", Bool).about("Takes in every group under target as well."),
			enum("expand",
				v("parents", "the managers of families they are in but don't manage"),
				v("children", "the non-managers of families they manage"),
				v("household", "everyone in their families, and themselves")).about("Replaces the selection with their relatives."),
			ref("within", "GROUP").about("Keeps only those among this group's effective members."),
		},
	},
	{
		Name:        "DOCUMENT",
		Sheet:       DocumentsSheet,
		Description: "A place some content appears: a node in a tree. A root is a whole thing the community received or published and has a kind; everything else hangs under one through parent and has a relation to it.",
		Columns: []Column{
			ident(DocumentPrefix),
			enum("kind",
				v("newsletter", "a school newsletter"),
				v("list", "mail to a class list"),
				v("page", "a website page"),
				v("portal", "a Veracross portal resource"),
				v("post", "a Loop post"),
				v("calendar", "a version of the school's year-calendar PDF")).about("What a root is to the community."),
			enum("relation",
				v("attachment", "attached to its parent mail"),
				v("inline", "embedded in its parent mail"),
				v("linked", "something its parent links to, fetched as content of its own"),
				v("extract", "text read out of its parent's content, as Markdown")).about("What a non-root is to its parent."),
			ref("parent", "DOCUMENT").about("The document it hangs under."),
			ref("content", "CONTENT").about("Its bytes."),
			col("title", Text).about("Its title."),
			col("date", Date).about("Its date: sent, published or posted."),
			ref("author", "PERSON").about("Who wrote or sent it."),
			col("url", URL).about("Where it lives on the web."),
			col("filename", Text).about("The file name it came with."),
			col("content_id", Text).about("A mail part's Content-ID, which its parent's HTML refers to."),
			ref("message", "MESSAGE").about("The mail it was filed from."),
			col("key_points", Text).about("Its key points, as the inbox shows them."),
			col("index", Blob).about("Ask's search index of its text."),
			col("order", Order).about("Its place among its parent's documents."),
		},
	},
	{
		Name:        "CONTENT",
		Sheet:       DocumentsSheet,
		Unique:      []string{"hash"},
		Description: "Bytes, once each: the same bytes arriving twice are one row and one object in the media bucket.",
		Columns: []Column{
			ident(ContentPrefix),
			col("hash", Text).required().about("The SHA-256 of the bytes."),
			col("blob", Blob).required().about("The object holding them, content/ and the hash."),
			col("mime", Text).required().about("Their type, read from the bytes, never a file name."),
			col("size", Int).required().about("Their length in bytes."),
		},
	},
	{
		Name:        "DOCUMENT_GROUP",
		Sheet:       DocumentsSheet,
		Unique:      []string{"document", "group", "relation"},
		Description: "A document's tie to a group. A document with no sent_to and no for is for everyone.",
		Columns: []Column{
			ident(DocumentGroupPrefix),
			ref("document", "DOCUMENT").required().about("The document."),
			ref("group", "GROUP").required().about("The group."),
			enum("relation",
				v("sent_to", "mailed to the group"),
				v("for", "meant for the group's people"),
				v("attached", "attached to the group, as its file")).required().about("How the document concerns the group."),
		},
	},
	{
		Name:        "REPORT",
		Sheet:       DocumentsSheet,
		Description: "A bug or idea someone sent with Report a problem or idea, and its triage.",
		Columns: []Column{
			ident(ReportPrefix),
			ref("reporter", "PERSON").required().about("Who sent it."),
			col("received", Date).about("When it came in."),
			enum("app", apps...).about("The app it was sent from."),
			enum("kind",
				v("bug", "something broken"),
				v("idea", "something wanted")).about("What it is."),
			enum("status",
				v("new", "not yet looked at"),
				v("filed", "made into a GitHub issue"),
				v("dismissed", "looked at and let go")).required().about("Where its triage stands."),
			col("summary", Text).about("Its one line."),
			col("details", Text).about("What the reporter wrote."),
			col("page_url", Text).about("The page it was sent from."),
			col("browser", Text).about("The browser it was sent from."),
			col("errors", Text).about("The page's recent errors."),
			col("screenshot", Blob).about("A picture of the page."),
			col("issue", Text).about("The GitHub issue it became."),
			ref("handled_by", "PERSON").about("Who triaged it."),
		},
	},
	{
		Name:        "MESSAGE",
		Sheet:       MailSheet,
		Description: "An email in or out: the send queue and the record at once. Kept forever, so the people and groups it names are deactivated or closed, never deleted.",
		Columns: []Column{
			ident(MessagePrefix),
			enum("direction",
				v("in", "received"),
				v("out", "sent, or to be sent")).required().about("Whether it came in or goes out."),
			enum("kind",
				v("post", "a Loop post: the one received, and its fan-out to the list"),
				v("invitation", "an invitation to an event"),
				v("skip", "records invitees not mailed"),
				v("update", "an event's changed details or its cancellation"),
				v("reminder", "a reminder"),
				v("calendar", "a calendar invite for someone going"),
				v("notice", "a notice to hosts or admins"),
				v("reply", "a reply to an invitation"),
				v("forward", "school mail forwarded in")).required().about("What it is."),
			ref("group", "GROUP").about("The group it is for or from."),
			ref("about", "").about("The row it concerns when its group isn't enough, in any table."),
			ref("from_person", "PERSON").about("Who it is from."),
			col("from_address", Email).about("The address it came from."),
			col("subject", Text).about("Its subject."),
			ref("content", "CONTENT").about("Its raw mail."),
			col("header_id", Text).about("Its Message-ID header."),
			ref("parent", "MESSAGE").about("The message it came from: a list post's fan-out, a reply, a cancellation."),
			col("created", Moment).required().about("When it was received or written."),
		},
	},
	{
		Name:        "RECIPIENT",
		Sheet:       MailSheet,
		Unique:      []string{"token"},
		Description: "One copy of an outbound message to one person: queued with only created, then sent, then delivered or failed.",
		Columns: []Column{
			ident(RecipientPrefix),
			ref("message", "MESSAGE").required().about("The message."),
			ref("person", "PERSON").required().about("Who it goes to."),
			token("token", MailTokenPrefix).about("The one secret every link and reply address in this copy carries. Minted by the server."),
			col("provider_id", Text).about("The mail provider's ID for the copy."),
			col("created", Moment).required().about("When it was queued."),
			col("sent", Moment).about("When it was handed to the provider."),
			col("delivered", Moment).about("When the provider delivered it."),
			col("failed", Moment).about("When the provider gave up on it."),
			col("detail", Text).about("Why it failed."),
		},
	},
	{
		Name:        "EFFECTIVE_MEMBER",
		Generated:   true,
		Description: "Who is in a group, and why: its member=yes rows, plus everyone its rules take in, less whoever is excluded or deactivated.",
		Columns: []Column{
			ident(EffectiveMemberPrefix),
			ref("group", "GROUP").about("The group."),
			ref("person", "PERSON").about("The person."),
			col("reasons", Text).about("Why they are in: member, or rule and the rule's order."),
		},
	},
	{
		Name:        "INBOX",
		Generated:   true,
		Description: "A newsletter, class-list mail or post in a person's inbox: one sent to or meant for a group holding them or anyone in their families.",
		Columns: []Column{
			ident(InboxPrefix),
			ref("person", "PERSON").about("Whose inbox."),
			ref("document", "DOCUMENT").about("The document."),
		},
	},
	{
		Name:        "SETTING",
		Sheet:       ConfigSheet,
		Unique:      []string{"app", "key"},
		Description: "A setting of one app, or of the whole site, by key.",
		Columns: []Column{
			ident(SettingPrefix),
			enum("app", append([]Value{v("platform", "the whole site")}, apps...)...).required().about("The app it belongs to."),
			col("key", Text).required().about("Which setting, as the app names it."),
			col("value", Text).about("Its value, as the app reads it."),
		},
	},
	{
		Name:        "CHARITY",
		Sheet:       ConfigSheet,
		Description: "A charity Staff Birthdays can give to.",
		Columns: []Column{
			ident(CharityPrefix),
			col("name", Text).required().about("Its name."),
			col("link", URL).about("The page where a donation is made."),
			col("about", Text).about("The newsletter's one sentence about it."),
			col("allowed", Bool).about("It may still be chosen."),
		},
	},
	{
		Name:        "APP",
		Sheet:       ConfigSheet,
		Unique:      []string{"key"},
		Description: "A community app: what it is called, who sees it in the app switch and on the front page, and who its admins are.",
		Columns: []Column{
			ident(AppPrefix),
			enum("key", apps...).required().about("Which app."),
			col("name", Text).about("What the switch and the front page call it."),
			col("tagline", Text).about("The line under its name."),
			ref("visible_to", "GROUP").required().about("The group whose effective members see it; everyone for all."),
			ref("admins", "GROUP").about("Its admins group; the super admins are admins of every app."),
			col("order", Order).about("Its place in the switch and on the front page."),
		},
	},
	{
		Name:        "WIDGET",
		Sheet:       ConfigSheet,
		Unique:      []string{"key"},
		Description: "A widget on the Heliosian front page: its place and who sees it.",
		Columns: []Column{
			ident(WidgetPrefix),
			enum("key",
				v("when", "Upcoming: the viewer's coming events"),
				v("team", "Team: activities wanting volunteers"),
				v("celebrate", "Celebrate: parties"),
				v("school", "Inbox: school mail")).required().about("Which widget."),
			col("order", Order).about("Its place on the page."),
			ref("visible_to", "GROUP").required().about("The group whose effective members see it; everyone for all."),
		},
	},
	{
		Name:        "GEOCODE",
		Sheet:       ConfigSheet,
		Unique:      []string{"address"},
		AppendOnly:  true,
		Description: "Where an address is on the map, kept so each address is looked up once.",
		Columns: []Column{
			ident(GeocodePrefix),
			col("address", Text).required().about("The address as written."),
			col("lat", Float).about("Its latitude."),
			col("lng", Float).about("Its longitude."),
		},
	},
	{
		Name:        "ALIAS",
		Sheet:       ConfigSheet,
		Unique:      []string{"alias"},
		Description: "Another name for a row: an old ID from the old sites, a friendly address, or a list's extra email address.",
		Columns: []Column{
			ident(AliasPrefix),
			col("alias", Text).required().about("The other name."),
			ref("target", "").required().about("The row it names, in any table."),
		},
	},
	{
		Name:        "REDIRECT",
		Sheet:       ConfigSheet,
		Unique:      []string{"app", "old"},
		Description: "An old path in an app and where it goes now.",
		Columns: []Column{
			ident(RedirectPrefix),
			enum("app", apps...).required().about("The app."),
			col("old", Text).required().about("The old path."),
			col("new", Text).required().about("Where it goes."),
			col("added", Date).about("When it was added."),
		},
	},
	{
		Name:        "INVITE_SERVICE",
		Sheet:       ConfigSheet,
		Unique:      []string{"name"},
		Description: "An invitation service the Invite List Builder exports to (Greenvelope, Evite, ...).",
		Columns: []Column{
			ident(InviteServicePrefix),
			col("name", Text).required().about("What the page calls it."),
			col("header_row", Bool).about("Its export starts with a row of headings."),
			col("supports_groups", Bool).about("It can take a family as one invite."),
			col("description", Text).about("What the page says about it."),
		},
	},
	{
		Name:        "INVITE_TEMPLATE",
		Sheet:       ConfigSheet,
		Description: "One column of an invitation service's export.",
		Columns: []Column{
			ident(InviteTemplatePrefix),
			ref("service", "INVITE_SERVICE").required().about("The service."),
			col("order", Order).required().about("Its place among the service's columns."),
			col("column", Text).about("The heading the export writes."),
			col("template", Text).about("What each row holds, as {{ parameter }} tokens filled in per family or person."),
		},
	},
	{
		Name:        "GREETING",
		Sheet:       ConfigSheet,
		Description: "A way the Invite List Builder can address a family or person: the site's own, or one someone added for themselves.",
		Columns: []Column{
			ident(GreetingPrefix),
			ref("owner", "PERSON").about("Who added it; blank for the site's own."),
			col("name", Text).required().about("What the page calls it."),
			col("format", Text).about("The greeting, as a sample the page fills in with real names."),
			col("grouped", Bool).about("It can address a family as one."),
			col("individual", Bool).about("It can address one person."),
		},
	},
	{
		Name:        ChangesTable,
		Sheet:       EverySheet,
		AppendOnly:  true,
		Description: "The history of every row: one entry per row a commit inserts and per cell it sets or deletes. Written only by the store.",
		Columns: []Column{
			ident(ChangePrefix),
			col("at", Moment).required().about("When, to the second."),
			col("actor", Text).required().about("Who the change was made as, or the work that made it (import, pictures)."),
			col("real_actor", Text).about("Who was signed in, when it differs from actor under Spoof Mode."),
			enum("action",
				v("insert", "a row added"),
				v("set", "a cell changed"),
				v("delete", "a row removed")).required().about("What happened."),
			col("table", Text).required().about("The table changed."),
			ref("row", "").required().about("The row changed, which may since be deleted."),
			col("column", Text).about("The column, for a set or a delete."),
			col("previous", Text).about("What the cell held before, for a set or a delete."),
		},
	},
}

var byName = func() map[string]*Table {
	if err := checkDescribed(Tables); err != nil {
		panic("db: schema: " + err.Error())
	}
	out := map[string]*Table{}
	for i := range Tables {
		out[Tables[i].Name] = &Tables[i]
	}
	return out
}()

func checkDescribed(tables []Table) error {
	for _, t := range tables {
		if strings.TrimSpace(t.Description) == "" {
			return fmt.Errorf("%s has no description", t.Name)
		}
		for _, c := range t.Columns {
			if strings.TrimSpace(c.Description) == "" {
				return fmt.Errorf("%s.%s has no description", t.Name, c.Name)
			}
			seen := map[string]bool{}
			for _, value := range c.Values {
				if seen[strings.ToLower(value.Name)] {
					return fmt.Errorf("%s.%s lists %s twice", t.Name, c.Name, value.Name)
				}
				seen[strings.ToLower(value.Name)] = true
				if strings.TrimSpace(value.Description) == "" {
					return fmt.Errorf("%s.%s value %s has no description", t.Name, c.Name, value.Name)
				}
			}
		}
	}
	return nil
}

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
