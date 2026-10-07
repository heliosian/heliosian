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

func (t Table) In(sheet string) bool {
	return t.Sheet == sheet
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
		v("wiki", "Helios Wiki, parent-to-parent info"),
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
			col("vc_name_long", Text).about("Their full name as the import reads it from vc_name (Juni Ashdown); name_long_override overrides it."),
			col("name_long_override", Text).about("Their full name as set in the app, in place of vc_name_long."),
			generated("name_long").about("Their full name: the override, else the import's."),
			col("vc_name_short", Text).about("Their short name as the import reads it from vc_name (Juni); name_short_override overrides it."),
			col("name_short_override", Text).about("Their short name as set in the app, in place of vc_name_short."),
			generated("name_short").about("Their short name: the override, else the import's."),
			col("vc_name_sort", Text).about("Their sorting name as the import reads it from vc_name (Ashdown, Juni); name_sort_override overrides it."),
			col("name_sort_override", Text).about("Their sorting name as set in the app, in place of vc_name_sort."),
			generated("name_sort").about("Their sorting name: the override, else the import's."),
			generated("name_show").about("The name to show: name_long, else Guest for a guest, else [Missing Name]."),
			enum("vc_grade", grades...).about("A student's grade as Veracross has it. Written by the import; grade_override overrides it."),
			enum("grade_override", grades...).about("A student's grade as set in the app, in place of vc_grade."),
			{Name: "grade", Kind: Enum, Values: grades, Generated: true, Description: "A student's grade: the override, else Veracross's."},
			ref("vc_classroom", "GROUP").about("A student's classroom as Veracross has it. Written by the import; classroom_override overrides it."),
			ref("classroom_override", "GROUP").about("A student's classroom as set in the app, in place of vc_classroom."),
			{Name: "classroom", Kind: Ref, Target: "GROUP", Generated: true, Description: "A student's classroom: the override, else Veracross's."},
			ref("vc_crew", "GROUP").about("A student's crew as Veracross has it. Written by the import; crew_override overrides it."),
			ref("crew_override", "GROUP").about("A student's crew as set in the app, in place of vc_crew."),
			{Name: "crew", Kind: Ref, Target: "GROUP", Generated: true, Description: "A student's crew: the override, else Veracross's."},
			ref("vc_department", "GROUP").about("A staff member's department as Veracross has it. Written by the import; department_override overrides it."),
			ref("department_override", "GROUP").about("A staff member's department as set in the app, in place of vc_department."),
			{Name: "department", Kind: Ref, Target: "GROUP", Generated: true, Description: "A staff member's department: the override, else Veracross's."},
			col("vc_job_title", Text).about("A staff member's job title, Veracross's, else the school website's. Written by the import; job_title_override overrides it."),
			col("job_title_override", Text).about("A staff member's job title as set in the app, in place of vc_job_title."),
			generated("job_title").about("A staff member's job title: the override, else the import's."),
			col("vc_phone", Text).private().about("Their phone number as Veracross has it. Written by the import; phone_override overrides it."),
			col("phone_override", Text).private().about("Their phone number as set in the app, in place of vc_phone."),
			generated("phone").about("Their phone number: the override, else Veracross's; blank unless phone_consent is shared."),
			enum("phone_consent", shares...).about("Whether their phone number is shown, from the opt-in form; written by the consent import."),
			enum("vc_phone_visibility",
				v("visible", "Veracross shows the numbers"),
				v("mixed", "Veracross shows some of them"),
				v("hidden", "Veracross hides them")).about("Veracross's own privacy setting for phone numbers, already applied to the export; kept only to explain it on the profile."),
			enum("address_consent", shares...).about("Whether their address is shown, from the opt-in form; written by the consent import."),
			enum("vc_address_visibility",
				v("full", "Veracross shows the address"),
				v("partial", "Veracross shows part of it"),
				v("hidden", "Veracross hides it")).about("Veracross's own privacy setting for the address, already applied to the export; kept only to explain it on the profile."),
			col("pronouns", Text).about("Their pronouns, as they write them."),
			col("pronunciation", Blob).about("A recording of how their name is said."),
			col("facts", Text).about("Their about-me words on the profile."),
			col("facts_updated", Date).about("When their about-me words last changed."),
			col("photo_updated", Date).about("When their picture last changed."),
			col("vc_bio", Text).about("A staff member's bio from the school website's staff page. Written by the import."),
			col("birthday", Date).about("A staff member's birthday, for Staff Birthdays; the year is not used."),
			enum("consent", consents...).private().about("Whether they consented to be in the directory, from the opt-in form; written by the consent import. Hidden from everyone but the import."),
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
			ref("person", "PERSON").required().about("Whose address it is."),
			col("address", Email).required().about("The address, in lower case. Never a .noemail placeholder."),
			col("primary", Bool).about("The address mail goes to. Exactly one per person with any address."),
			enum("source", sources...).required().about("Where the address came from."),
			{Name: "guest", Kind: Bool, Generated: true, Description: "Whether a guest holds it: source is guest."},
		},
	},
	{
		Name:        "PHOTO",
		Sheet:       PeopleSheet,
		Unique:      []string{"person", "group", "original"},
		Generate:    photoGenerated,
		Description: "A picture of a person or a group, exactly one: a portrait, a family photo, a classroom or grade tile, a category's or activity's picture. The first in order is the one shown. Added through /api/do/photo; the server makes the re-encode, crop and thumbnail after the commit.",
		Columns: []Column{
			ident(PhotoPrefix),
			ref("person", "PERSON").about("The person it is of."),
			ref("group", "GROUP").about("The group it is of."),
			col("order", Order).about("Its place among the person's or group's pictures; the first is shown."),
			col("original", Blob).required().private().about("The picture as uploaded, which can carry location and face tags."),
			col("reencode", Blob).about("The original turned upright, its tags dropped, as a JPEG at most 2048 pixels on its long side. Made by the server."),
			col("crop_left", Int).about("The crop box's left edge, in pixels of the re-encode."),
			col("crop_top", Int).about("The crop box's top edge, in pixels of the re-encode."),
			col("crop_width", Int).about("The crop box's width, in pixels of the re-encode."),
			col("crop_height", Int).about("The crop box's height, in pixels of the re-encode."),
			col("crop", Blob).about("The re-encode cut to the box. Made by the server."),
			col("thumbnail", Blob).about("A small copy of the image. Made by the server."),
			{Name: "image", Kind: Blob, Generated: true, Description: "The picture to show: the crop, else the re-encode."},
			col("ready", Bool).about("The made pictures match the current original and box. Set by the server; cleared when either changes."),
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
			col("assigned", Moment).about("When it was assigned to them."),
			col("ask_by", Date).about("The day to ask the staff member that the assignee's calendar invite last named; a new invite goes out when the day moves."),
			col("contacted", Moment).about("When the staff member was asked for their charity."),
			ref("contacted_by", "PERSON").about("Who asked them."),
			enum("participation",
				v("full", "asked, given for and in the newsletter"),
				v("skip", "left out altogether"),
				v("no_newsletter", "asked and given for, never in the newsletter")).about("How they want to take part."),
			ref("charity", "CHARITY").about("The charity given to in their name."),
			col("note", Text).about("The staff member's own words about their choice."),
			col("recorded", Moment).about("When the charity was recorded."),
			ref("recorded_by", "PERSON").about("Who recorded it; blank when the weekly copy to the shared sheet chose it for them."),
			col("published", Moment).about("When it was copied for the newsletter."),
			ref("published_by", "PERSON").about("Who copied it for the newsletter; blank for the weekly copy."),
		},
	},
	{
		Name:        "COLLECTION",
		Sheet:       PeopleSheet,
		Unique:      []string{"token"},
		Description: "A set of things a person keeps: a calendar they saved in When, things they set aside. What it holds is its COLLECTION_GROUP rows. Each is also a personal feed address.",
		Columns: []Column{
			ident(CollectionPrefix),
			ref("person", "PERSON").required().about("Whose collection it is."),
			col("name", Text).about("What they call it."),
			col("emoji", Text).about("The mark they gave it."),
			col("order", Order).about("Its place among their collections."),
			col("default", Bool).about("The calendar When opens to for them."),
			token("token", FeedTokenPrefix).about("The secret in its feed address. Minted by the server."),
		},
	},
	{
		Name:        "COLLECTION_GROUP",
		Sheet:       PeopleSheet,
		Unique:      []string{"collection", "clause", "relation", "group"},
		Description: "A group a collection is made from, and how a thing must stand to it. Rows sharing a clause are alternatives; a thing is in the collection when it meets every clause, and a collection with no rows holds everything.",
		Columns: []Column{
			ident(CollectionGroupPrefix),
			ref("collection", "COLLECTION").required().about("The collection."),
			col("clause", Int).required().about("The clause it is an alternative in."),
			enum("relation",
				v("under", "the group itself or anything under it through parent"),
				v("for", "anything one of whose rules names the group as target or within")).required().about("How a thing must stand to the group."),
			ref("group", "GROUP").required().about("The group."),
		},
	},
	{
		Name:        "GROUP",
		Sheet:       GroupsSheet,
		Generate:    groupGenerated,
		Description: "Any set of people, anything things are filed under, and anything on the calendar. A group sits under its parent; a thing's category is its parent. Its managers are the effective members of its managed_by group and of every group above it.",
		Columns: []Column{
			ident(GroupPrefix),
			ref("parent", "GROUP").about("The group it sits under: its category, the event or activity it is part of, a recurring event for an instance, its celebration, its band or classroom, its day."),
			enum("kind",
				v("family", "a household: it manages itself, its students its children and everyone else its parents; only the import changes who is in it"),
				v("classroom", "a school classroom, kept from people's classrooms"),
				v("grade", "a school grade, kept from people's grades"),
				v("band", "a band of grades and classrooms; it takes in each grade and classroom under it by a rule the import keeps"),
				v("crew", "a crew within a classroom"),
				v("department", "a staff department, Veracross's"),
				v("event", "a When event; on the calendar when it has a start, and a recurring event when it has none and its instances under it"),
				v("day", "a school day on its date, under its day-type category, for the classrooms its rules name or all of them"),
				v("day_part", "a part of a school day: Dropoff, School, Pickup or Aftercare"),
				v("activity", "a Team activity; each school year is a root activity holding that year's"),
				v("party", "a Celebrate party"),
				v("celebration", "a Celebrate celebration, holding its parties"),
				v("category", "a heading or filter things are filed under through their parent"),
				v("group", "a group that is only a set of people: role groups, tags, Loop lists, audiences, room parents, a party's waitlist, a group's managers, who said they are coming or not"),
				v("admins", "an app's admins, named by APP.admins")).required().about("What the group is, where it shows a different way."),
			enum("status",
				v("pending", "awaiting approval; seen by its managers and its app's admins alone"),
				v("open", "live"),
				v("done", "an activity that happened, still shown"),
				v("cancelled", "called off"),
				v("closed", "gone from every list and page, kept for the messages naming it")).about("Where it stands."),
			col("slug", Text).about("Its short name in addresses: a page's friendly address, a list's email address, a grade's grade-k to grade-8."),
			col("name", Text).about("Its name. A family's is written by a trigger from its members' last names. A managers group named <name> Managers, a waitlist named <name> Waitlist, or answer groups named <name> Going and <name> Not Going, are renamed with the group they serve."),
			col("subtitle", Text).about("A line under the name."),
			col("description", Text).about("What it is, in words. A category's is what the calendar classifier reads."),
			col("color", Text).about("A classroom's or grade's color."),
			col("flyer", Blob).about("A poster, shown whole."),
			col("pronunciation", Blob).about("A recording of how a family's name is said."),
			col("url", URL).about("A page elsewhere that it stands for, so a link is a group like any other."),
			col("listed", Bool).about("Shown in its app's lists; a group of kind group is also shown in Who? and the pickers."),
			col("default", Bool).about("A When category the calendar's filter starts with on."),
			col("order", Order).about("Its place among the groups beside it."),
			ref("visible_to", "GROUP").about("The group whose effective members can see it: everyone for all, the group itself for its members, blank for its managers and its app's admins alone (a hidden group). Its managers and its app's admins always can."),
			ref("members_visible_to", "GROUP").about("The group whose effective members can see who is in it, among those who can see it: everyone, the group itself, or blank for its managers alone."),
			ref("managed_by", "GROUP").about("The group whose effective members manage it and every group under it: edit it, see everyone in it, set anyone's place. A family manages itself, and a managers group usually itself; blank for none but its app's admins and the managers of the groups above it."),
			col("vc_address", Text).private().about("A family's address as Veracross has it. Written by the import; address_override overrides it."),
			col("address_override", Text).private().about("Its address as set in the app, in place of vc_address."),
			generated("address").about("Its address: the override, else Veracross's; for a family, blank unless address_consent is shared."),
			enum("address_consent", shares...).about("Whether a family's address is shown: shared only when every parent's is. Written by the consent import."),
			col("vc_phone", Text).private().about("A family's phone number as Veracross has it. Written by the import; phone_override overrides it."),
			col("phone_override", Text).private().about("Its phone number as set in the app, in place of vc_phone."),
			generated("phone").about("Its phone number: the override, else Veracross's; for a family, blank unless phone_consent is shared."),
			enum("phone_consent", shares...).about("Whether a family's phone number is shown: shared only when every parent's is. Written by the consent import."),
			enum("consent", consents...).private().about("A family's consent: listed only when every parent's is. Written by the consent import."),
			col("mail", Bool).about("Has an email address and is a list in Loop."),
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
			ref("eligible", "GROUP").about("The group whose effective members may join; blank for anyone."),
			col("capacity", Int).about("How many places there are; blank for no limit."),
			col("minimum", Int).about("How many it needs to go ahead."),
			ref("waitlist", "GROUP").about("The group whose members wait for a place in it when it is full, in the order their membership was added; set means it takes a waitlist. It sits under the party, whose managers manage it. Wanting two places is two members, the person and a guest of theirs; giving a place removes the waitlist membership and makes them a member here."),
			ref("rsvp_yes", "GROUP").about("The group of those who said they are coming, each membership's added and added_by when and by whom; set, with rsvp_no, means it takes answers. It sits under the group it answers, whose managers manage it. Joining it takes a person out of rsvp_no; in neither is maybe, or no answer."),
			ref("rsvp_no", "GROUP").about("The group of those who said they are not coming, each membership's added and added_by when and by whom. It sits under the group it answers, whose managers manage it. Joining it takes a person out of rsvp_yes."),
			col("lead_needed", Bool).about("An activity still wants a co-chair."),
			col("priority", Bool).about("An activity the community most needs hands for, marked by an admin."),
			col("price", Money).about("A party's price per place, in dollars."),
			col("unit", Text).about("What a party's price is per, in words (person, adult, couple)."),
			col("parent_ticket_required", Bool).about("A student's party place needs a parent with one."),
			col("drop_off_allowed", Bool).about("Students may be dropped off at a party."),
			col("start", Moment).about("When it starts; set means it is on the calendar. A date alone with all_day."),
			col("end", Moment).about("When it ends; for an all-day one, the last day it runs."),
			col("all_day", Bool).about("It runs whole days, without times."),
			col("timing", Text).about("When it happens, in words, where start can't say it (All Year, September/October)."),
			col("location", Text).about("Where it happens."),
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
			col("name", Text).about("The name as the source has it: the event's title, the PDF entry's words."),
			col("description", Text).about("The description as the source has it, as plain text."),
			col("start", Moment).about("The start as the source has it."),
			col("end", Moment).about("The end as the source has it."),
			col("all_day", Bool).about("Whether the source has it as whole days."),
			col("location", Text).about("The location as the source has it."),
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
		Description: "A person's place in one group: whether they are in it by name and whether they lead it, each a column of its own, so a co-chair who signed up is one row. Who runs a group is GROUP.managed_by; who said they are coming is GROUP.rsvp_yes and rsvp_no.",
		Columns: []Column{
			ident(MemberPrefix),
			ref("group", "GROUP").required().about("The group."),
			ref("person", "PERSON").required().about("The person."),
			enum("member",
				v("yes", "in the group by name: a ticket on a party, a sign-up on an activity, on the guest list of an event, a place on a party's waitlist, in it for any other kind"),
				v("cancelled", "gave it up, kept for the books: a ticket given back"),
				v("excluded", "kept out even though a rule would put them in: a Loop unsubscribe, someone taken off a group invite")).about("Whether they are in the group by name. Only yes makes an effective member."),
			col("lead", Bool).about("Leads the group: a co-chair, an event lead. Grants nothing; whether a lead also manages is whether they are in its managed_by group."),
			col("opened", Moment).about("When they first opened the invitation."),
			ref("guest_of", "PERSON").about("Who bought or brought them: the buyer of a ticket bought for someone else, the member who brought a guest, who put them on a waitlist."),
			col("price", Money).about("What was paid for it, in dollars."),
			col("purchase_id", Text).about("The payment provider's reference, shared by the rows bought together."),
			col("note", Text).about("Their note on it."),
			ref("added_by", "PERSON").about("Who added them."),
			col("added", Moment).about("When they were added; to the second on a waitlist, whose first in is first offered."),
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
			enum("replace_with",
				v("parents", "for a student, everyone in their families who isn't a student"),
				v("children", "for someone who isn't a student, the students in their families"),
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
			ref("parent", "DOCUMENT").about("The document it hangs under."),
			enum("kind",
				v("mail", "an email the community received, its content the raw message; a Loop post's also names its MESSAGE"),
				v("calendar", "a version of the school's year-calendar PDF, which the calendar import reads"),
				v("file", "a file the school or HCA shared, such as a slide deck, its content the file as it was fetched"),
				v("wiki", "a page of Helios Wiki, its content the page's Markdown")).about("What a root is: mail, the year calendar, a shared file or a wiki page; where it came from is its url, author and the groups it was sent to."),
			enum("relation",
				v("part", "a MIME part of its parent message"),
				v("linked", "something its parent links to, fetched as content of its own"),
				v("image", "an image its parent shows, fetched as content of its own"),
				v("extract", "text read out of its parent's content, as Markdown"),
				v("pages", "a run of its parent PDF's pages, split off to be read on its own; its name says which"),
				v("side", "a card beside its parent wiki page: its name the card's title, its content the card's Markdown")).about("What a non-root is to its parent."),
			col("order", Order).about("Its place among its parent's documents."),
			col("name", Text).about("Its title."),
			col("published", Moment).about("When it was sent, published or posted, to the second."),
			ref("author", "PERSON").about("Who wrote or sent it."),
			col("url", URL).about("Where it lives on the web."),
			enum("fetch",
				v("gone", "the address answered that it is not there, so it is never asked again"),
				v("sign_in", "the address wants someone signed in, so it waits for a fetch run with their credentials"),
				v("refused", "what the address answered is not kept: the wrong type, too large, or a tracking pixel"),
				v("skipped", "Claude judged the link not worth fetching")).about("Why a linked document with no content stopped being fetched, or was never fetched; blank while it is still to fetch."),
			enum("link",
				v("form", "a form to fill in"),
				v("signup", "a sign-up, RSVP, volunteer or ticket page"),
				v("social", "a social media profile or post"),
				v("homepage", "a site's front page, saying nothing about this email"),
				v("per_recipient", "a link made for the one person it was sent to: an account, a preference or an RSVP token"),
				v("tracking", "a mailer's tracking or unsubscribe link"),
				v("media", "a video, a map, an app store page or a photo gallery"),
				v("other", "anything else not worth fetching")).about("For a link Claude judged not worth fetching, what it judged the link to be."),
			col("filename", Text).about("The file name it came with."),
			col("content_id", Text).about("A mail part's Content-ID, which its parent's HTML refers to."),
			ref("content", "CONTENT").about("Its bytes."),
			ref("message", "MESSAGE").about("The mail it was filed from."),
			col("key_points", Text).about("Its key points, as the inbox shows them."),
			col("extracted", Moment).about("When the server finished making the nodes read out of its content; blank while that work is still to do."),
			col("slug", Text).about("For a wiki page, its own part of its address in place of the one its title gives, after its parents' parts: lowercase letters, digits and single hyphens, at most 40, never shaped like an id, no two pages under the same parent the same."),
			col("hidden", Bool).about("For a wiki page, kept out of the wiki's lists of pages for all but its author and the wiki's admins; its link still opens it.")},
	},
	{
		Name:        "CONTENT",
		Sheet:       DocumentsSheet,
		Unique:      []string{"hash", "mime"},
		Description: "Bytes of one type, once each: the same bytes of the same type arriving twice are one row, and the same bytes are one object in the media bucket whatever their type.",
		Columns: []Column{
			ident(ContentPrefix),
			col("hash", Text).required().about("The SHA-256 of the bytes."),
			col("blob", Blob).required().about("The object holding them, content/ and the hash."),
			col("mime", Text).required().about("Their type: read from the bytes, or text/markdown for a reading made of something else; never a file name."),
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
			enum("app", apps...).about("The app it was sent from."),
			enum("kind",
				v("bug", "something broken"),
				v("idea", "something wanted")).about("What it is."),
			col("summary", Text).about("Its one line."),
			col("details", Text).about("What its sender wrote."),
			col("url", Text).about("The page it was sent from."),
			col("page", Text).about("That page's title."),
			col("browser", Text).about("The browser it was sent from, as its user agent names it."),
			col("viewport", Text).about("The size of the browser's window, in CSS pixels."),
			col("screen", Text).about("The size of the screen, in CSS pixels, and its pixel ratio."),
			col("language", Text).about("The browser's language."),
			col("time_zone", Text).about("The browser's time zone."),
			col("errors", Text).about("The page's recent errors."),
			col("screenshot", Blob).about("A picture of the page."),
			enum("status",
				v("new", "not yet looked at"),
				v("filed", "made into a GitHub issue"),
				v("dismissed", "looked at and let go")).required().about("Where its triage stands."),
			col("issue", Text).about("The GitHub issue it became."),
			col("handled", Moment).about("When it was triaged."),
			ref("handled_by", "PERSON").about("Who triaged it."),
			ref("added_by", "PERSON").required().about("Who sent it."),
			col("added", Moment).about("When it came in."),
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
				v("calendar", "a calendar invite: to someone going, or to a birthday's assignee for the day to ask"),
				v("notice", "a notice to hosts or admins"),
				v("reply", "a reply to an invitation"),
				v("forward", "school mail forwarded in")).required().about("What it is."),
			ref("group", "GROUP").about("The group it is for or from."),
			ref("about", "").about("The row it concerns when its group isn't enough, in any table."),
			ref("parent", "MESSAGE").about("The message it came from: a list post's fan-out, a reply, a cancellation."),
			ref("from_person", "PERSON").about("Who it is from."),
			col("from_address", Email).about("The address it came from."),
			col("subject", Text).about("Its subject."),
			col("header_id", Text).about("Its Message-ID header."),
			ref("content", "CONTENT").about("Its raw mail."),
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
		Name:        "SEARCH",
		Generated:   true,
		Description: "A group's or person's search entry: the input built from what every reader of it may read, and what Claude and Vertex made of it, kept in the media bucket under its object. Read from the searcher's index as it stands, so an entry made since the last commit shows at once.",
		Columns: []Column{
			ident(SearchPrefix),
			ref("target", "").about("The group or person it is the entry of."),
			col("input", Text).about("The search input, built from the row."),
			col("summary", Text).about("Claude's summary of it, shown under its name in search results; blank until made."),
			col("keywords", Text).about("Claude's words someone might type looking for it, comma-separated; blank until made."),
			col("chunks", Int).about("How many pieces the input was cut into, each embedded by Vertex; blank until made."),
			col("object", Text).about("Where the entry is kept in the media bucket: search/ and the input's SHA-256."),
			col("made", Bool).about("Whether the entry is in the index; no while it waits for Claude and Vertex."),
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
			col("description", Text).about("The newsletter's one sentence about it."),
			col("url", URL).about("The page where a donation is made."),
			col("ein", Text).about("Its US employer identification number, which shows it is a registered nonprofit."),
			col("allowed", Bool).about("It may still be chosen."),
			col("not_allowed_reason", Text).about("Why it may no longer be chosen."),
			col("added", Moment).about("When it was added."),
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
			col("subtitle", Text).about("The line under its name."),
			ref("admins", "GROUP").about("Its admins group; the super admins are admins of every app."),
			ref("visible_to", "GROUP").required().about("The group whose effective members see it; everyone for all."),
			col("order", Order).about("Its place in the switch and on the front page."),
		},
	},
	{
		Name:        "WIDGET",
		Sheet:       ConfigSheet,
		Unique:      []string{"key", "group"},
		Description: "A widget on the Heliosian front page: its place and who sees it. A section of links is a widget naming the group its links sit under.",
		Columns: []Column{
			ident(WidgetPrefix),
			enum("key",
				v("when", "Upcoming: the viewer's coming events"),
				v("team", "Team: activities wanting volunteers"),
				v("celebrate", "Celebrate: parties"),
				v("school", "Inbox: school mail"),
				v("birthday", "Birthdays: the staff birthdays in the next two newsletters"),
				v("todo", "Reminders: the to-dos the viewer's mail asks of them"),
				v("apps", "Apps: the community apps the viewer sees"),
				v("links", "A section of links: the groups with a url under its group")).required().about("Which widget."),
			ref("group", "GROUP").about("For a section of links, the group its links sit under by parent."),
			col("name", Text).about("For a section, its heading."),
			col("icon", Text).about("For a section, the mark beside its heading: icon:<name>, or an emoji."),
			enum("style",
				v("grid", "an icon grid"),
				v("list", "a compact list")).about("For a section, how its links or apps are laid out."),
			col("descriptions", Bool).about("For a section, whether each link's description or app's subtitle shows under its name."),
			col("sidebar", Bool).about("Its first few items also show in the page's rail."),
			ref("visible_to", "GROUP").required().about("The group whose effective members see it; everyone for all."),
			col("order", Order).about("Its place on the page."),
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
			col("description", Text).about("What the page says about it."),
			col("header_row", Bool).about("Its export starts with a row of headings."),
			col("grouped", Bool).about("It can take a family as one invite."),
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
			col("name", Text).required().about("What the page calls it."),
			col("format", Text).about("The greeting, as a sample the page fills in with real names."),
			col("grouped", Bool).about("It can address a family as one."),
			col("individual", Bool).about("It can address one person."),
			ref("added_by", "PERSON").about("Who added it; blank for the site's own."),
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
