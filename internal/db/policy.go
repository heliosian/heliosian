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

const PolicySource = `
;; Definitions

; the viewer is an effective member of the group that manages @g or a group above it
(define (manages @g)
  (exists GROUP (in id (ancestors @g))
    (in managed_by (select EFFECTIVE_MEMBER.group (= person @viewer)))))

; @g is a plain group, not a role group, that manages a group of kind k and nothing else but itself
(define (managers_for @g k)
  (and (= @g.kind "group") (not (role_group @g))
       (exists GROUP (= managed_by @g) (= kind k))
       (not (exists GROUP (= managed_by @g) (!= kind k) (!= id @g)))))

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

; @g is one of the role groups the Veracross import keeps: students, parents, staff, adults, everyone
(define (role_group @g)
  (and (= @g.kind "group") (in @g.slug "students" "parents" "staff" "adults" "everyone")))

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

; the viewer is @p or in a family with @p
(define (self_or_household @p)
  (or (= @p @viewer) (in @p (household @viewer))))

; the viewer may change @ph: a picture of themselves or someone in their family, or of a family they manage
(define (edits_photo @ph)
  (or (and (not (blank @ph.person)) (self_or_household @ph.person))
      (and (not (blank @ph.group)) (= @ph.group.kind "family") (manages @ph.group))))

; @g is a plain group someone may make and run: under nothing, linking nowhere, and seen by its managers alone
(define (own_group @g)
  (and (= @g.kind "group") (blank @g.parent) (blank @g.url) (blank @g.visible_to)))

; the row's reference column c names a group other than the row itself, of which the row is not a managers, waitlist, answer or unsubscribed group
(define (counted c)
  (and (!= c id) (!= c.managed_by id) (!= c.waitlist id) (!= c.rsvp_yes id) (!= c.rsvp_no id) (!= c.unsubscribed id)))

; @g exists only to serve one group: one row names it, as its managers, waitlist, answer, unsubscribed or audience group, and nothing else names it
(define (implementation_only @g)
  (and (= (tally @g
                 (select GROUP.managed_by @r (counted managed_by))
                 (select GROUP.waitlist @r (counted waitlist))
                 (select GROUP.rsvp_yes @r (counted rsvp_yes))
                 (select GROUP.rsvp_no @r (counted rsvp_no))
                 (select GROUP.unsubscribed @r (counted unsubscribed))
                 (select GROUP.visible_to @r (counted visible_to))
                 (select GROUP.members_visible_to @r (counted members_visible_to))
                 (select GROUP.eligible @r (counted eligible)))
          1)
       (= (tally @g
                 (select GROUP.managed_by @r (counted managed_by))
                 (select GROUP.waitlist @r (counted waitlist))
                 (select GROUP.rsvp_yes @r (counted rsvp_yes))
                 (select GROUP.rsvp_no @r (counted rsvp_no))
                 (select GROUP.unsubscribed @r (counted unsubscribed))
                 (select GROUP.visible_to @r (counted visible_to))
                 (select GROUP.members_visible_to @r (counted members_visible_to))
                 (select GROUP.eligible @r (counted eligible))
                 (select GROUP.parent @r (counted parent))
                 (select RULE.target)
                 (select RULE.within)
                 (select APP.admins)
                 (select APP.visible_to)
                 (select WIDGET.group)
                 (select WIDGET.visible_to)
                 (select COLLECTION_GROUP.group)
                 (select PERSON.vc_classroom)
                 (select PERSON.classroom_override)
                 (select PERSON.vc_crew)
                 (select PERSON.crew_override)
                 (select PERSON.vc_department)
                 (select PERSON.department_override))
          1)))

; @g belongs in the rail's lists: open, not a day, a part of one or a category, and not there only to serve another group
(define (sidebar_group @g)
  (and (!= @g.status "closed") (not (in @g.kind "day" "day_part" "category")) (not (implementation_only @g))))

; the viewer manages @g, or is in it and it is not hidden from all but its managers
(define (sees_mail @g)
  (or (manages @g)
      (and (not (blank @g.visible_to))
           (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer)))))

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

;; Consent

; a person shows when they are a guest, or consented to the directory and are not hidden; a row naming one who doesn't show doesn't show either
(show PERSON (or (= source "guest") (and (= consent "listed") (not hidden))))
; a family shows when it consented to the directory; a row naming one that doesn't show doesn't show either
(show GROUP (or (!= kind "family") (= consent "listed")))

;; Import mode

; a super admin in import mode sees every row, shown or not
(reveal (and (super_admin) (mode "import")))

;; Everyone

; people the viewer may see
(read PERSON (person_visible @row))
; every column of a person but what the form shares
(read PERSON
  (id source vc_name vc_legal_name vc_name_long name_long_override name_long
   vc_name_short name_short_override name_short vc_name_sort name_sort_override name_sort name_show slug
   vc_grade grade_override grade vc_classroom classroom_override classroom vc_crew crew_override crew
   vc_department department_override department vc_job_title job_title_override job_title
   phone vc_phone_visibility vc_address_visibility pronouns pronunciation facts facts_updated
   photo_updated vc_bio)
  true)
; a person's birthday, to the Birthdays team
(read PERSON (birthday) (birthday_team))
; when a person last signed out everywhere and when the Veracross import dropped them, for the server and super admins
(read PERSON (signed_out deactivated) false)
; whether the form shares a person's address and phone, to them and their family
(read PERSON (address_consent phone_consent) (self_or_household @row))
; a person or someone in their family overrides their long name
(set PERSON.name_long_override (self_or_household @old))
; a person or someone in their family overrides their short name
(set PERSON.name_short_override (self_or_household @old))
; a person or someone in their family overrides their sort name
(set PERSON.name_sort_override (self_or_household @old))
; a person or someone in their family writes their pronouns
(set PERSON.pronouns (self_or_household @old))
; a person or someone in their family records how their name is said
(set PERSON.pronunciation (self_or_household @old))
; a person or someone in their family writes their about-me words
(set PERSON.facts (self_or_household @old))
; a person or someone in their family dates their about-me words
(set PERSON.facts_updated (self_or_household @old))
; a person or someone in their family dates their picture
(set PERSON.photo_updated (self_or_household @old))
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
; add a picture of oneself, someone in one's family, or a family one manages
(insert PHOTO (edits_photo @new))
; move such a picture among its person's or family's
(set PHOTO.order (edits_photo @old))
; crop such a picture: the box's left edge
(set PHOTO.crop_left (edits_photo @old))
; crop such a picture: the box's top edge
(set PHOTO.crop_top (edits_photo @old))
; crop such a picture: the box's width
(set PHOTO.crop_width (edits_photo @old))
; crop such a picture: the box's height
(set PHOTO.crop_height (edits_photo @old))
; remove such a picture
(delete PHOTO (edits_photo @old))
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
   default order visible_to members_visible_to managed_by address phone posting replying unsubscribed join adding
   eligible capacity minimum waitlist rsvp_yes rsvp_no lead_needed priority price unit parent_ticket_required
   drop_off_allowed start end all_day timing location added_by added)
  true)
; whether a family's adults all share their address and phone, to its members
(read GROUP (address_consent phone_consent) (exists MEMBER (= group @row) (= person @viewer)))
; a family's managers override its address
(set GROUP.address_override (and (= @old.kind "family") (manages @old)))
; a family's managers override its phone
(set GROUP.phone_override (and (= @old.kind "family") (manages @old)))
; a family's managers write the words under its photo
(set GROUP.description (and (= @old.kind "family") (manages @old)))
; a family's managers record how its name is said
(set GROUP.pronunciation (and (= @old.kind "family") (manages @old)))
; anyone but a guest makes a plain group of their own, a tag, run by a group they are in or, until they name one, by nobody
(insert GROUP
  (and (own_group @new) (= @new.status "open") (= @new.added_by @viewer) (!= @viewer.source "guest")
       (or (blank @new.managed_by) (exists EFFECTIVE_MEMBER (= group @new.managed_by) (= person @viewer)))))
; whoever made a plain group nobody runs makes it run itself, as a tag's managers group does
(set GROUP.managed_by (and (own_group @old) (= @old.added_by @viewer) (blank @old.managed_by) (= @new.managed_by @old.id)))
; whoever made a plain group that runs itself is the first in it
(insert MEMBER
  (and (own_group @new.group) (= @new.group.added_by @viewer) (= @new.group.managed_by @new.group)
       (= @new.person @viewer) (= @new.member "yes") (not (exists MEMBER (= group @new.group)))))
; a plain group's managers rename it
(set GROUP.name (and (own_group @old) (manages @old)))
; a plain group's managers close it, or open it again
(set GROUP.status (and (own_group @old) (manages @old) (in @new.status "open" "closed")))
; a plain group's managers put someone in it: tag them, or share the tag through its managers group
(insert MEMBER (and (own_group @new.group) (manages @new.group) (= @new.member "yes")))
; a plain group's managers take someone out of it, themselves included
(delete MEMBER (and (own_group @old.group) (manages @old.group)))
; an admins group's managers put someone in it
(insert MEMBER (and (= @new.group.kind "admins") (manages @new.group) (= @new.member "yes")))
; an admins group's managers take someone out of it, leaving someone in one that manages itself
(delete MEMBER
  (and (= @old.group.kind "admins") (manages @old.group)
       (or (!= @old.group.managed_by @old.group) (exists MEMBER (= group @old.group) (!= id @old.id)))))
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
; a person, someone in their family or their host gives up a place; the group's managers set anyone's; never in a family, which only super admins change
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
   key_points extracted slug hidden source terminal)
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
; a wiki page's author hides it from the wiki's lists of pages, or shows it again
(set DOCUMENT.hidden (and (= @old.kind "wiki") (not (blank @old.author)) (= @old.author @viewer)))
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
  (id direction kind group about parent from_person from_address subject header_id content created state detail)
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
; anyone but a guest adds a greeting of their own
(insert GREETING (and (= @new.added_by @viewer) (!= @viewer.source "guest")))
; a greeting's owner renames it
(set GREETING.name (= @old.added_by @viewer))
; a greeting's owner rewrites it
(set GREETING.format (= @old.added_by @viewer))
; a greeting's owner says whether it addresses a family
(set GREETING.grouped (= @old.added_by @viewer))
; a greeting's owner says whether it addresses one person
(set GREETING.individual (= @old.added_by @viewer))
; a greeting's owner removes it
(delete GREETING (= @old.added_by @viewer))

;; Who? admins

; every person, deactivated too
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
; every person's emails, deactivated too
(read PERSON_EMAIL (admin_of "who"))
; every picture's re-encode and crop, to crop it
(read PHOTO (reencode crop) (admin_of "who"))
; override anyone's grade
(set PERSON.grade_override (admin_of "who"))
; override anyone's classroom
(set PERSON.classroom_override (admin_of "who"))
; override anyone's crew
(set PERSON.crew_override (admin_of "who"))
; override anyone's department
(set PERSON.department_override (admin_of "who"))
; override anyone's job title
(set PERSON.job_title_override (admin_of "who"))
; override anyone's phone
(set PERSON.phone_override (admin_of "who"))
; write anyone's pronouns
(set PERSON.pronouns (admin_of "who"))
; record how anyone's name is said
(set PERSON.pronunciation (admin_of "who"))
; write anyone's about-me words
(set PERSON.facts (admin_of "who"))
; date anyone's about-me words
(set PERSON.facts_updated (admin_of "who"))
; date anyone's picture
(set PERSON.photo_updated (admin_of "who"))
; take anyone out of every view and sign-in; putting them back is for super admins in import mode
(set PERSON.hidden (admin_of "who"))
; make someone a band's room parent
(insert MEMBER (and (admin_of "who") (= @new.group.kind "group") (= @new.group.parent.kind "band") (= @new.member "yes")))
; stop someone being a band's room parent
(delete MEMBER (and (admin_of "who") (= @old.group.kind "group") (= @old.group.parent.kind "band")))
; override any family's address
(set GROUP.address_override (and (admin_of "who") (= @old.kind "family")))
; override any family's phone
(set GROUP.phone_override (and (admin_of "who") (= @old.kind "family")))
; write the words under any family's photo
(set GROUP.description (and (admin_of "who") (= @old.kind "family")))
; record how any family's name is said
(set GROUP.pronunciation (and (admin_of "who") (= @old.kind "family")))
; a classroom's or grade's color
(set GROUP.color (and (admin_of "who") (in @old.kind "classroom" "grade")))
; a classroom's or grade's place among the others
(set GROUP.order (and (admin_of "who") (in @old.kind "classroom" "grade")))
; add a picture of anyone, any family, or a classroom's or grade's tile
(insert PHOTO (and (admin_of "who") (or (not (blank @new.person)) (in @new.group.kind "family" "classroom" "grade"))))
; move any such picture among its person's or group's
(set PHOTO.order (and (admin_of "who") (or (not (blank @old.person)) (in @old.group.kind "family" "classroom" "grade"))))
; crop any such picture: the box's left edge
(set PHOTO.crop_left (and (admin_of "who") (or (not (blank @old.person)) (in @old.group.kind "family" "classroom" "grade"))))
; crop any such picture: the box's top edge
(set PHOTO.crop_top (and (admin_of "who") (or (not (blank @old.person)) (in @old.group.kind "family" "classroom" "grade"))))
; crop any such picture: the box's width
(set PHOTO.crop_width (and (admin_of "who") (or (not (blank @old.person)) (in @old.group.kind "family" "classroom" "grade"))))
; crop any such picture: the box's height
(set PHOTO.crop_height (and (admin_of "who") (or (not (blank @old.person)) (in @old.group.kind "family" "classroom" "grade"))))
; remove any such picture
(delete PHOTO (and (admin_of "who") (or (not (blank @old.person)) (in @old.group.kind "family" "classroom" "grade"))))
; the directory's site-wide settings: how old a picture or about-me words may get, the staff color, the privacy links
(set SETTING.value
  (and (admin_of "who") (= @old.app "platform")
       (in @old.key "Facts Stale Years" "Family Photo Stale Years" "Photo Stale Years" "Staff Color"
           "Helios Who Opt-In URL" "Veracross Preferences URL")))

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

;; Mail lists

; the viewer manages @g or is a super admin, unless it is a family, whose members only super admins change
(define (runs_list @g)
  (and (!= @g.kind "family") (or (manages @g) (super_admin))))

; anyone but a guest makes a mail list, a plain group under nothing and linking nowhere, run by a group they are in, seen by its managers or by everyone
(insert GROUP
  (and (= @new.kind "group") (blank @new.parent) (blank @new.url)
       (= @new.status "open") (not (blank @new.slug)) (= @new.added_by @viewer)
       (!= @viewer.source "guest")
       (exists EFFECTIVE_MEMBER (= group @new.managed_by) (= person @viewer))
       (or (blank @new.visible_to) (= @new.visible_to.slug "everyone"))
       (or (blank @new.members_visible_to) (= @new.members_visible_to.slug "everyone"))))
; a mail list's managers rename it
(set GROUP.name (runs_list @old))
; a mail list's managers describe it
(set GROUP.description (runs_list @old))
; a mail list's managers say who may post to it
(set GROUP.posting (runs_list @old))
; a mail list's managers say who may reply to its posts
(set GROUP.replying (runs_list @old))
; a mail list's managers show it to its managers alone, its own members, or everyone
(set GROUP.visible_to
  (and (runs_list @old)
       (or (blank @new.visible_to) (= @new.visible_to @old.id) (= @new.visible_to.slug "everyone"))))
; a mail list's managers show who is on it to its managers alone, its own members, or everyone
(set GROUP.members_visible_to
  (and (runs_list @old)
       (or (blank @new.members_visible_to) (= @new.members_visible_to @old.id)
           (= @new.members_visible_to.slug "everyone"))))
; a mail list's managers close it, or open it again
(set GROUP.status (and (runs_list @old) (in @new.status "open" "closed")))
; a mail list's managers give it a rule naming only groups and people whose members they may see
(insert RULE
  (and (runs_list @new.group)
       (or (blank @new.target) (sees_members @new.target))
       (or (blank @new.within) (sees_members @new.within))
       (or (blank @new.person) (person_visible @new.person))))
; a mail list's managers reorder its rules
(set RULE.order (runs_list @old.group))
; a mail list's managers take a rule away
(delete RULE (runs_list @old.group))
; a mail list's managers put someone on it by hand, or keep someone off it; an admins group's members follow its own rules
(insert MEMBER (and (runs_list @new.group) (!= @new.group.kind "admins") (in @new.member "yes" "excluded")))
; a mail list's managers turn a hand addition into a keeping off, or back
(set MEMBER.member (and (runs_list @old.group) (!= @old.group.kind "admins") (in @new.member "yes" "excluded")))
; a mail list's managers take away a hand addition or a keeping off
(delete MEMBER (and (runs_list @old.group) (!= @old.group.kind "admins")))
; whoever runs a group adds someone outside the directory, to put on one by hand
(insert PERSON
  (and (= @new.source "guest") (!= @viewer.source "guest") (exists GROUP @l (runs_list @l))))
; and gives them their address
(insert PERSON_EMAIL
  (and (= @new.source "guest") (= @new.person.source "guest") (!= @viewer.source "guest")
       (exists GROUP @l (runs_list @l))))
; a mail list's managers give it another address
(insert ALIAS (exists GROUP @l (= id @new.target) (runs_list @l)))
; a mail list's managers take one of its other addresses away
(delete ALIAS (exists GROUP @l (= id @old.target) (runs_list @l)))

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
; hide any wiki page from the wiki's lists of pages, or show it again
(set DOCUMENT.hidden (and (admin_of "wiki") (= @old.kind "wiki")))
; any wiki page with no sub-pages
(delete DOCUMENT (and (admin_of "wiki") (= @old.kind "wiki") (not (exists DOCUMENT (= parent @old) (= kind "wiki")))))

;; Old addresses

; an old name of a person the viewer may see, so an old link still finds their page
(read ALIAS (exists PERSON @p (= id @row.target) (person_visible @p)))
; an old name of a group the viewer may see, so an old link still finds its page
(read ALIAS (exists GROUP @g (= id @row.target) (visible @g)))
; an old ticket ID of a guest the viewer may see, so an old guest link still finds them
(read ALIAS (exists MEMBER @m (= id @row.target) (person_visible @m.person)))
; every column of an alias
(read ALIAS (id alias target) true)
; the wiki's old page addresses, so an old link still finds its page
(read REDIRECT (= app "wiki"))
; every column of a redirect
(read REDIRECT (id app old new added) true)
; every column of a search entry, to whoever may read it
(read SEARCH (id target source terminal input summary chunks object made failures) true)

;; Super admins

; every row of every table
(read * (super_admin))
; every column of every table; private ones only in import mode, since the consent view has none
(read * (*) (super_admin))
; add a row to any table
(insert * (super_admin))
; change any column of any table
(set *.* (super_admin))
; remove a row from any table
(delete * (super_admin))
`

type grant struct {
	cond    cond
	comment string
	gated   bool
	every   bool
}

type policySet struct {
	read    map[string][]grant
	open    map[string]bool
	insert  map[string][]grant
	set     map[string][]grant
	delete  map[string][]grant
	show    map[string]cond
	reveal  []grant
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

var actorHeads = map[string]bool{"admin_of": true, "super_admin": true, "birthday_team": true, "mode": true}

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
	p, d, err := compilePolicies(PolicySource)
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
	cx := &compiler{policy: true, defines: map[string]*define{}}
	out := &policySet{read: map[string][]grant{}, open: map[string]bool{}, insert: map[string][]grant{}, set: map[string][]grant{}, delete: map[string][]grant{}, show: map[string]cond{}, reveal: []grant{}, clauses: []Clause{}}
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
		if head == "show" {
			if err := cx.showClause(form, &described, out); err != nil {
				return nil, nil, err
			}
			out.clauses = append(out.clauses, described)
			continue
		}
		if head == "reveal" {
			if err := cx.revealClause(form, &described, out); err != nil {
				return nil, nil, err
			}
			out.clauses = append(out.clauses, described)
			continue
		}
		if head == "read" && len(form.list) == 4 {
			c, err := cx.columnGrant(form, notes[i].comment, out)
			if err != nil {
				return nil, nil, err
			}
			described.Kind, described.Table, described.Condition, described.cond = "read columns", form.list[1].text, form.list[3].render(0), c
			var sc *scope
			if form.list[1].text != "*" {
				t, err := tableNamed(form.list[1])
				if err != nil {
					return nil, nil, err
				}
				sc = &scope{table: t, name: "row"}
			}
			if err := cx.parts(&described, form.list[3], sc); err != nil {
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
		if form.list[1].text == "*" || form.list[1].text == "*.*" {
			if err := cx.wildcard(form, &described, out); err != nil {
				return nil, nil, err
			}
			out.clauses = append(out.clauses, described)
			continue
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
	for _, t := range Tables {
		for _, c := range t.Columns {
			named := slices.ContainsFunc(out.read[t.Name+"."+c.Name], func(g grant) bool { return !g.every })
			if !named && !c.Private {
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

func (cx *compiler) showClause(form *sexp, d *Clause, out *policySet) error {
	if len(form.list) != 3 || form.list[1].isList || form.list[1].kind != atomName {
		return form.errorf("a show clause is (show TABLE condition)")
	}
	t, err := tableNamed(form.list[1])
	if err != nil {
		return err
	}
	if t.Generated {
		return form.list[1].errorf("%s is generated, so it shows as the rows it is made from do", t.Name)
	}
	if _, dup := out.show[t.Name]; dup {
		return form.errorf("%s has two show clauses", t.Name)
	}
	cx.rowOnly = true
	c, err := cx.cond(form.list[2], &scope{table: t, name: "row"})
	cx.rowOnly = false
	if err != nil {
		return err
	}
	out.show[t.Name] = c
	d.Table, d.Condition, d.Rest, d.cond, d.rest = t.Name, form.list[2].render(0), form.list[2].render(0), c, c
	return nil
}

func (cx *compiler) revealClause(form *sexp, d *Clause, out *policySet) error {
	if len(form.list) != 2 {
		return form.errorf("a reveal clause is (reveal condition)")
	}
	c, err := cx.cond(form.list[1], nil)
	if err != nil {
		return err
	}
	out.reveal = append(out.reveal, grant{cond: c, comment: d.Comment})
	d.Condition, d.cond = form.list[1].render(0), c
	return cx.parts(d, form.list[1], nil)
}

func (cx *compiler) wildcard(form *sexp, d *Clause, out *policySet) error {
	head, every := form.head(), form.list[1].text == "*.*"
	if (head == "set") != every || !slices.Contains([]string{"read", "insert", "delete", "set"}, head) {
		return form.errorf("a policy on every table is read *, insert *, delete * or set *.*")
	}
	c, err := cx.cond(form.list[2], nil)
	if err != nil {
		return err
	}
	actor, _ := splitActor(form.list[2])
	g := grant{cond: c, comment: d.Comment, gated: head == "read" && actor != nil}
	for _, t := range Tables {
		switch {
		case head == "read":
			out.read[t.Name] = append(out.read[t.Name], g)
		case t.Generated:
		case head == "insert":
			out.insert[t.Name] = append(out.insert[t.Name], g)
		case head == "delete":
			out.delete[t.Name] = append(out.delete[t.Name], g)
		default:
			for _, col := range t.Columns {
				if !col.Generated {
					out.set[t.Name+"."+col.Name] = append(out.set[t.Name+"."+col.Name], g)
				}
			}
		}
	}
	d.Table, d.Condition, d.cond = "*", form.list[2].render(0), c
	if every {
		d.Column = "*"
	}
	return cx.parts(d, form.list[2], nil)
}

func (cx *compiler) columnGrant(form *sexp, comment string, out *policySet) (cond, error) {
	if form.list[1].isList || form.list[1].kind != atomName || !form.list[2].isList || len(form.list[2].list) == 0 {
		return cond{}, form.errorf("a column grant is (read TABLE (column…) condition)")
	}
	if form.list[1].text == "*" {
		if len(form.list[2].list) != 1 || form.list[2].list[0].isList || form.list[2].list[0].text != "*" {
			return cond{}, form.errorf("a column grant on every table is (read * (*) condition)")
		}
		c, err := cx.cond(form.list[3], nil)
		if err != nil {
			return cond{}, err
		}
		for _, t := range Tables {
			for _, col := range t.Columns {
				if !col.Private {
					out.read[t.Name+"."+col.Name] = append(out.read[t.Name+"."+col.Name], grant{cond: c, comment: comment, every: true})
				}
			}
		}
		return c, nil
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
			return cond{}, item.errorf("%s.%s is private, kept from every view but the whole tables by the consent step", t.Name, col.Name)
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
	if r.system != "" {
		return true
	}
	key := cellKey{table: t.Name, id: row["id"]}
	if seen, ok := r.rows[key]; ok {
		r.policy.Tally(t.Name).Inc("cached")
		return seen
	}
	ok := r.check(t.Name, policies.read[t.Name], &frame{table: t, row: row, name: "row", run: r})
	r.rows[key] = ok
	return ok
}

func (r *run) columnReadable(t *Table, row store.Row, c Column) bool {
	if c.Private || r.system != "" {
		// The consent step strips private columns from every view but the whole tables.
		return true
	}
	key := cellKey{table: t.Name, column: c.Name, id: row["id"]}
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

func (r *run) redact(t *Table, row store.Row, columns []Column) store.Row {
	if columns == nil {
		columns = t.Columns
	}
	out := store.Row{"id": row["id"]}
	for _, c := range columns {
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
	if !ok || t.Generated && (c.Old == nil || c.New != nil) {
		return access.Forbidden("no table %s to change", c.Table)
	}
	if env.System != "" {
		return nil
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
