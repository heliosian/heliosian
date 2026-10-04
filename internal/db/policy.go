package db

import (
	"fmt"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/store"
)

const policySource = `
;; Definitions

; the viewer is an accepted manager of @g or of a group above it
(define (manages @g)
  (exists MEMBER (in group (ancestors @g)) (= person @viewer) (= role "manager") (= status "yes")))

; the members of every family @p manages
(define (household @p)
  (select MEMBER.person
    (in group (select MEMBER.group (= person @p) (= role "manager") (= group.kind "family")))
    (= role "member")))

; @g is one of the role groups the import keeps: students, parents, staff, adults, everyone
(define (role_group @g)
  (and (= @g.kind "group") (in @g.slug "students" "parents" "staff" "adults" "everyone")))

; every group @p has a membership row in
(define (groups_of @p)
  (select MEMBER.group (= person @p)))

; the viewer is a super admin, or in effect a member of the app's admins group
(define (admin_of app)
  (or (super_admin)
      (exists EFFECTIVE_MEMBER (in group (select APP.admins (= key app))) (= person @viewer))))

; the viewer is in effect a member of super-admins
(define (super_admin)
  (exists EFFECTIVE_MEMBER (= group.slug "super-admins") (= person @viewer)))

; @g is not closed, and the viewer manages it or its visibility lets them see it
(define (visible @g)
  (and (!= @g.status "closed")
       (or (manages @g)
           (and (!= @g.status "pending")
                (or (= @g.visibility "everyone")
                    (and (= @g.visibility "members")
                         (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer)))
                    (and (= @g.visibility "group")
                         (exists EFFECTIVE_MEMBER (= group @g.visible_to) (= person @viewer))))))))

; the viewer may see @g, and @g lets them see its members
(define (sees_members @g)
  (and (visible @g)
       (or (manages @g)
           (blank @g.members_visible)
           (= @g.members_visible "everyone")
           (and (= @g.members_visible "members")
                (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer))))))

; @p is the viewer, a guest in one of the viewer's groups or in a group whose members the viewer sees, or anyone else not hidden and active; the consent step has already removed everyone unconsented
(define (person_visible @p)
  (or (= @p @viewer)
      (and (= @p.source "guest")
           (or (exists MEMBER (in group (groups_of @viewer)) (= person @p))
               (exists MEMBER (= person @p) (sees_members group))))
      (and (!= @p.source "guest") (not @p.hidden) (blank @p.deactivated))))

; @g is an event, a school day or a part of one
(define (calendar_kind @g)
  (in @g.kind "event" "day" "day_part"))

; the viewer is @p or a manager of @p's family
(define (self_or_household @p)
  (or (= @p @viewer) (in @p (household @viewer))))

; the viewer manages @g, or is in it and it is not for its managers alone
(define (sees_mail @g)
  (or (manages @g)
      (and (!= @g.visibility "managers")
           (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer)))))

; @d was sent to no group that takes mail, or to one whose mail the viewer sees
(define (document_visible @d)
  (or (not (exists DOCUMENT_GROUP (= document @d) (= relation "sent_to") group.mail))
      (exists DOCUMENT_GROUP (= document @d) (= relation "sent_to") group.mail (sees_mail group))))

;; Everyone

; people the viewer may see
(read PERSON (person_visible @row))
; every column of a person but what the form shares
(read PERSON
  (id source vc_name vc_legal_name vc_grade vc_classroom vc_crew vc_department vc_job_title vc_bio
   vc_address_visibility vc_phone_visibility name_long_import name_short_import name_sort_import
   name_long_override name_short_override name_sort_override name_long name_short name_sort
   name_show grade classroom crew job_title department phone facts pronouns pronunciation
   facts_updated photo_updated birthday)
  true)
; when a person last signed out everywhere and when the import dropped them, for the server alone
(read PERSON (signed_out deactivated) false)
; whether the form shares a person's address and phone, to them and their family's managers
(read PERSON (address_consent phone_consent) (self_or_household @row))
; a person or a manager of their family overrides their long name
(set PERSON.name_long_override (self_or_household @old))
; a person or a manager of their family overrides their short name
(set PERSON.name_short_override (self_or_household @old))
; a person or a manager of their family overrides their sort name
(set PERSON.name_sort_override (self_or_household @old))
; the emails of people the viewer may see
(read PERSON_EMAIL (person_visible person))
; every column of an email
(read PERSON_EMAIL (id address person primary source guest) true)
; the photos of people and groups the viewer may see
(read PHOTO
  (or (and (not (blank person)) (person_visible person))
      (and (not (blank group)) (visible group))))
; every column of a photo but its original, its re-encode and its crop
(read PHOTO (id person group crop_left crop_top crop_width crop_height image thumbnail ready order) true)
; a photo's re-encode and crop, to its person and their family's managers, or its group's managers
(read PHOTO (reencode crop)
  (or (and (not (blank person)) (self_or_household person))
      (and (not (blank group)) (manages group))))
; the viewer's own app settings
(read PERSON_SETTING (= person @viewer))
; every column of an app setting
(read PERSON_SETTING (id person app key value) true)
; the birthday years of people the viewer may see
(read BIRTHDAY_YEAR (person_visible person))
; every column of a birthday year
(read BIRTHDAY_YEAR
  (id person year assigned_to contacted_on contacted_by charity participation used_on note)
  true)
; the viewer's own saved calendars
(read SAVED_VIEW (= person @viewer))
; every column of a saved calendar
(read SAVED_VIEW (id token person name groups emoji order is_default) true)
; the viewer's own bug reports and ideas
(read REPORT (= reporter @viewer))
; every column of a bug report or idea
(read REPORT
  (id reporter received app kind status summary details page_url browser errors screenshot
   issue handled_by)
  true)

; groups the viewer may see
(read GROUP (visible @row))
; every column of a group but what its family's form shares
(read GROUP
  (id parent kind listed mail slug title subtitle description
   color flyer pronunciation address phone status visibility visible_to members_visible posting
   replying join adding capacity minimum price unit waitlist eligible parent_required
   manager_needed priority start end all_day timing location url default order added_by added)
  true)
; whether a family's adults all share their address and phone, to its members
(read GROUP (address_consent phone_consent) (exists MEMBER (= group @row) (= person @viewer)))
; where the calendar groups the viewer may see came from
(read GROUP_SOURCE (visible group))
; every column of where a calendar group came from
(read GROUP_SOURCE
  (id group calendar_event document title start end all_day location description marker hash)
  true)
; the rules of groups the viewer manages
(read RULE (manages group))
; every column of a rule
(read RULE (id group order exclude target person search property value descend expand within) true)
; the viewer's own, household's and guests' memberships, those in groups they manage, visible groups' managers, and members where shown
(read MEMBER
  (or (self_or_household person)
      (= guest_of @viewer)
      (manages group)
      (and (= role "manager") (visible group))
      (and (= role "member") (sees_members group))))
; every column of a membership but what it cost
(read MEMBER
  (id group person role status lead quantity guest_of note answered answered_by via opened
   added_by added)
  true)
; what a membership cost and its payment reference, to the member, their host and the group's managers
(read MEMBER (price purchase_id) (or (= person @viewer) (= guest_of @viewer) (manages group)))
; a person, their family's manager or their host answers an invitation; the group's managers set any status
(set MEMBER.status
  (or (and (or (self_or_household @old.person)
               (= @old.guest_of @viewer))
           (in @new.status "yes" "maybe" "no" "cancelled"))
      (manages @old.group)))
; the viewer's own and household's effective memberships, and members of groups that show them
(read EFFECTIVE_MEMBER
  (or (self_or_household person)
      (sees_members group)))
; every column of an effective membership
(read EFFECTIVE_MEMBER (id group person status reasons) true)

; documents sent to no group that takes mail, or to one whose mail the viewer sees
(read DOCUMENT (document_visible @row))
; every column of a document
(read DOCUMENT (id kind title date author url message object hash key_points indexed order) true)
; links between a document and a group, where the viewer may see both
(read DOCUMENT_GROUP (and (document_visible document) (visible group)))
; every column of a link between a document and a group
(read DOCUMENT_GROUP (id document group relation) true)
; the viewer's own inbox
(read INBOX (= person @viewer))
; every column of an inbox entry
(read INBOX (id person document) true)

; mail the viewer sent or received, and mail to groups they manage
(read MESSAGE
  (or (= from_person @viewer)
      (exists RECIPIENT (= message @row) (= person @viewer))
      (manages group)))
; every column of a message
(read MESSAGE
  (id direction kind group about from_person from_address subject object header_id parent created)
  true)
; the viewer's own deliveries, deliveries of mail they sent, and of mail to groups they manage
(read RECIPIENT
  (or (= person @viewer)
      (= message.from_person @viewer)
      (manages message.group)))
; every column of a delivery but its link token
(read RECIPIENT (id message person provider_id created sent delivered failed detail) true)
; a delivery's link token, to its recipient alone
(read RECIPIENT (token) (= person @viewer))

; app settings
(read SETTING true)
; every column of an app setting
(read SETTING (id app key value) true)
; the birthday charities
(read CHARITY true)
; every column of a charity
(read CHARITY (id name link about allowed) true)
; apps open to everyone or to a group the viewer is in
(read APP
  (or (blank visible_to)
      (exists EFFECTIVE_MEMBER (= group @row.visible_to) (= person @viewer))))
; every column of an app
(read APP (id key name tagline visible_to admins order) true)
; front-page widgets open to everyone or to a group the viewer is in
(read WIDGET
  (or (blank visible_to)
      (exists EFFECTIVE_MEMBER (= group @row.visible_to) (= person @viewer))))
; every column of a widget
(read WIDGET (id key order visible_to) true)
; the coordinates of a family address the viewer may see; a withheld address is blank, so it matches nothing
(read GEOCODE
  (exists GROUP @f (= kind "family") (= address @row.address) (visible @f)))
; every column of a geocode
(read GEOCODE (id address lat lng) true)
; the invite list services
(read INVITE_SERVICE true)
; every column of an invite list service
(read INVITE_SERVICE (id name header_row supports_groups description) true)
; the invite list templates
(read INVITE_TEMPLATE true)
; every column of an invite list template
(read INVITE_TEMPLATE (id service order column template) true)
; shared greetings and the viewer's own
(read GREETING (or (blank owner) (= owner @viewer)))
; every column of a greeting
(read GREETING (id owner name format grouped individual) true)

;; Who? admins

; every person, hidden or deactivated too
(read PERSON (admin_of "who"))
; which people are hidden
(read PERSON (hidden) (admin_of "who"))
; override anyone's long name
(set PERSON.name_long_override (admin_of "who"))
; override anyone's short name
(set PERSON.name_short_override (admin_of "who"))
; override anyone's sort name
(set PERSON.name_sort_override (admin_of "who"))
; every directory group and plain group, pending too
(read GROUP (and (admin_of "who") (in kind "family" "group" "classroom" "grade" "band" "crew" "department")))
; every directory group's and plain group's memberships
(read MEMBER (and (admin_of "who") (in group.kind "family" "group" "classroom" "grade" "band" "crew" "department")))
; every directory group's and plain group's effective members
(read EFFECTIVE_MEMBER (and (admin_of "who") (in group.kind "family" "group" "classroom" "grade" "band" "crew" "department")))
; the rules that pick each plain group's members
(read RULE (and (admin_of "who") (= group.kind "group")))

;; When admins

; every calendar group: events, days and their parts
(read GROUP (and (admin_of "when") (in kind "event" "day" "day_part")))
; open, cancel or close an event
(set GROUP.status (and (admin_of "when") (= @old.kind "event")))
; where every calendar group came from
(read GROUP_SOURCE (and (admin_of "when") (in group.kind "event" "day" "day_part")))
; every event's invitees and hosts
(read MEMBER (and (admin_of "when") (= group.kind "event")))
; make an invitee a host, or a host an invitee
(set MEMBER.role (and (admin_of "when") (= @old.group.kind "event")))
; every event's effective invitees
(read EFFECTIVE_MEMBER (and (admin_of "when") (= group.kind "event")))
; the rules that say who events and day parts are for
(read RULE (and (admin_of "when") (in group.kind "event" "day_part")))
; all mail about events
(read MESSAGE (and (admin_of "when") (= group.kind "event")))
; every delivery of mail about events
(read RECIPIENT (and (admin_of "when") (= message.group.kind "event")))

;; Team admins

; every activity
(read GROUP (and (admin_of "team") (in kind "activity")))
; open, finish, cancel or close an activity
(set GROUP.status (and (admin_of "team") (in @old.kind "activity")))
; every activity's volunteers and chairs
(read MEMBER (and (admin_of "team") (in group.kind "activity")))
; make a volunteer a chair, or a chair a volunteer
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
; open, cancel or close a party or celebration
(set GROUP.status (and (admin_of "celebrate") (in @old.kind "party" "celebration")))
; every party's and celebration's ticket holders and hosts
(read MEMBER (and (admin_of "celebrate") (in group.kind "party" "celebration")))
; what every party ticket cost and its payment reference
(read MEMBER (price purchase_id) (and (admin_of "celebrate") (in group.kind "party")))
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

; every group that takes mail
(read GROUP (and (admin_of "loop") mail))
; open or close a group that takes mail
(set GROUP.status (and (admin_of "loop") @old.mail))
; every mail group's members and managers
(read MEMBER (and (admin_of "loop") group.mail))
; make a member a manager, or a manager a member
(set MEMBER.role (and (admin_of "loop") @old.group.mail))
; every mail group's effective members
(read EFFECTIVE_MEMBER (and (admin_of "loop") group.mail))
; the rules that pick each mail group's members
(read RULE (and (admin_of "loop") group.mail))
; all mail to groups that take it
(read MESSAGE (and (admin_of "loop") group.mail))
; every delivery of mail to groups that take it
(read RECIPIENT (and (admin_of "loop") message.group.mail))
; every post sent to a group that takes mail
(read DOCUMENT (and (admin_of "loop") (exists DOCUMENT_GROUP (= document @row) (= relation "sent_to") group.mail)))
; which mail groups each post was sent to
(read DOCUMENT_GROUP (and (admin_of "loop") group.mail))

;; Home admins

; every plain group and admins group
(read GROUP (and (admin_of "home") (in kind "group" "admins")))
; the members of plain groups and admins groups
(read MEMBER (and (admin_of "home") (in group.kind "group" "admins")))
; the effective members of plain groups and admins groups
(read EFFECTIVE_MEMBER (and (admin_of "home") (in group.kind "group" "admins")))
; the rules that pick plain groups' and admins groups' members
(read RULE (and (admin_of "home") (in group.kind "group" "admins")))
; every app, whoever it is open to
(read APP (admin_of "home"))
; every front-page widget, whoever it is open to
(read WIDGET (admin_of "home"))

;; Super admins

; old IDs and the rows they now name
(read ALIAS (super_admin))
; every column of an alias
(read ALIAS (id alias target) true)
; old paths and where they now go
(read REDIRECT (super_admin))
; every column of a redirect
(read REDIRECT (id app old new added) true)
; every change the store recorded, but those the consent step hides
(read CHANGES (super_admin))
; every column of a change
(read CHANGES (id at actor real_actor action table row column previous) true)

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
; a staff member's department as Veracross has it
(set PERSON.vc_department (system "import"))
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
; whether a person is deactivated, to compare with the export
(read PERSON (deactivated) (system "import"))
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
; every photo, to skip those already imported
(read PHOTO (system "import"))
; add a portrait from Veracross or the website, or a picture the old sheets held
(insert PHOTO (system "import"))
; every directory group the import keeps, withheld families too
(read GROUP (and (system "import") (or (in kind "family" "classroom" "crew" "grade" "band" "department") (role_group @row))))
; add a family, role group, classroom, crew, grade, band or department new in the export
(insert GROUP (and (system "import") (or (in @new.kind "family" "classroom" "crew" "grade" "band" "department") (role_group @new))))
; put a classroom under the band of its students' grades
(set GROUP.parent (and (system "import") (= @old.kind "classroom")))
; every band's rules, to find the one that takes in its grades and classrooms
(read RULE (and (system "import") (= group.kind "band")))
; make a band hold everyone in its grades and classrooms
(insert RULE (and (system "import") (= @new.group.kind "band")))
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
; every family and role group membership, to compare with the export
(read MEMBER (and (system "import") (or (= group.kind "family") (role_group group))))
; add a family or role group membership new in the export
(insert MEMBER (and (system "import") (or (= @new.group.kind "family") (role_group @new.group))))
; the status of a family or role group membership
(set MEMBER.status (and (system "import") (or (= @old.group.kind "family") (role_group @old.group))))
; remove a family or role group membership gone from the export
(delete MEMBER (and (system "import") (or (= @old.group.kind "family") (role_group @old.group))))

;; System: import, the calendar

; every event, day and day part, to compare with the school's calendars
(read GROUP (and (system "import") (calendar_kind @row)))
; add an event, day or day part
(insert GROUP (and (system "import") (calendar_kind @new)))
; a calendar group's title as its source has it
(set GROUP.title (and (system "import") (calendar_kind @old)))
; a calendar group's start as its source has it
(set GROUP.start (and (system "import") (calendar_kind @old)))
; a calendar group's end as its source has it
(set GROUP.end (and (system "import") (calendar_kind @old)))
; whether a calendar group runs all day, as its source has it
(set GROUP.all_day (and (system "import") (calendar_kind @old)))
; a calendar group's location as its source has it
(set GROUP.location (and (system "import") (calendar_kind @old)))
; a calendar group's description as its source has it
(set GROUP.description (and (system "import") (calendar_kind @old)))
; the recurring event an event is an instance of
(set GROUP.parent (and (system "import") (calendar_kind @old)))
; remove a calendar group its sources no longer state
(delete GROUP (and (system "import") (calendar_kind @old)))
; where every calendar group came from
(read GROUP_SOURCE (system "import"))
; record where a calendar group came from
(insert GROUP_SOURCE (and (system "import") (calendar_kind @new.group)))
; a source's title as it reads now
(set GROUP_SOURCE.title (and (system "import") (calendar_kind @old.group)))
; a source's start as it reads now
(set GROUP_SOURCE.start (and (system "import") (calendar_kind @old.group)))
; a source's end as it reads now
(set GROUP_SOURCE.end (and (system "import") (calendar_kind @old.group)))
; whether a source runs all day as it reads now
(set GROUP_SOURCE.all_day (and (system "import") (calendar_kind @old.group)))
; a source's location as it reads now
(set GROUP_SOURCE.location (and (system "import") (calendar_kind @old.group)))
; a source's description as it reads now
(set GROUP_SOURCE.description (and (system "import") (calendar_kind @old.group)))
; the version of the year calendar a source now comes from
(set GROUP_SOURCE.document (and (system "import") (calendar_kind @old.group)))
; the hash of what the classifier was last given for a source
(set GROUP_SOURCE.hash (and (system "import") (calendar_kind @old.group)))
; remove a source the school's calendars no longer state
(delete GROUP_SOURCE (and (system "import") (calendar_kind @old.group)))
; every category, to classify into and file under
(read GROUP (and (system "import") (= kind "category")))
; the rules saying who every calendar group is for
(read RULE (and (system "import") (calendar_kind group)))
; say who a calendar group is for
(insert RULE (and (system "import") (calendar_kind @new.group)))
; take back who a calendar group was for
(delete RULE (and (system "import") (calendar_kind @old.group)))
; the people of every calendar group, to remove them with it
(read MEMBER (and (system "import") (calendar_kind group)))
; remove someone from a calendar group being removed
(delete MEMBER (and (system "import") (calendar_kind @old.group)))
; links between documents and calendar groups, to remove them with the group
(read DOCUMENT_GROUP (and (system "import") (calendar_kind group)))
; unlink a document from a calendar group being removed
(delete DOCUMENT_GROUP (and (system "import") (calendar_kind @old.group)))
; add a version of the school's year calendar
(insert DOCUMENT (and (system "import") (= @new.kind "calendar")))

;; System: import, the sync from the old sheets, until the cutover

; every alias, to find what an earlier sync wrote
(read ALIAS (system "import"))
; an old ID and the row it now names
(insert ALIAS (system "import"))
; a person's long name as the old directory overrode it
(set PERSON.name_long_override (system "import"))
; a person's short name as the old directory overrode it
(set PERSON.name_short_override (system "import"))
; a person's sort name, built from the old directory's long name
(set PERSON.name_sort_override (system "import"))
; a person's pronouns
(set PERSON.pronouns (system "import"))
; a person's facts
(set PERSON.facts (system "import"))
; when a person's facts were last written
(set PERSON.facts_updated (system "import"))
; when a person's photo was last changed
(set PERSON.photo_updated (system "import"))
; a person's recorded name
(set PERSON.pronunciation (system "import"))
; a person's grade as the old directory overrode it
(set PERSON.grade (system "import"))
; a person's classroom as the old directory overrode it
(set PERSON.classroom (system "import"))
; a person's crew as the old directory overrode it
(set PERSON.crew (system "import"))
; a person's job title as the old directory overrode it
(set PERSON.job_title (system "import"))
; a person's phone as the old directory overrode it
(set PERSON.phone_override (system "import"))
; whether a person is hidden and when they last signed out, to compare with the old directory
(read PERSON (hidden signed_out) (system "import"))
; hide a person the old directory hid
(set PERSON.hidden (system "import"))
; when a person last signed out everywhere
(set PERSON.signed_out (system "import"))
; another address the old directory knew for a person
(insert PERSON_EMAIL (and (system "import") (= @new.source "manual")))
; where the old directory cropped a photo: its left edge
(set PHOTO.crop_left (system "import"))
; where the old directory cropped a photo: its top edge
(set PHOTO.crop_top (system "import"))
; where the old directory cropped a photo: its width
(set PHOTO.crop_width (system "import"))
; where the old directory cropped a photo: its height
(set PHOTO.crop_height (system "import"))
; a photo's place among a person's or group's photos as the old directory showed them
(set PHOTO.order (system "import"))
; a family's address as the old directory overrode it
(set GROUP.address_override (and (system "import") (= @old.kind "family")))
; a family's phone as the old directory overrode it
(set GROUP.phone_override (and (system "import") (= @old.kind "family")))
; a family photo's caption
(set GROUP.description (and (system "import") (= @old.kind "family")))
; a family's recorded name
(set GROUP.pronunciation (and (system "import") (= @old.kind "family")))
; a classroom's or grade's color
(set GROUP.color (and (system "import") (in @old.kind "classroom" "grade")))
; every plain group and admins group, to find what an earlier sync added
(read GROUP (and (system "import") (in kind "group" "admins")))
; add a tag, a band's room parents or an admins group
(insert GROUP (and (system "import") (in @new.kind "group" "admins")))
; the memberships of plain groups and admins groups
(read MEMBER (and (system "import") (in group.kind "group" "admins")))
; add someone to a tag, a band's room parents or an admins group
(insert MEMBER (and (system "import") (in @new.group.kind "group" "admins")))
; the rules of admins groups
(read RULE (and (system "import") (= group.kind "admins")))
; let super admins into an app's admins group
(insert RULE (and (system "import") (= @new.group.kind "admins")))
; every app, to find its admins group
(read APP (system "import"))
; add an app, naming its admins group
(insert APP (system "import"))
; add an app setting the old sheets held
(insert SETTING (system "import"))
; change an app setting to what the old sheets hold
(set SETTING.value (system "import"))
; every geocoded address, to skip those already held
(read GEOCODE (system "import"))
; add an address the old directory had geocoded
(insert GEOCODE (system "import"))
; add an invite list service the old Invites sheet held
(insert INVITE_SERVICE (system "import"))
; whether a service's list has a header row, as the old sheet has it
(set INVITE_SERVICE.header_row (system "import"))
; whether a service takes one row per family, as the old sheet has it
(set INVITE_SERVICE.supports_groups (system "import"))
; a service's description, as the old sheet has it
(set INVITE_SERVICE.description (system "import"))
; add a column of a service's list
(insert INVITE_TEMPLATE (system "import"))
; a column's place in its service's list, as the old sheet has it
(set INVITE_TEMPLATE.order (system "import"))
; a column's heading, as the old sheet has it
(set INVITE_TEMPLATE.column (system "import"))
; what fills a column, as the old sheet has it
(set INVITE_TEMPLATE.template (system "import"))
; every greeting, owned ones too, to find what an earlier sync added
(read GREETING (system "import"))
; add a greeting the old Invites sheet held
(insert GREETING (system "import"))
; a greeting's name, as the old sheet has it
(set GREETING.name (system "import"))
; a greeting's format, as the old sheet has it
(set GREETING.format (system "import"))
; whether a greeting addresses a family, as the old sheet has it
(set GREETING.grouped (system "import"))
; whether a greeting addresses one person, as the old sheet has it
(set GREETING.individual (system "import"))
; a Loop list's address, as the old sheet has it
(set GROUP.slug (and (system "import") (= @old.kind "group")))
; a Loop list's title, as the old sheet has it
(set GROUP.title (and (system "import") (= @old.kind "group")))
; a Loop list's description, as the old sheet has it
(set GROUP.description (and (system "import") (= @old.kind "group")))
; who sees a Loop list, as the old sheet has it
(set GROUP.visibility (and (system "import") (= @old.kind "group")))
; who may post to a Loop list, as the old sheet has it
(set GROUP.posting (and (system "import") (= @old.kind "group")))
; who may reply on a Loop list, as the old sheet has it
(set GROUP.replying (and (system "import") (= @old.kind "group")))
; the rules of plain groups, to compare a Loop list's with the old sheet's
(read RULE (and (system "import") (= group.kind "group")))
; a Loop list's rule
(insert RULE (and (system "import") (= @new.group.kind "group")))
; someone a Loop list's manager added by address alone
(insert PERSON (and (system "import") (= @new.source "guest")))
; the address of someone a Loop list's manager added
(insert PERSON_EMAIL (and (system "import") (= @new.source "guest")))
; Loop's mail, to find what an earlier sync added
(read MESSAGE (and (system "import") (= group.kind "group")))
; a post to a Loop list, and the copy it sent out
(insert MESSAGE (and (system "import") (= @new.group.kind "group")))
; Loop's deliveries, to find what an earlier sync added
(read RECIPIENT (and (system "import") (= message.group.kind "group")))
; a copy of a Loop post that went to one person
(insert RECIPIENT (and (system "import") (= @new.message.group.kind "group")))
; a copy of a Loop post recorded on someone who can't be shown, now on their guest
(delete RECIPIENT (and (system "import") (= @old.message.group.kind "group")))
; every activity, to find what an earlier sync added
(read GROUP (and (system "import") (= kind "activity")))
; add a school year, an activity or a heading of an event's activities
(insert GROUP (and (system "import") (= @new.kind "activity")))
; an activity's place in its tree, as the old sheet has it
(set GROUP.parent (and (system "import") (= @old.kind "activity")))
; an activity's title, as the old sheet has it
(set GROUP.title (and (system "import") (= @old.kind "activity")))
; an activity's description, as the old sheet has it
(set GROUP.description (and (system "import") (= @old.kind "activity")))
; an activity's stub, as the old sheet has it
(set GROUP.slug (and (system "import") (= @old.kind "activity")))
; whether an activity is open or done, as the old sheet has it
(set GROUP.status (and (system "import") (= @old.kind "activity")))
; whether an activity is hidden, as the old sheet has it
(set GROUP.visibility (and (system "import") (= @old.kind "activity")))
; when an activity starts, as the old sheet has it
(set GROUP.start (and (system "import") (= @old.kind "activity")))
; when an activity ends, as the old sheet has it
(set GROUP.end (and (system "import") (= @old.kind "activity")))
; when an activity happens, in words, as the old sheet has it
(set GROUP.timing (and (system "import") (= @old.kind "activity")))
; where an activity happens, as the old sheet has it
(set GROUP.location (and (system "import") (= @old.kind "activity")))
; how many volunteers an activity takes, as the old sheet has it
(set GROUP.capacity (and (system "import") (= @old.kind "activity")))
; how volunteers join an activity, as the old sheet has it
(set GROUP.join (and (system "import") (= @old.kind "activity")))
; who sees an activity's volunteers, as the old sheet has it
(set GROUP.members_visible (and (system "import") (= @old.kind "activity")))
; who may add activities under an activity, as the old sheet has it
(set GROUP.adding (and (system "import") (= @old.kind "activity")))
; whether an activity needs a co-chair, as the old sheet has it
(set GROUP.manager_needed (and (system "import") (= @old.kind "activity")))
; whether an activity is a priority, as the old sheet has it
(set GROUP.priority (and (system "import") (= @old.kind "activity")))
; an activity's flyer, as the old sheet has it
(set GROUP.flyer (and (system "import") (= @old.kind "activity")))
; an activity's place among its siblings, as the old sheet has it
(set GROUP.order (and (system "import") (= @old.kind "activity")))
; who added an activity, as the old sheet has it
(set GROUP.added_by (and (system "import") (= @old.kind "activity")))
; the volunteers and co-chairs of activities
(read MEMBER (and (system "import") (= group.kind "activity")))
; add a volunteer or co-chair the old sheet has
(insert MEMBER (and (system "import") (= @new.group.kind "activity")))
; add a category the old tables or sheets held
(insert GROUP (and (system "import") (= @new.kind "category")))
; a category's title, as the old tables or sheets have it
(set GROUP.title (and (system "import") (= @old.kind "category")))
; a category's description, as the old tables or sheets have it
(set GROUP.description (and (system "import") (= @old.kind "category")))
; a category's place among the others, as the old tables or sheets have it
(set GROUP.order (and (system "import") (= @old.kind "category")))
; a category's place in the tree, as the old tables or sheets have it
(set GROUP.parent (and (system "import") (= @old.kind "category")))
; whether a category is on its app's page, as the old tables or sheets have it
(set GROUP.listed (and (system "import") (= @old.kind "category")))
; whether a When category is on by default, as the old tables have it
(set GROUP.default (and (system "import") (= @old.kind "category")))
; a category's color, as the old tables have it
(set GROUP.color (and (system "import") (= @old.kind "category")))
; whether a heading is hidden, as the old sheet has it
(set GROUP.visibility (and (system "import") (= @old.kind "category")))
; how volunteers join the activities under a heading, as the old sheet has it
(set GROUP.join (and (system "import") (= @old.kind "category")))
; who sees the volunteers of the activities under a heading, as the old sheet has it
(set GROUP.members_visible (and (system "import") (= @old.kind "category")))
; who may add activities under a heading, as the old sheet has it
(set GROUP.adding (and (system "import") (= @old.kind "category")))
; every redirect, to find what an earlier sync added
(read REDIRECT (system "import"))
; add an old path the old sheets redirect
(insert REDIRECT (system "import"))
; where an old path goes, as the old sheets have it
(set REDIRECT.new (system "import"))
; every app setting of people, to find what an earlier sync added
(read PERSON_SETTING (system "import"))
; add a person's app setting the old sheets hold
(insert PERSON_SETTING (system "import"))
; a person's app setting, as the old sheets hold it
(set PERSON_SETTING.value (system "import"))
`

type policySet struct {
	read    map[string][]cond
	open    map[string]bool
	insert  map[string][]cond
	set     map[string][]cond
	delete  map[string][]cond
	clauses []Clause
}

type Clause struct {
	Section   string   `json:"section"`
	Comment   string   `json:"comment"`
	Kind      string   `json:"kind"`
	Table     string   `json:"table,omitempty"`
	Column    string   `json:"column,omitempty"`
	Columns   []string `json:"columns,omitempty"`
	Name      string   `json:"name,omitempty"`
	Params    []string `json:"params,omitempty"`
	Condition string   `json:"condition"`
	Actor     string   `json:"actor"`
	Rest      string   `json:"rest"`
	Form      string   `json:"form"`
	cond      cond
	actor     cond
	rest      cond
}

var actorHeads = map[string]bool{"admin_of": true, "super_admin": true, "system": true}

func splitActor(c *sexp) (*sexp, *sexp) {
	if actorHeads[c.head()] {
		return c, &sexp{kind: atomName, text: "true", pos: -1}
	}
	if c.head() != "and" || len(c.list) < 3 || !actorHeads[c.list[1].head()] {
		return nil, c
	}
	rest := c.list[2:]
	if len(rest) == 1 {
		return c.list[1], rest[0]
	}
	return c.list[1], &sexp{isList: true, pos: -1, list: append([]*sexp{c.list[0]}, rest...)}
}

func (cx *compiler) parts(d *Clause, c *sexp, sc *scope) error {
	actor, rest := splitActor(c)
	var err error
	if d.rest, err = cx.cond(rest, sc); err != nil {
		return err
	}
	d.Rest = rest.render(0)
	if actor == nil {
		return nil
	}
	d.Actor = actor.flat()
	d.actor, err = cx.cond(actor, sc)
	return err
}

var (
	policies    *policySet
	definitions map[string]*define
)

func init() {
	p, d, err := compilePolicies(policySource)
	if err != nil {
		panic("db: policies: " + err.Error())
	}
	policies, definitions = p, d
}

func compilePolicies(src string) (*policySet, map[string]*define, error) {
	forms, notes, err := readForms(src)
	if err != nil {
		return nil, nil, err
	}
	cx := &compiler{policy: true, defines: map[string]*define{}, used: map[string]bool{}}
	out := &policySet{read: map[string][]cond{}, open: map[string]bool{}, insert: map[string][]cond{}, set: map[string][]cond{}, delete: map[string][]cond{}, clauses: []Clause{}}
	for i, form := range forms {
		head := form.head()
		described := Clause{Section: notes[i].section, Comment: notes[i].comment, Kind: head, Form: form.render(0)}
		if head == "define" {
			if err := cx.define(form); err != nil {
				return nil, nil, err
			}
			d := cx.defines[form.list[1].list[0].text]
			described.Name, described.Params, described.Condition = form.list[1].list[0].text, d.params, d.body.render(0)
			out.clauses = append(out.clauses, described)
			continue
		}
		if head == "read" && len(form.list) == 4 {
			c, err := cx.columnGrant(form, out)
			if err != nil {
				return nil, nil, err
			}
			described.Kind, described.Table, described.Condition, described.cond = "read columns", form.list[1].text, form.list[3].render(0), c
			t, err := tableNamed(form.list[1])
			if err != nil {
				return nil, nil, err
			}
			if err := cx.parts(&described, form.list[3], &scope{table: t, name: "row"}); err != nil {
				return nil, nil, err
			}
			for _, item := range form.list[2].list {
				described.Columns = append(described.Columns, item.text)
			}
			out.clauses = append(out.clauses, described)
			continue
		}
		if len(form.list) != 3 || form.list[1].isList || form.list[1].kind != atomName {
			return nil, nil, form.errorf("a policy is (%s TABLE[.column] condition)", head)
		}
		tableName, column, hasColumn := strings.Cut(form.list[1].text, ".")
		t, err := tableNamed(&sexp{text: tableName, pos: form.list[1].pos})
		if err != nil {
			return nil, nil, err
		}
		if hasColumn {
			if _, err := columnNamed(t, column, form.list[1]); err != nil {
				return nil, nil, err
			}
		}
		var sc *scope
		var into map[string][]cond
		switch {
		case head == "read" && !hasColumn:
			sc, into = &scope{table: t, name: "row"}, out.read
		case head == "set" && hasColumn:
			sc, into = &scope{table: t, name: "new", outer: &scope{table: t, name: "old"}}, out.set
		case head == "insert" && !hasColumn:
			sc, into = &scope{table: t, name: "new"}, out.insert
		case head == "delete" && !hasColumn:
			sc, into = &scope{table: t, name: "old"}, out.delete
		default:
			return nil, nil, form.errorf("policies are define, read TABLE, read TABLE (column…), set TABLE.column, insert TABLE and delete TABLE")
		}
		c, err := cx.cond(form.list[2], sc)
		if err != nil {
			return nil, nil, err
		}
		into[form.list[1].text] = append(into[form.list[1].text], c)
		described.Table, described.Column, described.Condition, described.cond = tableName, column, form.list[2].render(0), c
		if err := cx.parts(&described, form.list[2], sc); err != nil {
			return nil, nil, err
		}
		out.clauses = append(out.clauses, described)
	}
	for _, form := range forms {
		if form.head() == "define" && !cx.used[form.list[1].list[0].text] {
			return nil, nil, form.errorf("%s is never used", form.list[1].list[0].text)
		}
	}
	for _, t := range Tables {
		for _, c := range t.Columns {
			if _, ok := out.read[t.Name+"."+c.Name]; !ok && !c.Private {
				return nil, nil, fmt.Errorf("%s.%s has no read grant", t.Name, c.Name)
			}
		}
	}
	return out, cx.defines, nil
}

func (cx *compiler) columnGrant(form *sexp, out *policySet) (cond, error) {
	if form.list[1].isList || form.list[1].kind != atomName || !form.list[2].isList || len(form.list[2].list) == 0 {
		return cond{}, form.errorf("a column grant is (read TABLE (column…) condition)")
	}
	t, err := tableNamed(form.list[1])
	if err != nil {
		return cond{}, err
	}
	c, err := cx.cond(form.list[3], &scope{table: t, name: "row"})
	if err != nil {
		return cond{}, err
	}
	open := !form.list[3].isList && form.list[3].kind == atomName && form.list[3].text == "true"
	for _, item := range form.list[2].list {
		if item.isList || item.kind != atomName {
			return cond{}, item.errorf("a column grant lists column names")
		}
		col, err := columnNamed(t, item.text, item)
		if err != nil {
			return cond{}, err
		}
		if col.Private {
			return cond{}, item.errorf("%s.%s is private, kept from everyone but the import by the consent step", t.Name, col.Name)
		}
		key := t.Name + "." + col.Name
		out.read[key] = append(out.read[key], c)
		if open {
			out.open[key] = true
		}
	}
	return c, nil
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

func (r *run) columnReadable(t *Table, row store.Row, c Column) bool {
	if c.Private {
		// The consent step strips private columns from every view but the import's.
		return true
	}
	key := t.Name + "." + c.Name + "\x00" + row["id"]
	if seen, ok := r.columns[key]; ok {
		return seen
	}
	held := holds(policies.read[t.Name+"."+c.Name], &frame{table: t, row: row, name: "row", run: r})
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
	if raw == "" || !r.columnReadable(t, row, c) {
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
