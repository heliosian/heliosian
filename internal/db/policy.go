package db

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/store"
	"heliosian/internal/trace"
)

const policySource = `
;; Definitions

; the viewer is an effective member of the group that manages @g or a group above it
(define (manages @g)
  (exists EFFECTIVE_MEMBER @e (= person @viewer)
    (exists GROUP (= managed_by @e.group) (in id (ancestors @g)))))

; @g is a plain group, neither a role group nor a mail list, that manages a group of kind k and nothing else but itself
(define (managers_for @g k)
  (and (= @g.kind "group") (not (role_group @g)) (not @g.mail)
       (exists GROUP (= managed_by @g) (= kind k))
       (not (exists GROUP (= managed_by @g) (!= kind k) (!= id @g)))))

; @g is a plain group, neither a role group nor a mail list, that manages a mail list and nothing else but itself
(define (managers_for_mail @g)
  (and (= @g.kind "group") (not (role_group @g)) (not @g.mail)
       (exists GROUP (= managed_by @g) mail)
       (not (exists GROUP (= managed_by @g) (not mail) (!= id @g)))))

; @g is the group that manages a group the viewer may see
(define (manages_visible @g)
  (exists GROUP @v (= managed_by @g) (visible @v)))

; @g holds those who said they are coming, or not, to a group the viewer may see
(define (answers_visible @g)
  (or (exists GROUP @answered (= rsvp_yes @g) (visible @answered))
      (exists GROUP @answered (= rsvp_no @g) (visible @answered))))

; the viewer may answer for @m's person in @m's answer group: themselves, someone in their family, their guest at the group answered, or anyone in a group they manage
(define (answers_for @m)
  (or (self_or_household @m.person)
      (exists MEMBER (= group @m.group.parent) (= person @m.person) (= guest_of @viewer))
      (manages @m.group)))

; the members of every family @p is in
(define (household @p)
  (select MEMBER.person
    (in group (select MEMBER.group (= person @p) (= member "yes") (= group.kind "family")))
    (= member "yes")))

; @g is one of the role groups the import keeps: students, parents, staff, adults, everyone
(define (role_group @g)
  (and (= @g.kind "group") (in @g.slug "students" "parents" "staff" "adults" "everyone")))

; @g is one of the grade Parents groups the import keeps, whose one rule is its grade's parents
(define (grade_parents @g)
  (and (= @g.kind "group")
       (in @g.slug "grade-k-parents" "grade-1-parents" "grade-2-parents" "grade-3-parents" "grade-4-parents"
           "grade-5-parents" "grade-6-parents" "grade-7-parents" "grade-8-parents")))

; @g is the waitlist a party names
(define (party_waitlist @g)
  (exists GROUP (= kind "party") (= waitlist @g)))

; every group @p has a membership row in
(define (groups_of @p)
  (select MEMBER.group (= person @p)))

; the viewer is a super admin, or in effect a member of the app's admins group
(define (admin_of app)
  (or (super_admin)
      (exists EFFECTIVE_MEMBER (in group (select APP.admins (= key app))) (= person @viewer))))

; the viewer is in effect a member of the group the Birthdays app is open to, or a Birthdays admin
(define (birthday_team)
  (or (admin_of "birthday")
      (exists EFFECTIVE_MEMBER (in group (select APP.visible_to (= key "birthday"))) (= person @viewer))))

; the viewer is in effect a member of super-admins
(define (super_admin)
  (exists EFFECTIVE_MEMBER (= group.slug "super-admins") (= person @viewer)))

; @g is not closed, and the viewer manages it or, unless it is pending, is an effective member of the group it is visible to
(define (visible @g)
  (and (!= @g.status "closed")
       (or (manages @g)
           (and (!= @g.status "pending")
                (exists EFFECTIVE_MEMBER (= group @g.visible_to) (= person @viewer))))))

; the viewer may see @g, and manages it or is an effective member of the group its members are visible to
(define (sees_members @g)
  (and (visible @g)
       (or (manages @g)
           (exists EFFECTIVE_MEMBER (= group @g.members_visible_to) (= person @viewer)))))

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

; the viewer is @p or in a family with @p
(define (self_or_household @p)
  (or (= @p @viewer) (in @p (household @viewer))))

; the viewer manages @g, or is in it and it is not hidden from all but its managers
(define (sees_mail @g)
  (or (manages @g)
      (and (not (blank @g.visible_to))
           (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer)))))

; @d, or a document it sits under, was sent to a group that takes mail
(define (mailed @d)
  (exists DOCUMENT_GROUP (in document (ancestors @d)) (= relation "sent_to") group.mail))

; @d or a document it sits under was sent to a group whose mail the viewer sees, or it is no mail and was sent to no group
(define (document_visible @d)
  (or (exists DOCUMENT_GROUP (in document (ancestors @d)) (= relation "sent_to") (sees_mail group))
      (and (not (exists DOCUMENT (in id (ancestors @d)) (= kind "mail")))
           (not (exists DOCUMENT_GROUP (in document (ancestors @d)) (= relation "sent_to"))))))

; @c is Markdown that no document but a wiki page or one of its side cards holds
(define (wiki_content @c)
  (and (= @c.mime "text/markdown")
       (not (exists DOCUMENT (= content @c) (!= kind "wiki") (!= relation "side")))))

; the viewer sent or received @m, or manages the group it went to
(define (message_visible @m)
  (or (= @m.from_person @viewer)
      (exists RECIPIENT (= message @m) (= person @viewer))
      (manages @m.group)))

;; Everyone

; people the viewer may see
(read PERSON (person_visible @row))
; every column of a person but what the form shares
(read PERSON
  (id source vc_name vc_legal_name vc_name_long name_long_override name_long
   vc_name_short name_short_override name_short vc_name_sort name_sort_override name_sort name_show
   vc_grade grade_override grade vc_classroom classroom_override classroom vc_crew crew_override crew
   vc_department department_override department vc_job_title job_title_override job_title
   phone vc_phone_visibility vc_address_visibility pronouns pronunciation facts facts_updated
   photo_updated vc_bio)
  true)
; a person's birthday, to the Birthdays team and the sync
(read PERSON (birthday) (or (birthday_team) (system "import")))
; when a person last signed out everywhere and when the import dropped them, for the server alone
(read PERSON (signed_out deactivated) false)
; whether the form shares a person's address and phone, to them and their family
(read PERSON (address_consent phone_consent) (self_or_household @row))
; a person or someone in their family overrides their long name
(set PERSON.name_long_override (self_or_household @old))
; a person or someone in their family overrides their short name
(set PERSON.name_short_override (self_or_household @old))
; a person or someone in their family overrides their sort name
(set PERSON.name_sort_override (self_or_household @old))
; the emails of people the viewer may see
(read PERSON_EMAIL (person_visible person))
; every column of an email
(read PERSON_EMAIL (id person address primary source guest) true)
; the photos of people and groups the viewer may see
(read PHOTO
  (or (and (not (blank person)) (person_visible person))
      (and (not (blank group)) (visible group))))
; every column of a photo but its original, its re-encode and its crop
(read PHOTO (id person group order crop_left crop_top crop_width crop_height thumbnail image ready) true)
; a photo's re-encode and crop, to its person and their family, or its group's managers
(read PHOTO (reencode crop)
  (or (and (not (blank person)) (self_or_household person))
      (and (not (blank group)) (manages group))))
; the viewer's own app settings
(read PERSON_SETTING (= person @viewer))
; every column of an app setting
(read PERSON_SETTING (id person app key value) true)
; the birthday years of people the viewer may see, to the Birthdays team
(read BIRTHDAY_YEAR (and (birthday_team) (person_visible person)))
; every column of a birthday year
(read BIRTHDAY_YEAR
  (id person year assigned_to assigned ask_by contacted contacted_by participation charity note recorded
   recorded_by published published_by)
  true)
; the viewer's own collections
(read COLLECTION (= person @viewer))
; every column of a collection
(read COLLECTION (id person name emoji order default token) true)
; what the viewer's own collections are made from
(read COLLECTION_GROUP (= collection.person @viewer))
; every column of what a collection is made from
(read COLLECTION_GROUP (id collection clause relation group) true)
; the viewer's own bug reports and ideas
(read REPORT (= added_by @viewer))
; every column of a bug report or idea
(read REPORT
  (id app kind summary details url page browser viewport screen language time_zone errors screenshot
   status issue handled handled_by added_by added)
  true)

; groups the viewer may see
(read GROUP (visible @row))
; the waitlist of a party the viewer may see
(read GROUP (exists GROUP @p (= waitlist @row) (= kind "party") (visible @p)))
; the group that manages a group the viewer may see
(read GROUP (manages_visible @row))
; who said they are coming, or not, to a group the viewer may see
(read GROUP (answers_visible @row))
; every column of a group but what its family's form shares
(read GROUP
  (id parent kind status slug name subtitle description color flyer pronunciation url listed
   default order visible_to members_visible_to managed_by address phone mail posting replying join adding
   eligible capacity minimum waitlist rsvp_yes rsvp_no lead_needed priority price unit parent_ticket_required
   drop_off_allowed start end all_day timing location added_by added)
  true)
; whether a family's adults all share their address and phone, to its members
(read GROUP (address_consent phone_consent) (exists MEMBER (= group @row) (= person @viewer)))
; where the calendar groups the viewer may see came from
(read GROUP_SOURCE (visible group))
; every column of where a calendar group came from
(read GROUP_SOURCE
  (id group calendar_event document name description start end all_day location marker hash)
  true)
; the rules of groups the viewer manages
(read RULE (manages group))
; every column of a rule
(read RULE (id group order exclude target person search property value replace_with within) true)
; the viewer's own, household's and guests' memberships, those in groups they manage, the managers of visible groups, and everyone where members are shown
(read MEMBER
  (or (self_or_household person)
      (= guest_of @viewer)
      (manages group)
      (manages_visible group)
      (sees_members group)))
; every column of a membership but what it cost
(read MEMBER
  (id group person member lead opened guest_of note added_by added)
  true)
; what a membership cost and its payment reference, to the member, their host and the group's managers
(read MEMBER (price purchase_id) (or (= person @viewer) (= guest_of @viewer) (manages group)))
; say someone is coming, or not, to a group the viewer may see, for whoever the viewer may answer for
(insert MEMBER (and (answers_visible @new.group) (= @new.member "yes") (answers_for @new)))
; take back someone's answer about coming to a group the viewer may see, for whoever the viewer may answer for
(delete MEMBER (and (answers_visible @old.group) (answers_for @old)))
; a person, someone in their family or their host gives up a place; the group's managers set anyone's; never in a family, which only the import changes
(set MEMBER.member
  (and (!= @old.group.kind "family")
       (or (and (or (self_or_household @old.person)
                    (= @old.guest_of @viewer))
                (= @new.member "cancelled"))
           (manages @old.group))))
; the viewer's own and household's effective memberships, the managers of visible groups, and members of groups that show them
(read EFFECTIVE_MEMBER
  (or (self_or_household person)
      (manages_visible group)
      (sees_members group)))
; every column of an effective membership
(read EFFECTIVE_MEMBER (id group person reasons) true)

; documents sent to a group whose mail the viewer sees, or no mail and sent to no group, and every document under them
(read DOCUMENT (document_visible @row))
; every column of a document
(read DOCUMENT
  (id parent kind relation order name published author url fetch link filename content_id content message
   key_points extracted slug)
  true)
; the bytes of documents and mail the viewer may see
(read CONTENT
  (or (exists DOCUMENT @d (= content @row) (document_visible @d))
      (exists MESSAGE @m (= content @row) (message_visible @m))))
; every column of a document's bytes
(read CONTENT (id hash blob mime size) true)
; links between a document and a group, where the viewer may see both
(read DOCUMENT_GROUP (and (document_visible document) (visible group)))
; every column of a link between a document and a group
(read DOCUMENT_GROUP (id document group relation) true)
; anyone but a guest starts a wiki page as its author, and may give it a slug
(insert DOCUMENT
  (and (= @new.kind "wiki") (= @new.author @viewer) (!= @viewer.source "guest") (wiki_content @new.content)))
; anyone but a guest renames a wiki page
(set DOCUMENT.name (and (= @old.kind "wiki") (!= @viewer.source "guest")))
; anyone but a guest rewrites a wiki page
(set DOCUMENT.content (and (= @old.kind "wiki") (!= @viewer.source "guest") (wiki_content @new.content)))
; anyone but a guest moves a wiki page under another, or to the top
(set DOCUMENT.parent (and (= @old.kind "wiki") (!= @viewer.source "guest")))
; anyone but a guest reorders a wiki page among its siblings
(set DOCUMENT.order (and (= @old.kind "wiki") (!= @viewer.source "guest")))
; anyone but a guest adds a side card to a wiki page
(insert DOCUMENT
  (and (= @new.relation "side") (= @new.parent.kind "wiki") (!= @viewer.source "guest") (wiki_content @new.content)))
; anyone but a guest retitles a wiki page's side card
(set DOCUMENT.name (and (= @old.relation "side") (!= @viewer.source "guest")))
; anyone but a guest rewrites a wiki page's side card
(set DOCUMENT.content (and (= @old.relation "side") (!= @viewer.source "guest") (wiki_content @new.content)))
; anyone but a guest reorders a wiki page's side cards
(set DOCUMENT.order (and (= @old.relation "side") (!= @viewer.source "guest")))
; anyone but a guest removes a wiki page's side card
(delete DOCUMENT (and (= @old.relation "side") (!= @viewer.source "guest")))
; a wiki page's author gives it a slug, changes it or takes it away
(set DOCUMENT.slug (and (= @old.kind "wiki") (not (blank @old.author)) (= @old.author @viewer)))
; a wiki page's author deletes it once it has no sub-pages
(delete DOCUMENT (and (= @old.kind "wiki") (= @old.author @viewer) (not (exists DOCUMENT (= parent @old) (= kind "wiki")))))
; the viewer's own inbox
(read INBOX (= person @viewer))
; every column of an inbox entry
(read INBOX (id person document) true)

; mail the viewer sent or received, and mail to groups they manage
(read MESSAGE (message_visible @row))
; every column of a message
(read MESSAGE
  (id direction kind group about parent from_person from_address subject header_id content created)
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
; the birthday charities, to the Birthdays team
(read CHARITY (birthday_team))
; every column of a charity
(read CHARITY (id name description url ein allowed not_allowed_reason added) true)
; apps open to a group the viewer is in
(read APP (exists EFFECTIVE_MEMBER (= group @row.visible_to) (= person @viewer)))
; every column of an app
(read APP (id key name subtitle admins visible_to order) true)
; front-page widgets open to a group the viewer is in
(read WIDGET (exists EFFECTIVE_MEMBER (= group @row.visible_to) (= person @viewer)))
; every column of a widget
(read WIDGET (id key group name icon style descriptions sidebar visible_to order) true)
; the coordinates of a family address the viewer may see; a withheld address is blank, so it matches nothing
(read GEOCODE
  (exists GROUP @f (= kind "family") (= address @row.address) (visible @f)))
; every column of a geocode
(read GEOCODE (id address lat lng) true)
; the invite list services
(read INVITE_SERVICE true)
; every column of an invite list service
(read INVITE_SERVICE (id name description header_row grouped) true)
; the invite list templates
(read INVITE_TEMPLATE true)
; every column of an invite list template
(read INVITE_TEMPLATE (id service order column template) true)
; shared greetings and the viewer's own
(read GREETING (or (blank added_by) (= added_by @viewer)))
; every column of a greeting
(read GREETING (id name format grouped individual added_by) true)

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
; name the group of those coming to an event
(set GROUP.rsvp_yes (and (admin_of "when") (= @old.kind "event")))
; name the group of those not coming to an event
(set GROUP.rsvp_no (and (admin_of "when") (= @old.kind "event")))
; where every calendar group came from
(read GROUP_SOURCE (and (admin_of "when") (in group.kind "event" "day" "day_part")))
; every event's invitees and hosts
(read MEMBER (and (admin_of "when") (= group.kind "event")))
; make a plain group to manage events
(insert GROUP (and (admin_of "when") (= @new.kind "group")))
; name the group that manages an event, or what manages an event's managers
(set GROUP.managed_by (and (admin_of "when") (or (= @old.kind "event") (managers_for @old "event"))))
; every event's managers
(read GROUP (and (admin_of "when") (managers_for @row "event")))
; who manages events
(read MEMBER (and (admin_of "when") (managers_for group "event")))
; who in effect manages events
(read EFFECTIVE_MEMBER (and (admin_of "when") (managers_for group "event")))
; make someone a host of an event
(insert MEMBER (and (admin_of "when") (managers_for @new.group "event")))
; make someone a host of an event again, or keep them from being one
(set MEMBER.member (and (admin_of "when") (managers_for @old.group "event")))
; stop someone being a host of an event
(delete MEMBER (and (admin_of "when") (managers_for @old.group "event")))
; every event's effective invitees
(read EFFECTIVE_MEMBER (and (admin_of "when") (= group.kind "event")))
; the rules that say who events and day parts are for
(read RULE (and (admin_of "when") (in group.kind "event" "day_part")))
; all mail about events
(read MESSAGE (and (admin_of "when") (= group.kind "event")))
; the bytes of all mail about events
(read CONTENT (and (admin_of "when") (exists MESSAGE (= content @row) (= group.kind "event"))))
; every delivery of mail about events
(read RECIPIENT (and (admin_of "when") (= message.group.kind "event")))

;; Team admins

; every activity
(read GROUP (and (admin_of "team") (in kind "activity")))
; open, finish, cancel or close an activity
(set GROUP.status (and (admin_of "team") (in @old.kind "activity")))
; name the group of those coming to an activity
(set GROUP.rsvp_yes (and (admin_of "team") (= @old.kind "activity")))
; name the group of those not coming to an activity
(set GROUP.rsvp_no (and (admin_of "team") (= @old.kind "activity")))
; every activity's volunteers and chairs
(read MEMBER (and (admin_of "team") (in group.kind "activity")))
; make a plain group to manage activities
(insert GROUP (and (admin_of "team") (= @new.kind "group")))
; name the group that manages an activity, or what manages an activity's managers
(set GROUP.managed_by (and (admin_of "team") (or (= @old.kind "activity") (managers_for @old "activity"))))
; every activity's managers
(read GROUP (and (admin_of "team") (managers_for @row "activity")))
; who manages activities
(read MEMBER (and (admin_of "team") (managers_for group "activity")))
; who in effect manages activities
(read EFFECTIVE_MEMBER (and (admin_of "team") (managers_for group "activity")))
; make someone a manager of an activity
(insert MEMBER (and (admin_of "team") (managers_for @new.group "activity")))
; make someone a manager of an activity again, or keep them from being one
(set MEMBER.member (and (admin_of "team") (managers_for @old.group "activity")))
; stop someone being a manager of an activity
(delete MEMBER (and (admin_of "team") (managers_for @old.group "activity")))
; every activity's effective volunteers
(read EFFECTIVE_MEMBER (and (admin_of "team") (in group.kind "activity")))
; the rules that say who activities are for
(read RULE (and (admin_of "team") (in group.kind "activity")))
; all mail about activities
(read MESSAGE (and (admin_of "team") (in group.kind "activity")))
; the bytes of all mail about activities
(read CONTENT (and (admin_of "team") (exists MESSAGE (= content @row) (in group.kind "activity"))))
; every delivery of mail about activities
(read RECIPIENT (and (admin_of "team") (in message.group.kind "activity")))

;; Celebrate admins

; every party and celebration
(read GROUP (and (admin_of "celebrate") (in kind "party" "celebration")))
; open, cancel or close a party or celebration
(set GROUP.status (and (admin_of "celebrate") (in @old.kind "party" "celebration")))
; name the group of those coming to a party
(set GROUP.rsvp_yes (and (admin_of "celebrate") (= @old.kind "party")))
; name the group of those not coming to a party
(set GROUP.rsvp_no (and (admin_of "celebrate") (= @old.kind "party")))
; every party's and celebration's ticket holders and hosts
(read MEMBER (and (admin_of "celebrate") (in group.kind "party" "celebration")))
; what every party ticket cost and its payment reference
(read MEMBER (price purchase_id) (and (admin_of "celebrate") (in group.kind "party")))
; make a plain group to host parties
(insert GROUP (and (admin_of "celebrate") (= @new.kind "group")))
; name the group that hosts a party, or what manages a party's hosts
(set GROUP.managed_by (and (admin_of "celebrate") (or (= @old.kind "party") (managers_for @old "party"))))
; every party's hosts
(read GROUP (and (admin_of "celebrate") (managers_for @row "party")))
; who hosts parties
(read MEMBER (and (admin_of "celebrate") (managers_for group "party")))
; who in effect hosts parties
(read EFFECTIVE_MEMBER (and (admin_of "celebrate") (managers_for group "party")))
; make someone a host of a party
(insert MEMBER (and (admin_of "celebrate") (managers_for @new.group "party")))
; make someone a host of a party again, or keep them from being one
(set MEMBER.member (and (admin_of "celebrate") (managers_for @old.group "party")))
; stop someone being a host of a party
(delete MEMBER (and (admin_of "celebrate") (managers_for @old.group "party")))
; every party's waitlist
(read GROUP (and (admin_of "celebrate") (party_waitlist @row)))
; who waits on every party's waitlist
(read MEMBER (and (admin_of "celebrate") (party_waitlist group)))
; every party's and celebration's effective members
(read EFFECTIVE_MEMBER (and (admin_of "celebrate") (in group.kind "party" "celebration")))
; the rules that say who parties and celebrations are for
(read RULE (and (admin_of "celebrate") (in group.kind "party" "celebration")))
; all mail about parties and celebrations
(read MESSAGE (and (admin_of "celebrate") (in group.kind "party" "celebration")))
; the bytes of all mail about parties and celebrations
(read CONTENT (and (admin_of "celebrate") (exists MESSAGE (= content @row) (in group.kind "party" "celebration"))))
; every delivery of mail about parties and celebrations
(read RECIPIENT (and (admin_of "celebrate") (in message.group.kind "party" "celebration")))

;; Loop admins

; every group that takes mail
(read GROUP (and (admin_of "loop") mail))
; open or close a group that takes mail
(set GROUP.status (and (admin_of "loop") @old.mail))
; every mail group's members
(read MEMBER (and (admin_of "loop") group.mail))
; make a plain group to manage mail groups
(insert GROUP (and (admin_of "loop") (= @new.kind "group")))
; name the group that manages a mail group, or what manages a mail group's managers
(set GROUP.managed_by (and (admin_of "loop") (or @old.mail (managers_for_mail @old))))
; every mail group's managers
(read GROUP (and (admin_of "loop") (managers_for_mail @row)))
; who manages mail groups
(read MEMBER (and (admin_of "loop") (managers_for_mail group)))
; who in effect manages mail groups
(read EFFECTIVE_MEMBER (and (admin_of "loop") (managers_for_mail group)))
; make someone a manager of a mail group
(insert MEMBER (and (admin_of "loop") (managers_for_mail @new.group)))
; make someone a manager of a mail group again, or keep them from being one
(set MEMBER.member (and (admin_of "loop") (managers_for_mail @old.group)))
; stop someone being a manager of a mail group
(delete MEMBER (and (admin_of "loop") (managers_for_mail @old.group)))
; every mail group's effective members
(read EFFECTIVE_MEMBER (and (admin_of "loop") group.mail))
; the rules that pick each mail group's members
(read RULE (and (admin_of "loop") group.mail))
; all mail to groups that take it
(read MESSAGE (and (admin_of "loop") group.mail))
; the bytes of all mail to groups that take it
(read CONTENT (and (admin_of "loop") (exists MESSAGE (= content @row) group.mail)))
; every delivery of mail to groups that take it
(read RECIPIENT (and (admin_of "loop") message.group.mail))
; every post sent to a group that takes mail, and every document under one
(read DOCUMENT (and (admin_of "loop") (mailed @row)))
; the bytes of every post sent to a group that takes mail, and of every document under one
(read CONTENT (and (admin_of "loop") (exists DOCUMENT @d (= content @row) (mailed @d))))
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

;; Wiki admins

; give any wiki page a slug, change it or take it away
(set DOCUMENT.slug (and (admin_of "wiki") (= @old.kind "wiki")))
; any wiki page with no sub-pages
(delete DOCUMENT (and (admin_of "wiki") (= @old.kind "wiki") (not (exists DOCUMENT (= parent @old) (= kind "wiki")))))

;; Super admins

; old IDs and the rows they now name
(read ALIAS (super_admin))
; every column of an alias
(read ALIAS (id alias target) true)
; old paths and where they now go
(read REDIRECT (super_admin))
; the wiki's old page addresses, so an old link still finds its page
(read REDIRECT (= app "wiki"))
; every column of a redirect
(read REDIRECT (id app old new added) true)
; every group's and person's search entry
(read SEARCH (super_admin))
; every column of a search entry
(read SEARCH (id target input summary keywords chunks object made) true)
; every document, whoever it was sent to
(read DOCUMENT (super_admin))
; every document's groups
(read DOCUMENT_GROUP (super_admin))
; every stored file
(read CONTENT (super_admin))
; every bug report and idea, to triage
(read REPORT (super_admin))
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
(set PERSON.vc_name_long (system "import"))
; a person's short name derived from Veracross's
(set PERSON.vc_name_short (system "import"))
; a person's sort name derived from Veracross's
(set PERSON.vc_name_sort (system "import"))
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
(read GROUP (and (system "import") (or (in kind "family" "classroom" "crew" "grade" "band" "department") (role_group @row) (grade_parents @row))))
; add a family, role group, grade Parents group, classroom, crew, grade, band or department new in the export
(insert GROUP (and (system "import") (or (in @new.kind "family" "classroom" "crew" "grade" "band" "department") (role_group @new) (grade_parents @new))))
; every grade Parents group's rules, to keep each its grade's parents
(read RULE (and (system "import") (grade_parents group)))
; make a grade Parents group take in its grade's parents
(insert RULE (and (system "import") (grade_parents @new.group)))
; stop a grade Parents group taking in anything but its grade's parents
(delete RULE (and (system "import") (grade_parents @old.group)))
; put a classroom under the band of its students' grades
(set GROUP.parent (and (system "import") (= @old.kind "classroom")))
; every band's rules, to compare with the grades and classrooms under it
(read RULE (and (system "import") (= group.kind "band")))
; make a band take in a grade or classroom under it
(insert RULE (and (system "import") (= @new.group.kind "band")))
; stop a band taking in a grade or classroom no longer under it
(delete RULE (and (system "import") (= @old.group.kind "band")))
; make a family manage itself
(set GROUP.managed_by (and (system "import") (= @old.kind "family") (= @new.managed_by @old.id)))
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
; whether someone is in a family or role group
(set MEMBER.member (and (system "import") (or (= @old.group.kind "family") (role_group @old.group))))
; remove a family or role group membership gone from the export
(delete MEMBER (and (system "import") (or (= @old.group.kind "family") (role_group @old.group))))

;; System: import, the calendar

; every event, day and day part, to compare with the school's calendars
(read GROUP (and (system "import") (calendar_kind @row)))
; add an event, day or day part
(insert GROUP (and (system "import") (calendar_kind @new)))
; a calendar group's name as its source has it
(set GROUP.name (and (system "import") (calendar_kind @old)))
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
; a source's name as it reads now
(set GROUP_SOURCE.name (and (system "import") (calendar_kind @old.group)))
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
; unlink a document from a calendar group being removed
(delete DOCUMENT_GROUP (and (system "import") (calendar_kind @old.group)))
; the old IDs naming every calendar group, to remove them with it
(read ALIAS (and (system "import") (exists GROUP (= id @row.target) (in kind "event" "day" "day_part"))))
; remove an old ID naming a calendar group being removed
(delete ALIAS (and (system "import") (exists GROUP (= id @old.target) (in kind "event" "day" "day_part"))))
; add a version of the school's year calendar
(insert DOCUMENT (and (system "import") (= @new.kind "calendar")))
; add a file the school or HCA shared, such as a slide deck
(insert DOCUMENT (and (system "import") (= @new.kind "file")))
; store the bytes of a shared file: a PDF, such as a version of the year calendar or a slide deck, or a page
(insert CONTENT (and (system "import") (in @new.mime "application/pdf" "text/html")))
; add a mail message the community received
(insert DOCUMENT (and (system "import") (= @new.kind "mail")))
; say which groups a mail message was sent to
(insert DOCUMENT_GROUP (and (system "import") (= @new.relation "sent_to") (= @new.document.kind "mail")))
; every document, to find the mail already uploaded
(read DOCUMENT (system "import"))
; every document's groups, to find the mail whose groups are not yet said
(read DOCUMENT_GROUP (system "import"))
; an image or a link of an email's HTML part, still to fetch or judged not worth it, for HTML extracted before extraction placed them
(insert DOCUMENT (and (system "import") (in @new.relation "image" "linked") (= @new.parent.relation "part") (blank @new.content)))
; fill an image or a link still to fetch with what a fetch run with someone's own credentials got
(set DOCUMENT.content (and (system "import") (in @old.relation "image" "linked") (blank @old.content)))
; say why fetching an image or a link stopped, or that it no longer has
(set DOCUMENT.fetch (and (system "import") (in @old.relation "image" "linked") (blank @old.content)))
; mark an image placed before images had a relation of their own
(set DOCUMENT.relation (and (system "import") (= @old.relation "linked") (= @new.relation "image")))
; store the bytes of an image, a pdf or a page such a fetch got
(insert CONTENT (and (system "import") (in @new.mime "image/png" "image/jpeg" "image/gif" "image/webp" "image/bmp" "image/x-icon" "application/pdf" "text/html; charset=utf-8")))
; start a wiki page with no author, for the wiki's own import
(insert DOCUMENT (and (system "import") (= @new.kind "wiki") (blank @new.author) (wiki_content @new.content)))
; add a side card to a wiki page
(insert DOCUMENT (and (system "import") (= @new.relation "side") (= @new.parent.kind "wiki") (wiki_content @new.content)))

;; System: import, the sync from the old sheets, until the cutover

; every membership, to compare the old sites' with
(read MEMBER (system "import"))
; every group's effective members, to compare the old sites' with
(read EFFECTIVE_MEMBER (system "import"))
; every rule, to compare the old sites' with
(read RULE (system "import"))
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
(set PERSON.grade_override (system "import"))
; a person's classroom as the old directory overrode it
(set PERSON.classroom_override (system "import"))
; a person's crew as the old directory overrode it
(set PERSON.crew_override (system "import"))
; a person's job title as the old directory overrode it
(set PERSON.job_title_override (system "import"))
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
; add a tag, a band's room parents, an admins group or a party's waitlist
(insert GROUP (and (system "import") (in @new.kind "group" "admins")))
; the memberships of plain groups and admins groups
(read MEMBER (and (system "import") (in group.kind "group" "admins")))
; add someone to a tag, a band's room parents, an admins group or a party's waitlist
(insert MEMBER (and (system "import") (in @new.group.kind "group" "admins")))
; the rules of admins groups
(read RULE (and (system "import") (= group.kind "admins")))
; let super admins into an app's admins group
(insert RULE (and (system "import") (= @new.group.kind "admins")))
; every app, to find its admins group
(read APP (system "import"))
; add an app, naming its admins group
(insert APP (system "import"))
; who sees an app, as the old Apps sheet's Audience tab has it
(set APP.visible_to (system "import"))
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
(set INVITE_SERVICE.grouped (system "import"))
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
; a Loop list's name, as the old sheet has it
(set GROUP.name (and (system "import") (= @old.kind "group")))
; a Loop list's description, as the old sheet has it
(set GROUP.description (and (system "import") (= @old.kind "group")))
; who sees a Loop list, as the old sheet has it
(set GROUP.visible_to (and (system "import") (in @old.kind "group" "admins")))
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
; every stored file, to find a Loop post's message an earlier sync stored
(read CONTENT (system "import"))
; a Loop post's message as it arrived
(insert CONTENT (and (system "import") (= @new.mime "message/rfc822")))
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
; an activity's name, as the old sheet has it
(set GROUP.name (and (system "import") (= @old.kind "activity")))
; an activity's description, as the old sheet has it
(set GROUP.description (and (system "import") (= @old.kind "activity")))
; an activity's stub, as the old sheet has it
(set GROUP.slug (and (system "import") (= @old.kind "activity")))
; whether an activity is open or done, as the old sheet has it
(set GROUP.status (and (system "import") (= @old.kind "activity")))
; whether an activity is hidden, as the old sheet has it
(set GROUP.visible_to (and (system "import") (= @old.kind "activity")))
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
(set GROUP.members_visible_to (and (system "import") (= @old.kind "activity")))
; who may add activities under an activity, as the old sheet has it
(set GROUP.adding (and (system "import") (= @old.kind "activity")))
; whether an activity needs a co-chair, as the old sheet has it
(set GROUP.lead_needed (and (system "import") (= @old.kind "activity")))
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
; a category's name, as the old tables or sheets have it
(set GROUP.name (and (system "import") (= @old.kind "category")))
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
(set GROUP.visible_to (and (system "import") (= @old.kind "category")))
; how volunteers join the activities under a heading, as the old sheet has it
(set GROUP.join (and (system "import") (= @old.kind "category")))
; who sees the volunteers of the activities under a heading, as the old sheet has it
(set GROUP.members_visible_to (and (system "import") (= @old.kind "category")))
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
; a guest's address that the old Celebrate sheet says has moved
(set PERSON_EMAIL.address (and (system "import") (= @old.source "guest")))
; every party and celebration, to find what an earlier sync added
(read GROUP (and (system "import") (in kind "party" "celebration")))
; add a celebration or a party
(insert GROUP (and (system "import") (in @new.kind "party" "celebration")))
; a party's category or celebration, as the old sheet has it
(set GROUP.parent (and (system "import") (in @old.kind "party" "celebration")))
; a party's or celebration's address on the site, as the old sheet has it
(set GROUP.slug (and (system "import") (in @old.kind "party" "celebration")))
; a party's or celebration's name, as the old sheet has it
(set GROUP.name (and (system "import") (in @old.kind "party" "celebration")))
; a party's or celebration's subtitle, as the old sheet has it
(set GROUP.subtitle (and (system "import") (in @old.kind "party" "celebration")))
; a party's or celebration's description, as the old sheet has it
(set GROUP.description (and (system "import") (in @old.kind "party" "celebration")))
; where a party or celebration is, in words, as the old sheet has it
(set GROUP.location (and (system "import") (in @old.kind "party" "celebration")))
; a party's or celebration's street address, as the old sheet has it
(set GROUP.address_override (and (system "import") (in @old.kind "party" "celebration")))
; when a party or celebration starts, as the old sheet has it
(set GROUP.start (and (system "import") (in @old.kind "party" "celebration")))
; when a party or celebration ends, as the old sheet has it
(set GROUP.end (and (system "import") (in @old.kind "party" "celebration")))
; whether a party or celebration runs all day, as the old sheet has it
(set GROUP.all_day (and (system "import") (in @old.kind "party" "celebration")))
; whether a party is pending, as the old sheet has it
(set GROUP.status (and (system "import") (in @old.kind "party" "celebration")))
; whether a party is hidden, as the old sheet has it
(set GROUP.visible_to (and (system "import") (in @old.kind "party" "celebration")))
; whether a party's tickets are on sale, as the old sheet has it
(set GROUP.join (and (system "import") (= @old.kind "party")))
; what a party's ticket is for, as the old sheet has it
(set GROUP.unit (and (system "import") (= @old.kind "party")))
; a party's ticket price, as the old sheet has it
(set GROUP.price (and (system "import") (= @old.kind "party")))
; how many tickets a party has, as the old sheet has it
(set GROUP.capacity (and (system "import") (= @old.kind "party")))
; how many tickets a party needs to go ahead, as the old sheet has it
(set GROUP.minimum (and (system "import") (= @old.kind "party")))
; a party's flyer, as the old sheet has it
(set GROUP.flyer (and (system "import") (= @old.kind "party")))
; a party's waitlist, set when the old sheet has it take one
(set GROUP.waitlist (and (system "import") (= @old.kind "party")))
; who may come to a party, as the old sheet has it
(set GROUP.eligible (and (system "import") (= @old.kind "party")))
; whether kids may be dropped off at a party, as the old sheet has it
(set GROUP.drop_off_allowed (and (system "import") (= @old.kind "party")))
; whether a parent who stays needs a ticket, as the old sheet has it
(set GROUP.parent_ticket_required (and (system "import") (= @old.kind "party")))
; who posted a party, as the old sheet has it
(set GROUP.added_by (and (system "import") (= @old.kind "party")))
; when a party was posted, as the old sheet has it
(set GROUP.added (and (system "import") (= @old.kind "party")))
; the hosts and ticket holders of every party
(read MEMBER (and (system "import") (= group.kind "party")))
; add a host or a ticket the old sheet has
(insert MEMBER (and (system "import") (= @new.group.kind "party")))
; what a ticket cost, to compare with the old sheet
(read MEMBER (price) (system "import"))
; remove someone waiting for a party the old sheet no longer has
(delete MEMBER (and (system "import") (party_waitlist @old.group)))
; move a membership to the guest a same-named guest is merged into
(set MEMBER.person (and (system "import") (= @old.person.source "guest")))
; move a purchase to the guest a same-named guest is merged into
(set MEMBER.guest_of (and (system "import") (= @old.guest_of.source "guest")))
; remove a merged guest's membership that the guest it joins already holds
(delete MEMBER (and (system "import") (= @old.person.source "guest")))
; the group that runs a tag, list, activity or party, as the old sheets have it
(set GROUP.managed_by (and (system "import") (in @old.kind "group" "activity" "party" "event")))
; whether someone is in a tag, list, admins group, activity, party or event, as the old sheets have it
(set MEMBER.member (and (system "import") (in @old.group.kind "group" "admins" "activity" "party" "event")))
; whether a volunteer co-chairs an activity, as the old sheet has it
(set MEMBER.lead (and (system "import") (= @old.group.kind "activity")))
; move a merged guest's address to the guest it joins
(set PERSON_EMAIL.person (and (system "import") (= @old.source "guest")))
; whether a moved guest address is the guest's main one
(set PERSON_EMAIL.primary (and (system "import") (= @old.source "guest")))
; point an old ID at the row a merged guest's membership collapsed into
(set ALIAS.target (system "import"))
; remove a merged guest once nothing names it
(delete PERSON (and (system "import") (= @old.source "guest")))
; remove an old ID naming a row the sync removed
(delete ALIAS (system "import"))
; a hand-added event's status, as the old calendar has it
(set GROUP.status (and (system "import") (calendar_kind @old)))
; whether a hand-added event is listed, as the old calendar has it
(set GROUP.listed (and (system "import") (calendar_kind @old)))
; who sees an event, as the old calendar has it
(set GROUP.visible_to (and (system "import") (calendar_kind @old)))
; who sees an event's guests, as the old calendar has it
(set GROUP.members_visible_to (and (system "import") (calendar_kind @old)))
; a hand-added event's link, as the old calendar has it
(set GROUP.url (and (system "import") (calendar_kind @old)))
; an event's friendly address, as the old calendar has it
(set GROUP.slug (and (system "import") (calendar_kind @old)))
; who may add guests to an event, as the old calendar has it
(set GROUP.adding (and (system "import") (calendar_kind @old)))
; an event's flyer, as the old calendar has it
(set GROUP.flyer (and (system "import") (calendar_kind @old)))
; who added a hand-added event, as the old calendar has it
(set GROUP.added_by (and (system "import") (calendar_kind @old)))
; when a hand-added event was added, as the old calendar has it
(set GROUP.added (and (system "import") (calendar_kind @old)))
; who said they are coming to an event, party or activity, as the old calendar has it
(set GROUP.rsvp_yes (and (system "import") (in @old.kind "event" "party" "activity")))
; who said they are not coming to an event, party or activity, as the old calendar has it
(set GROUP.rsvp_no (and (system "import") (in @old.kind "event" "party" "activity")))
; a guest on an event's list or someone kept off it, as the old calendar has it
(insert MEMBER (and (system "import") (calendar_kind @new.group)))
; when someone first opened their invitation, as the old calendar has it
(set MEMBER.opened (and (system "import") (in @old.group.kind "event" "party" "activity")))
; every invitation, to compare with the old calendar's
(read MESSAGE (and (system "import") (= kind "invitation")))
; an invitation the old calendar sent or holds unsent
(insert MESSAGE (and (system "import") (= @new.kind "invitation")))
; every copy of an invitation, to compare with the old calendar's
(read RECIPIENT (and (system "import") (= message.kind "invitation")))
; a copy of an invitation the old calendar sent or holds unsent
(insert RECIPIENT (and (system "import") (= @new.message.kind "invitation")))
; every collection, to compare with the old calendar's saved calendars and hidden events
(read COLLECTION (system "import"))
; a saved calendar or set of hidden events from the old calendar
(insert COLLECTION (system "import"))
; a saved calendar's name, as the old calendar has it
(set COLLECTION.name (system "import"))
; a saved calendar's mark, as the old calendar has it
(set COLLECTION.emoji (system "import"))
; a saved calendar's place, as the old calendar has it
(set COLLECTION.order (system "import"))
; whether a saved calendar is the one When opens to, as the old calendar has it
(set COLLECTION.default (system "import"))
; every collection's groups, to compare with the old calendar's filters
(read COLLECTION_GROUP (system "import"))
; a saved calendar's filter or a hidden event from the old calendar
(insert COLLECTION_GROUP (system "import"))
; every front-page widget, to find what an earlier sync added
(read WIDGET (system "import"))
; add a front-page widget the old Apps sheet has
(insert WIDGET (system "import"))
; who sees a widget, as the old Apps sheet's Audience tab has it
(set WIDGET.visible_to (system "import"))
; a widget's place on the page, as the old Apps sheet has it
(set WIDGET.order (system "import"))
; a section's group of links, as the old Apps sheet has it
(set WIDGET.group (system "import"))
; a section's heading, as the old Apps sheet has it
(set WIDGET.name (system "import"))
; a section's mark, as the old Apps sheet has it
(set WIDGET.icon (system "import"))
; how a section is laid out, as the old Apps sheet has it
(set WIDGET.style (system "import"))
; whether a section shows descriptions, as the old Apps sheet has it
(set WIDGET.descriptions (system "import"))
; whether a widget shows in the rail, as the old Apps sheet has it
(set WIDGET.sidebar (system "import"))
; what an app is called, as the old Apps sheet has it
(set APP.name (system "import"))
; the line under an app's name, as the old Apps sheet has it
(set APP.subtitle (system "import"))
; an app's place in the switch, as the old Apps sheet has it
(set APP.order (system "import"))
; where a front-page link goes, as the old Apps sheet has it
(set GROUP.url (and (system "import") (= @old.kind "group")))
; the section a front-page link sits in, as the old Apps sheet has it
(set GROUP.parent (and (system "import") (= @old.kind "group")))
; a front-page link's place in its section, as the old Apps sheet has it
(set GROUP.order (and (system "import") (= @old.kind "group")))
; a staff member's birthday, as the old Birthdays sheet has it
(set PERSON.birthday (system "import"))
; every birthday year, to find what an earlier sync added
(read BIRTHDAY_YEAR (system "import"))
; add a staff member's birthday year the old Birthdays sheet has
(insert BIRTHDAY_YEAR (system "import"))
; who has a birthday, as the old sheet has it
(set BIRTHDAY_YEAR.assigned_to (system "import"))
; when a birthday was assigned, as the old sheet has it
(set BIRTHDAY_YEAR.assigned (system "import"))
; the day to ask the last invite named, as the old sheet has it
(set BIRTHDAY_YEAR.ask_by (system "import"))
; when the staff member was asked, as the old sheet has it
(set BIRTHDAY_YEAR.contacted (system "import"))
; who asked the staff member, as the old sheet has it
(set BIRTHDAY_YEAR.contacted_by (system "import"))
; how the staff member takes part, as the old sheet has it
(set BIRTHDAY_YEAR.participation (system "import"))
; the charity chosen, as the old sheet has it
(set BIRTHDAY_YEAR.charity (system "import"))
; the staff member's words about their charity, as the old sheet has it
(set BIRTHDAY_YEAR.note (system "import"))
; when the charity was recorded, as the old sheet has it
(set BIRTHDAY_YEAR.recorded (system "import"))
; who recorded the charity, as the old sheet has it
(set BIRTHDAY_YEAR.recorded_by (system "import"))
; when the newsletter copy took it, as the old sheet has it
(set BIRTHDAY_YEAR.published (system "import"))
; who copied it for the newsletter, as the old sheet has it
(set BIRTHDAY_YEAR.published_by (system "import"))
; every charity, to find what an earlier sync added
(read CHARITY (system "import"))
; add a charity the old Birthdays sheet has
(insert CHARITY (system "import"))
; a charity's name, as the old sheet has it
(set CHARITY.name (system "import"))
; a charity's sentence for the newsletter, as the old sheet has it
(set CHARITY.description (system "import"))
; where a donation to a charity is made, as the old sheet has it
(set CHARITY.url (system "import"))
; a charity's EIN, as the old sheet has it
(set CHARITY.ein (system "import"))
; whether a charity may be chosen, as the old sheet has it
(set CHARITY.allowed (system "import"))
; why a charity may not be chosen, as the old sheet has it
(set CHARITY.not_allowed_reason (system "import"))
; when a charity was added, as the old sheet has it
(set CHARITY.added (system "import"))
; every calendar invite, to compare with the old Birthdays sheet's
(read MESSAGE (and (system "import") (= kind "calendar")))
; a calendar invite the old Birthdays sheet sent an assignee
(insert MESSAGE (and (system "import") (= @new.kind "calendar")))
; every copy of a calendar invite, to compare with the old Birthdays sheet's
(read RECIPIENT (and (system "import") (= message.kind "calendar")))
; the copy of a calendar invite that went to an assignee
(insert RECIPIENT (and (system "import") (= @new.message.kind "calendar")))
; every bug report and idea, to find what an earlier sync added
(read REPORT (system "import"))
; add a bug report or idea the old Feedback sheet has
(insert REPORT (system "import"))
; where a report's triage stands, as the old sheet has it
(set REPORT.status (system "import"))
; the GitHub issue a report became, as the old sheet has it
(set REPORT.issue (system "import"))
; when a report was triaged, as the old sheet has it
(set REPORT.handled (system "import"))
; who triaged a report, as the old sheet has it
(set REPORT.handled_by (system "import"))
`

type grant struct {
	cond    cond
	comment string
	gated   bool
}

type policySet struct {
	read    map[string][]grant
	open    map[string]bool
	insert  map[string][]grant
	set     map[string][]grant
	delete  map[string][]grant
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

var actorHeads = map[string]bool{"admin_of": true, "super_admin": true, "birthday_team": true, "system": true}

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
	out := &policySet{read: map[string][]grant{}, open: map[string]bool{}, insert: map[string][]grant{}, set: map[string][]grant{}, delete: map[string][]grant{}, clauses: []Clause{}}
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
			c, err := cx.columnGrant(form, notes[i].comment, out)
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
		var into map[string][]grant
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
		actor, _ := splitActor(form.list[2])
		into[form.list[1].text] = append(into[form.list[1].text], grant{cond: c, comment: notes[i].comment, gated: head == "read" && actor != nil})
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
	for _, grants := range out.read {
		slices.SortStableFunc(grants, func(a, b grant) int {
			switch {
			case a.gated == b.gated:
				return 0
			case a.gated:
				return -1
			}
			return 1
		})
	}
	return out, cx.defines, nil
}

func (cx *compiler) columnGrant(form *sexp, comment string, out *policySet) (cond, error) {
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
		out.read[key] = append(out.read[key], grant{cond: c, comment: comment})
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

func holds(grants []grant, f *frame, tally *trace.Span) bool {
	for _, g := range grants {
		clause := tally.Tally(g.comment)
		start := time.Now()
		held := g.cond.eval(f)
		clause.Add(time.Since(start))
		if held {
			clause.Inc("held")
			return true
		}
	}
	return false
}

func (r *run) check(name string, grants []grant, f *frame) bool {
	tally := r.policy.Tally(name)
	start := time.Now()
	held := holds(grants, f, tally)
	took := time.Since(start)
	tally.Add(took)
	r.policy.Add(took)
	if held {
		tally.Inc("held")
	}
	return held
}

func (r *run) readable(t *Table, row store.Row) bool {
	key := t.Name + "\x00" + row["id"]
	if seen, ok := r.rows[key]; ok {
		r.policy.Tally(t.Name).Inc("cached")
		return seen
	}
	ok := r.check(t.Name, policies.read[t.Name], &frame{table: t, row: row, name: "row", run: r})
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
	held := r.check(t.Name+" columns", policies.read[t.Name+"."+c.Name], &frame{table: t, row: row, name: "row", run: r})
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
	if c.Kind == Ref && !r.idReadable(raw) {
		return ""
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
		if !holds(policies.insert[t.Name], &frame{table: t, row: c.New, name: "new", run: r}, nil) {
			return access.Forbidden("you may not add to %s", t.Name)
		}
	case c.New == nil:
		if !holds(policies.delete[t.Name], old, nil) {
			return access.Forbidden("you may not remove from %s", t.Name)
		}
	default:
		changed := &frame{table: t, row: c.New, name: "new", outer: old, run: r}
		for _, col := range t.Columns {
			if col.Generated || strings.TrimSpace(c.Old[col.Name]) == strings.TrimSpace(c.New[col.Name]) {
				continue
			}
			if !holds(policies.set[t.Name+"."+col.Name], changed, nil) {
				return access.Forbidden("you may not change %s.%s", t.Name, col.Name)
			}
		}
	}
	return nil
}
