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

; @g is the group that manages a group the viewer may see, unless its managers hid it by blanking who sees it
(define (manages_visible @g)
  (and (not (blank @g.visible_to))
       (exists GROUP @v (= managed_by @g) (visible @v))))

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

; @p is in effect a member of the group the Birthdays app is open to, or of its admins group
(define (birthday_helper @p)
  (or (exists EFFECTIVE_MEMBER (in group (select APP.visible_to (= key "birthday"))) (= person @p))
      (exists EFFECTIVE_MEMBER (in group (select APP.admins (= key "birthday"))) (= person @p))))

; the viewer is in effect a member of super-admins
(define (super_admin)
  (exists EFFECTIVE_MEMBER (= group.slug "super-admins") (= person @viewer)))

; @g is not closed, and the viewer manages it, added it while it is pending or, unless it is pending, is an effective member of the group it is visible to
(define (visible @g)
  (and (!= @g.status "closed")
       (or (manages @g)
           (and (= @g.status "pending") (= @g.added_by @viewer))
           (and (!= @g.status "pending")
                (in @g.visible_to (select EFFECTIVE_MEMBER.group (= person @viewer)))))))

; the viewer may see @g, and manages it or is an effective member of the group its members are visible to
(define (sees_members @g)
  (and (visible @g)
       (or (manages @g)
           (in @g.members_visible_to (select EFFECTIVE_MEMBER.group (= person @viewer))))))

; @p is the viewer, a guest in one of the viewer's groups or in a group whose members the viewer sees, or anyone else not hidden and active; the consent step has already removed everyone unconsented
(define (person_visible @p)
  (or (= @p @viewer)
      (and (= @p.source "guest")
           (or (exists MEMBER (in group (groups_of @viewer)) (= person @p))
               (exists MEMBER (= person @p) (sees_members group))))
      (and (!= @p.source "guest") (not @p.hidden) (blank @p.deactivated))))

; @p may join @g: @g names no eligible group, or @p is in effect a member of it
(define (eligible_for @g @p)
  (or (blank @g.eligible) (exists EFFECTIVE_MEMBER (= group @g.eligible) (= person @p))))

; the viewer is @p or in a family with @p
(define (self_or_household @p)
  (or (= @p @viewer) (in @p (household @viewer))))

; @g is the group of those who hid something from themselves
(define (hides @g)
  (exists GROUP (= hidden_for @g)))

; the viewer manages @g, is a super admin, is a When admin and @g an event, is a Team admin and @g an activity, a heading under one or one's co-chair offers or leads group, or is a Celebrate admin and @g a party, a celebration, a heading under one or a party's waitlist, unless it is a family, whose members only super admins change, or a hidden group, which each member joins and leaves alone
(define (runs_list @g)
  (and (!= @g.kind "family") (not (hides @g))
       (or (manages @g) (super_admin)
           (and (admin_of "when")
                (or (= @g.kind "event") (and (= @g.kind "group") (= @g.parent.kind "event") (exists GROUP (= audience @g)))))
           (and (admin_of "team")
                (or (= @g.kind "activity") (and (= @g.kind "category") (= @g.parent.kind "activity"))
                    (and (= @g.kind "group") (= @g.parent.kind "activity")
                         (exists GROUP (or (= chair_offers @g) (= leads @g))))))
           (and (admin_of "celebrate")
                (or (in @g.kind "party" "celebration") (and (= @g.kind "category") (= @g.parent.kind "celebration"))
                    (party_waitlist @g))))))

; @g is the leads group of a group of kind k
(define (leads_for @g k)
  (exists GROUP (= leads @g) (= kind k)))

; the calendar import brings @g from a source, which keeps its times and place
(define (sourced @g)
  (exists GROUP_SOURCE (= group @g)))

; @g is the kind of what it sits under, a heading, under a heading the kind of what the heading sits under, an event filed under a category, or a party under a celebration or a heading of one
(define (child_kind @g)
  (or (= @g.kind @g.parent.kind) (= @g.kind "category")
      (and (= @g.parent.kind "category") (= @g.kind @g.parent.parent.kind))
      (and (= @g.kind "event") (= @g.parent.kind "category"))
      (and (= @g.kind "party")
           (or (= @g.parent.kind "celebration")
               (and (= @g.parent.kind "category") (= @g.parent.parent.kind "celebration"))))))

; the viewer may change @ph: a picture of themselves or someone in their family, or of a group they manage or run
(define (edits_photo @ph)
  (or (and (not (blank @ph.person)) (self_or_household @ph.person))
      (and (not (blank @ph.group)) (or (manages @ph.group) (runs_list @ph.group)))))

; @g is a plain group someone may make and run: under nothing, linking nowhere, and seen by its managers alone
(define (own_group @g)
  (and (= @g.kind "group") (blank @g.parent) (blank @g.url) (blank @g.visible_to)))

; the row's reference column c names a group other than the row itself, of which the row is not a managers, waitlist, answer, unsubscribed, co-chair offers, leads, audience or hidden group
(define (counted c)
  (and (!= c id) (!= c.managed_by id) (!= c.waitlist id) (!= c.rsvp_yes id) (!= c.rsvp_no id) (!= c.unsubscribed id) (!= c.chair_offers id)
       (!= c.leads id) (!= c.audience id) (!= c.hidden_for id)))

; @g exists only to serve one group: one row names it, as its managers, waitlist, answer, unsubscribed, co-chair offers, leads, audience or hidden group, and nothing else names it
(define (implementation_only @g)
  (and (= (tally @g
                 (select GROUP.managed_by @r (= managed_by @g) (counted managed_by))
                 (select GROUP.waitlist @r (= waitlist @g) (counted waitlist))
                 (select GROUP.rsvp_yes @r (= rsvp_yes @g) (counted rsvp_yes))
                 (select GROUP.rsvp_no @r (= rsvp_no @g) (counted rsvp_no))
                 (select GROUP.unsubscribed @r (= unsubscribed @g) (counted unsubscribed))
                 (select GROUP.chair_offers @r (= chair_offers @g) (counted chair_offers))
                 (select GROUP.leads @r (= leads @g) (counted leads))
                 (select GROUP.audience @r (= audience @g) (counted audience))
                 (select GROUP.hidden_for @r (= hidden_for @g) (counted hidden_for))
                 (select GROUP.visible_to @r (= visible_to @g) (counted visible_to))
                 (select GROUP.members_visible_to @r (= members_visible_to @g) (counted members_visible_to))
                 (select GROUP.eligible @r (= eligible @g) (counted eligible)))
          1)
       (= (tally @g
                 (select GROUP.managed_by @r (= managed_by @g) (counted managed_by))
                 (select GROUP.waitlist @r (= waitlist @g) (counted waitlist))
                 (select GROUP.rsvp_yes @r (= rsvp_yes @g) (counted rsvp_yes))
                 (select GROUP.rsvp_no @r (= rsvp_no @g) (counted rsvp_no))
                 (select GROUP.unsubscribed @r (= unsubscribed @g) (counted unsubscribed))
                 (select GROUP.chair_offers @r (= chair_offers @g) (counted chair_offers))
                 (select GROUP.leads @r (= leads @g) (counted leads))
                 (select GROUP.audience @r (= audience @g) (counted audience))
                 (select GROUP.hidden_for @r (= hidden_for @g) (counted hidden_for))
                 (select GROUP.visible_to @r (= visible_to @g) (counted visible_to))
                 (select GROUP.members_visible_to @r (= members_visible_to @g) (counted members_visible_to))
                 (select GROUP.eligible @r (= eligible @g) (counted eligible))
                 (select GROUP.parent @r (= parent @g) (counted parent))
                 (select RULE.target (= target @g))
                 (select RULE.within (= within @g))
                 (select APP.admins (= admins @g))
                 (select APP.visible_to (= visible_to @g))
                 (select WIDGET.group (= group @g))
                 (select WIDGET.visible_to (= visible_to @g))
                 (select PERSON.vc_classroom (= vc_classroom @g))
                 (select PERSON.classroom_override (= classroom_override @g))
                 (select PERSON.vc_crew (= vc_crew @g))
                 (select PERSON.crew_override (= crew_override @g))
                 (select PERSON.vc_department (= vc_department @g))
                 (select PERSON.department_override (= department_override @g)))
          1)))

; two rows other than @g's own serving groups name @g as their audience, so @g serves more than one group
(define (shared_audience @g)
  (exists GROUP @a (= visible_to @g) (counted visible_to)
    (exists GROUP @b (= visible_to @g) (counted visible_to) (!= id @a))))

; @g belongs in the rail's lists: open, not a day, a part of one or a category, and not there only to serve another group
(define (sidebar_group @g)
  (and (!= @g.status "closed") (not (in @g.kind "day" "day_part" "category"))
       (or (shared_audience @g) (not (implementation_only @g)))))

; the viewer manages @g, or is in it and it is not hidden from all but its managers
(define (sees_mail @g)
  (or (manages @g)
      (and (not (blank @g.visible_to))
           (in @g (select EFFECTIVE_MEMBER.group (= person @viewer))))))

; @d or a document it sits under was sent to a group whose mail the viewer sees, or it is no mail and was sent to no group
(define (document_visible @d)
  (or (exists DOCUMENT_GROUP (in document (ancestors @d)) (= relation "sent_to") (sees_mail group))
      (and (not (exists DOCUMENT (in id (ancestors @d)) (= kind "mail")))
           (not (exists DOCUMENT_GROUP (in document (ancestors @d)) (= relation "sent_to"))))))

; @g is the group a section of links on the front page holds its links under
(define (home_section @g)
  (exists WIDGET (= group @g)))

; @g is a section of links on the front page, or a link in one
(define (home_thing @g)
  (and (= @g.kind "group") (or (home_section @g) (home_section @g.parent))))

; @g is a plain group with no slug that something on the front page shows to: an app, a widget, or a link
(define (home_audience @g)
  (and (= @g.kind "group") (blank @g.slug)
       (or (exists WIDGET (= visible_to @g))
           (exists APP (= visible_to @g))
           (exists GROUP @l (= visible_to @g) (home_section @l.parent)))))

; @c is Markdown that no document but a wiki page or one of its side cards holds
(define (wiki_content @c)
  (and (= @c.mime "text/markdown")
       (not (exists DOCUMENT (= content @c) (!= kind "wiki") (!= relation "side")))))

; the viewer sent @m, received it, manages the group it went to, or it is an invitation to a student in their family
(define (message_visible @m)
  (or (= @m.from_person @viewer)
      (exists RECIPIENT (= message @m) (= person @viewer))
      (manages @m.group)
      (and (in @m.kind "invitation" "skip")
           (exists RECIPIENT (= message @m) (child_of_viewer person)))))

; @p is a student in the viewer's family
(define (child_of_viewer @p)
  (and (in @p (household @viewer)) (student @p)))

; @p is a student
(define (student @p)
  (in @p (members (slug "students"))))

; the viewer runs a group, or is no student and sees a party taking joining that has a price or a waitlist
(define (brings_guests)
  (or (exists GROUP @l (runs_list @l))
      (and (not (student @viewer))
           (exists GROUP @p (= kind "party") (in join "direct" "others") (visible @p)
             (or (not (blank price)) (not (blank waitlist)))))))

; @p may hold a place the viewer buys at @g: any guest, or anyone in the directory the viewer sees whom it admits
(define (buys_for @g @p)
  (or (= @p.source "guest")
      (and (person_visible @p) (eligible_for @g @p))))

; the viewer's household holds a ticket, a sign-up or a yes for @e
(define (going @e)
  (or (in @e (select MEMBER.group (in person (household @viewer)) (= member "yes")))
      (in @e.rsvp_yes (select MEMBER.group (in person (household @viewer)) (= member "yes")))))

; someone in the viewer's household is on @e's waitlist
(define (waitlisted @e)
  (in @e.waitlist (select MEMBER.group (in person (household @viewer)) (= member "yes"))))

; the group whose slug is w covers @e's audience, has people when @e has no audience, or sits above @e
(define (searched_for @e w)
  (or (covers @e.audience (members (slug w)))
      (and (blank @e.audience) (exists PERSON (in id (members (slug w)))))
      (exists GROUP (= id (slug w)) (in id (ancestors @e)))))

; the viewer hid @e from themselves
(define (hidden_by_viewer @e)
  (in @e.hidden_for (select EFFECTIVE_MEMBER.group (= person @viewer))))

;; Consent

; a person shows when they are a guest, or consented to the directory and are not hidden; a row naming one who doesn't show doesn't show either
(show PERSON (or (= source "guest") (and (= consent "listed") (not hidden))))
; a family shows when it consented to the directory; a row naming one that doesn't show doesn't show either
(show GROUP (or (!= kind "family") (= consent "listed")))

;; Import mode

; a super admin in import mode sees every row, shown or not
(reveal (and (mode "import") (super_admin)))

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
   photo_updated vc_bio path href who_href)
  true)
; a person's birthday and how they take part, to the Birthdays team
(read PERSON (birthday birthday_participation) (birthday_team))
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
(set PERSON.pronunciation_original (self_or_household @old))
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
; write an app setting of one's own
(insert PERSON_SETTING (= @new.person @viewer))
; change an app setting of one's own
(set PERSON_SETTING.value (= @old.person @viewer))
; remove an app setting of one's own
(delete PERSON_SETTING (= @old.person @viewer))
; every column of an app setting
(read PERSON_SETTING (id person app key value) true)
; the birthday years of people the viewer may see, to the Birthdays team
(read BIRTHDAY_YEAR (and (birthday_team) (person_visible person)))
; every column of a birthday year
(read BIRTHDAY_YEAR
  (id person year assigned_to assigned invite_day contacted contacted_by charity note recorded
   recorded_by published published_by)
  true)
; the birthdays of people the viewer may see, each school year, to the Birthdays team
(read BIRTHDAY (and (birthday_team) (person_visible person)))
; every column of a birthday
(read BIRTHDAY (id person year day newsletter ask_by due_by record) true)
; the viewer's own saved searches
(read SAVED_SEARCH (= person @viewer))
; save a search of one's own
(insert SAVED_SEARCH (= @new.person @viewer))
; rename one's own saved search
(set SAVED_SEARCH.name (= @old.person @viewer))
; mark one's own saved search
(set SAVED_SEARCH.emoji (= @old.person @viewer))
; move one's own saved search among one's others
(set SAVED_SEARCH.order (= @old.person @viewer))
; make one's own saved search the one When opens to, or not
(set SAVED_SEARCH.default (= @old.person @viewer))
; change the words of one's own saved search
(set SAVED_SEARCH.search (= @old.person @viewer))
; remove one's own saved search
(delete SAVED_SEARCH (= @old.person @viewer))
; every column of a saved search
(read SAVED_SEARCH (id person name emoji order default token search condition) true)
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
   eligible notify capacity minimum waitlist rsvp_yes rsvp_no chair_offers leads audience hidden_for auto_invite priority price unit parent_ticket_required
   drop_off_allowed start end all_day timing location added_by added path href who_href loop_href when_href team_href celebrate_href)
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
(set GROUP.pronunciation_original (and (= @old.kind "family") (manages @old)))
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
  (id group calendar_event document name description start end all_day location marker hash match)
  true)
; the rules of groups the viewer manages
(read RULE (manages group))
; every column of a rule
(read RULE (id group order exclude target under person search property value replace_with within) true)
; the viewer's own, household's and guests' memberships, those in groups they manage, the managers of visible groups, and everyone where members are shown, but never who hid something
(read MEMBER
  (or (self_or_household person)
      (= guest_of @viewer)
      (and (not (hides group))
           (or (manages group)
               (manages_visible group)
               (sees_members group)))))
; every column of a membership
(read MEMBER
  (id group person member opened guest_of note added_by added)
  true)
; say someone is coming, or not, to a group the viewer may see, for whoever the viewer may answer for
(insert MEMBER (and (answers_visible @new.group) (= @new.member "yes") (answers_for @new)))
; take back someone's answer about coming to a group the viewer may see, for whoever the viewer may answer for
(delete MEMBER (and (answers_visible @old.group) (answers_for @old)))
; join a group that takes joining and no payment and has room: oneself or one's household when eligible, or with join others, once eligible oneself, anyone the viewer sees
(insert MEMBER
  (and (visible @new.group) (in @new.group.join "direct" "others") (blank @new.group.price)
       (= @new.member "yes") (= @new.added_by @viewer)
       (or (and (self_or_household @new.person) (eligible_for @new.group @new.person))
           (and (= @new.group.join "others") (eligible_for @new.group @viewer) (person_visible @new.person)))
       (or (blank @new.group.capacity)
           (< (count MEMBER (= group @new.group) (= member "yes")) @new.group.capacity))))
; note that the viewer or someone in their family opened the invitation to a group the viewer sees, on a row that puts nobody in it
(insert MEMBER
  (and (visible @new.group) (self_or_household @new.person) (blank @new.member) (not (blank @new.opened))
       (blank @new.guest_of) (blank @new.note) (blank @new.added_by)))
; note when the viewer or someone in their family first opened the invitation
(set MEMBER.opened (and (self_or_household @old.person) (blank @old.opened)))
; take back a guest the viewer brought to a group that takes joining and no payment
(delete MEMBER (and (= @old.guest_of @viewer) (not (blank @old.group.join)) (blank @old.group.price)))
; hide something the viewer sees from themselves: join its hidden group, oneself alone
(insert MEMBER
  (and (= @new.group.join "self") (= @new.person @viewer) (= @new.member "yes") (= @new.added_by @viewer)
       (exists GROUP @t (= hidden_for @new.group) (visible @t))))
; leave a group that takes joining and no payment, or take someone in one's family out of it unless each joins it alone, even once it is closed or full
(delete MEMBER
  (and (not (blank @old.group.join)) (blank @old.group.price)
       (or (= @old.person @viewer) (and (!= @old.group.join "self") (self_or_household @old.person)))))
; a person, someone in their family unless each joins alone, or their host gives up a place; the group's managers set anyone's but who hid something; never in a family, which only super admins change
(set MEMBER.member
  (and (!= @old.group.kind "family")
       (or (and (or (= @old.person @viewer)
                    (and (!= @old.group.join "self") (self_or_household @old.person))
                    (= @old.guest_of @viewer))
                (= @new.member "cancelled"))
           (and (manages @old.group) (not (hides @old.group))))))
; what the viewer, their household or a ticket of theirs was billed, and what was billed for a group they manage
(read PAYMENT
  (or (self_or_household person) (and (not (blank for)) (self_or_household for)) (manages group)))
; every column of a payment
(read PAYMENT (id person for group name amount added stripe_invoice status) true)
; pay for a place at a party at its price, owed and not yet invoiced: the viewer, not a student, for themselves or an adult in their household, for someone they may buy a place for
(insert PAYMENT
  (and (= @new.group.kind "party") (visible @new.group) (in @new.group.join "direct" "others") (not (student @viewer))
       (self_or_household @new.person) (not (student @new.person))
       (= @new.amount @new.group.price) (blank @new.status) (blank @new.stripe_invoice) (blank @new.name)
       (not (blank @new.for)) (buys_for @new.group @new.for)))
; take a paid place at a party that takes joining and has room, once its payment is in: the viewer, not a student, for someone they may buy a place for, a guest of theirs brought as theirs
(insert MEMBER
  (and (visible @new.group) (= @new.group.kind "party") (in @new.group.join "direct" "others") (not (blank @new.group.price))
       (= @new.member "yes") (= @new.added_by @viewer) (not (student @viewer))
       (buys_for @new.group @new.person)
       (or (!= @new.person.source "guest") (= @new.guest_of @viewer))
       (or (blank @new.group.capacity)
           (< (count MEMBER (= group @new.group) (= member "yes")) @new.group.capacity))
       (exists PAYMENT (= group @new.group) (= for @new.person) (blank status))))
; ask for another place on a party's waitlist: a guest, brought by the viewer or someone in their household, or someone the viewer may buy a place for, brought by the viewer
(insert MEMBER
  (and (party_waitlist @new.group) (visible @new.group) (= @new.member "yes") (= @new.added_by @viewer)
       (or (and (= @new.person.source "guest") (self_or_household @new.guest_of))
           (and (= @new.guest_of @viewer) (not (student @viewer)) (buys_for @new.group @new.person)))))
; pass a party ticket to someone else: its holder, their household, whoever brought them, or the party's hosts
(set MEMBER.person
  (and (= @old.group.kind "party") (= @old.member "yes")
       (or (self_or_household @old.person) (= @old.guest_of @viewer) (runs_list @old.group))))
; the viewer's own and household's effective memberships, the managers of visible groups, and members of groups that show them, but never who hid something
(read EFFECTIVE_MEMBER
  (or (self_or_household person)
      (and (not (hides group))
           (or (manages_visible group)
               (sees_members group)))))
; every column of an effective membership
(read EFFECTIVE_MEMBER (id group person reasons) true)

; documents sent to a group whose mail the viewer sees, or no mail and sent to no group, and every document under them
(read DOCUMENT (document_visible @row))
; every column of a document
(read DOCUMENT
  (id parent kind relation order name first_page last_page published author url fetch link filename content_id content message
   summary key_points repeats digest classified extracted slug hidden source terminal path href)
  true)
; what the mail the viewer may read asks of its readers
(read TODO (document_visible document))
; every column of a to-do
(read TODO (id document point title summary details link due order) true)
; the viewer's own marks on to-dos
(read TODO_MARK (= person @viewer))
; every column of a mark
(read TODO_MARK (id person todo state changed) true)
; the viewer marks a to-do of mail they may read done or saved
(insert TODO_MARK (and (= @new.person @viewer) (document_visible @new.todo.document)))
; the viewer changes their own mark
(set TODO_MARK.state (= @old.person @viewer))
; and when they set it
(set TODO_MARK.changed (= @old.person @viewer))
; the viewer takes their own mark away
(delete TODO_MARK (= @old.person @viewer))
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

; mail the viewer sent or received, and mail to groups they manage
(read MESSAGE (message_visible @row))
; every column of a message
(read MESSAGE
  (id direction kind group about parent from_person from_address subject header_id content created state detail)
  true)
; the viewer's own deliveries, invitations to the students in their family, deliveries of mail they sent, and of mail to groups they manage
(read RECIPIENT
  (or (= person @viewer)
      (and (in message.kind "invitation" "skip") (child_of_viewer person))
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
; the team gives a staff member's birthday its record of a school year, unless they asked to be left out, assigned to someone on the team, giving only to an allowed charity
(insert BIRTHDAY_YEAR
  (and (birthday_team) (person_visible @new.person) (!= @new.person.birthday_participation "skip")
       (or (blank @new.assigned_to) (birthday_helper @new.assigned_to))
       (or (blank @new.charity) @new.charity.allowed)))
; the team assigns a birthday to someone on the team, or takes it back
(set BIRTHDAY_YEAR.assigned_to
  (and (birthday_team) (!= @old.person.birthday_participation "skip")
       (or (blank @new.assigned_to) (birthday_helper @new.assigned_to))))
; the team says when it was assigned
(set BIRTHDAY_YEAR.assigned (birthday_team))
; the team says when the staff member was asked
(set BIRTHDAY_YEAR.contacted (birthday_team))
; the team says who asked
(set BIRTHDAY_YEAR.contacted_by (birthday_team))
; the team records the charity, an allowed one
(set BIRTHDAY_YEAR.charity (and (birthday_team) (or (blank @new.charity) @new.charity.allowed)))
; the team records the staff member's own words
(set BIRTHDAY_YEAR.note (birthday_team))
; the team says when the charity was recorded
(set BIRTHDAY_YEAR.recorded (birthday_team))
; the team says who recorded it
(set BIRTHDAY_YEAR.recorded_by (birthday_team))
; the team says when it went to the newsletter
(set BIRTHDAY_YEAR.published (birthday_team))
; the team says who sent it to the newsletter
(set BIRTHDAY_YEAR.published_by (birthday_team))
; a Birthdays admin removes a year's record
(delete BIRTHDAY_YEAR (admin_of "birthday"))
; the team keeps a staff member's birthday
(set PERSON.birthday (and (birthday_team) (in @old.id (members (slug "staff")))))
; the team keeps how a staff member takes part
(set PERSON.birthday_participation (and (birthday_team) (in @old.id (members (slug "staff")))))
; the team adds an allowed charity; a Birthdays admin also adds one that is not
(insert CHARITY (and (birthday_team) (or @new.allowed (admin_of "birthday"))))
; the team names a charity
(set CHARITY.name (birthday_team))
; the team writes the newsletter's sentence about a charity
(set CHARITY.description (birthday_team))
; the team keeps the page where a donation to a charity is made
(set CHARITY.url (birthday_team))
; the team keeps a charity's tax number
(set CHARITY.ein (birthday_team))
; a Birthdays admin allows a charity, or stops allowing it
(set CHARITY.allowed (admin_of "birthday"))
; a Birthdays admin says why a charity is not allowed
(set CHARITY.not_allowed_reason (admin_of "birthday"))
; a Birthdays admin removes a charity no birthday names
(delete CHARITY (and (admin_of "birthday") (not (exists BIRTHDAY_YEAR (= charity @old)))))
; a Birthdays admin changes Staff Birthdays' settings
(set SETTING.value (and (admin_of "birthday") (= @old.app "birthday")))
; a Birthdays admin adds a Staff Birthdays setting
(insert SETTING (and (admin_of "birthday") (= @new.app "birthday")))
; a Birthdays admin adds someone to a group whose members the Birthdays app is open to
(insert MEMBER
  (and (admin_of "birthday") (= @new.member "yes")
       (in @new.group (select RULE.target (in group (select APP.visible_to (= key "birthday")))))))
; a Birthdays admin takes someone out of a group whose members the Birthdays app is open to
(delete MEMBER
  (and (admin_of "birthday")
       (in @old.group (select RULE.target (in group (select APP.visible_to (= key "birthday")))))))
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
(read GROUP (and (admin_of "who") (in kind "family" "group" "classroom" "grade" "gradeband" "crew" "department")))
; every directory group's and plain group's memberships, but who hid something
(read MEMBER (and (admin_of "who") (in group.kind "family" "group" "classroom" "grade" "gradeband" "crew" "department") (not (hides group))))
; every directory group's and plain group's effective members, but who hid something
(read EFFECTIVE_MEMBER (and (admin_of "who") (in group.kind "family" "group" "classroom" "grade" "gradeband" "crew" "department") (not (hides group))))
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
(set PERSON.pronunciation_original (admin_of "who"))
; write anyone's about-me words
(set PERSON.facts (admin_of "who"))
; date anyone's about-me words
(set PERSON.facts_updated (admin_of "who"))
; date anyone's picture
(set PERSON.photo_updated (admin_of "who"))
; take anyone out of every view and sign-in; putting them back is for super admins in import mode
(set PERSON.hidden (admin_of "who"))
; make someone a gradeband's room parent
(insert MEMBER (and (admin_of "who") (= @new.group.kind "group") (= @new.group.parent.kind "gradeband") (= @new.member "yes")))
; stop someone being a gradeband's room parent
(delete MEMBER (and (admin_of "who") (= @old.group.kind "group") (= @old.group.parent.kind "gradeband")))
; override any family's address
(set GROUP.address_override (and (admin_of "who") (= @old.kind "family")))
; override any family's phone
(set GROUP.phone_override (and (admin_of "who") (= @old.kind "family")))
; write the words under any family's photo
(set GROUP.description (and (admin_of "who") (= @old.kind "family")))
; record how any family's name is said
(set GROUP.pronunciation_original (and (admin_of "who") (= @old.kind "family")))
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
; add an event under a category, live
(insert GROUP (and (admin_of "when") (= @new.kind "event") (= @new.parent.kind "category")))
; add a top-level category
(insert GROUP (and (admin_of "when") (= @new.kind "category") (blank @new.parent)))
; a top-level category's name
(set GROUP.name (and (admin_of "when") (= @old.kind "category") (blank @old.parent)))
; a top-level category's description
(set GROUP.description (and (admin_of "when") (= @old.kind "category") (blank @old.parent)))
; whether When's filter starts with a top-level category on
(set GROUP.default (and (admin_of "when") (= @old.kind "category") (blank @old.parent)))
; a top-level category's place among the others
(set GROUP.order (and (admin_of "when") (= @old.kind "category") (blank @old.parent)))
; a top-level category's slug, the word saved searches name it by
(set GROUP.slug (and (admin_of "when") (= @old.kind "category") (blank @old.parent)))
; who may add events under a top-level category
(set GROUP.adding (and (admin_of "when") (= @old.kind "category") (blank @old.parent)))
; a top-level category nothing sits under
(delete GROUP (and (admin_of "when") (= @old.kind "category") (blank @old.parent) (not (exists GROUP (= parent @old)))))
; a top-level category's picture
(insert PHOTO (and (admin_of "when") (= @new.group.kind "category") (blank @new.group.parent)))
; remove a top-level category's picture
(delete PHOTO (and (admin_of "when") (= @old.group.kind "category") (blank @old.group.parent)))
; open, cancel or close an event
(set GROUP.status (and (admin_of "when") (= @old.kind "event")))
; put an event on the calendar or take it off
(set GROUP.listed (and (admin_of "when") (= @old.kind "event")))
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
; every activity's volunteers
(read MEMBER (and (admin_of "team") (in group.kind "activity")))
; every activity's leads group
(read GROUP (and (admin_of "team") (leads_for @row "activity")))
; every activity's co-chairs
(read MEMBER (and (admin_of "team") (leads_for group "activity")))
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
; Team's introduction and expense form
(set SETTING.value (and (admin_of "team") (= @old.app "team")))
; a Team setting not yet written
(insert SETTING (and (admin_of "team") (= @new.app "team")))
; an old Team address and where it goes now
(insert REDIRECT (and (admin_of "team") (= @new.app "team")))
; where an old Team address goes
(set REDIRECT.new (and (admin_of "team") (= @old.app "team")))
; which old Team address it is
(set REDIRECT.old (and (admin_of "team") (= @old.app "team")))
; an old Team address no longer kept
(delete REDIRECT (and (admin_of "team") (= @old.app "team")))
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
; every payment
(read PAYMENT (admin_of "celebrate"))
; say where a payment stands
(set PAYMENT.status (admin_of "celebrate"))
; say who pays
(set PAYMENT.person (admin_of "celebrate"))
; Celebrate's introduction, ticket note, current celebration and banner
(set SETTING.value (and (admin_of "celebrate") (= @old.app "celebrate")))
; a Celebrate setting not yet written
(insert SETTING (and (admin_of "celebrate") (= @new.app "celebrate")))
; make a plain group to host parties
(insert GROUP (and (admin_of "celebrate") (= @new.kind "group")))
; add a celebration under the Celebrate category
(insert GROUP
  (and (admin_of "celebrate") (= @new.kind "celebration") (= @new.parent (slug "celebrate"))
       (= @new.added_by @viewer) (blank @new.managed_by) (blank @new.url)))
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

; anyone but a guest makes a mail list, a plain group under nothing and linking nowhere, run by a group they are in, seen by its managers or by everyone
(insert GROUP
  (and (= @new.kind "group") (blank @new.parent) (blank @new.url)
       (= @new.status "open") (not (blank @new.slug)) (= @new.added_by @viewer)
       (!= @viewer.source "guest")
       (exists EFFECTIVE_MEMBER (= group @new.managed_by) (= person @viewer))
       (or (blank @new.visible_to) (= @new.visible_to (slug "everyone")))
       (or (blank @new.members_visible_to) (= @new.members_visible_to (slug "everyone")))))
; a mail list's managers rename it
(set GROUP.name (runs_list @old))
; a mail list's managers change its address or take it away
(set GROUP.slug (runs_list @old))
; a mail list's managers describe it
(set GROUP.description (runs_list @old))
; a mail list's managers say who may post to it
(set GROUP.posting (runs_list @old))
; a mail list's managers say who may reply to its posts
(set GROUP.replying (runs_list @old))
; a mail list's managers show it to its managers alone, its own members, or everyone
(set GROUP.visible_to
  (and (runs_list @old)
       (or (blank @new.visible_to) (= @new.visible_to @old.id) (= @new.visible_to (slug "everyone")))))
; a mail list's managers show who is on it to its managers alone, its own members, or everyone
(set GROUP.members_visible_to
  (and (runs_list @old)
       (or (blank @new.members_visible_to) (= @new.members_visible_to @old.id)
           (= @new.members_visible_to (slug "everyone")))))
; a group's managers close it, open it again or say it happened, and whoever runs what it sits under approves it
(set GROUP.status
  (and (runs_list @old) (in @new.status "open" "closed" "done")
       (or (!= @old.status "pending") (runs_list @old.parent))))
; a group's managers say when it starts, unless the calendar import brings it
(set GROUP.start (and (runs_list @old) (not (sourced @old))))
; a group's managers say when it ends, unless the calendar import brings it
(set GROUP.end (and (runs_list @old) (not (sourced @old))))
; a group's managers say it runs whole days, unless the calendar import brings it
(set GROUP.all_day (and (runs_list @old) (not (sourced @old))))
; a group's managers say where it happens, unless the calendar import brings it
(set GROUP.location (and (runs_list @old) (not (sourced @old))))
; a group's managers say in words when it happens
(set GROUP.timing (runs_list @old))
; a group's managers write the line under its name
(set GROUP.subtitle (runs_list @old))
; a group's managers say how many places it has
(set GROUP.capacity (runs_list @old))
; a party's hosts set its price
(set GROUP.price (and (runs_list @old) (= @old.kind "party")))
; a party's hosts, and Celebrate admins for a celebration, give its street address
(set GROUP.address_override (and (runs_list @old) (in @old.kind "party" "celebration")))
; a party's hosts bill a place they give at its price, owed and not yet invoiced
(insert PAYMENT
  (and (= @new.group.kind "party") (runs_list @new.group) (= @new.amount @new.group.price)
       (blank @new.status) (blank @new.stripe_invoice) (blank @new.name)))
; a party's hosts say what its price is per
(set GROUP.unit (and (runs_list @old) (= @old.kind "party")))
; a party's hosts say how many it needs to go ahead
(set GROUP.minimum (and (runs_list @old) (= @old.kind "party")))
; a party's hosts say whether a student's place needs a parent with one
(set GROUP.parent_ticket_required (and (runs_list @old) (= @old.kind "party")))
; a party's hosts say whether students may be dropped off
(set GROUP.drop_off_allowed (and (runs_list @old) (= @old.kind "party")))
; a party's hosts give it a waitlist, naming a group under it, or take it away
(set GROUP.waitlist
  (and (runs_list @old) (= @old.kind "party") (or (blank @new.waitlist) (= @new.waitlist.parent @old.id))))
; a group's managers give it a poster
(set GROUP.flyer (runs_list @old))
; a group's managers say whether whoever newly comes onto its guest list is sent the invitation at once
(set GROUP.auto_invite (runs_list @old))
; a group's managers say who may join it by themselves and add others
(set GROUP.join (runs_list @old))
; a group's managers say whose members may join it, naming only a group whose members they see
(set GROUP.eligible (and (runs_list @old) (or (blank @new.eligible) (sees_members @new.eligible))))
; a group's managers say who may add groups under it
(set GROUP.adding (runs_list @old))
; a group's managers say who hears of sign-ups and additions on it and under it, naming only a group whose members they see
(set GROUP.notify (and (runs_list @old) (or (blank @new.notify) (sees_members @new.notify))))
; a group's managers open it to offers to co-chair it, naming a group under it, or close it
(set GROUP.chair_offers (and (runs_list @old) (or (blank @new.chair_offers) (= @new.chair_offers.parent @old.id))))
; a group's managers give it a co-chair offers group: open, under it, joined directly by its own members alone, seen as it is, and who offered seen by its managers alone
(insert GROUP
  (and (= @new.kind "group") (runs_list @new.parent) (= @new.status "open") (= @new.added_by @viewer)
       (blank @new.managed_by) (blank @new.url) (= @new.join "direct") (= @new.eligible @new.parent)
       (= @new.visible_to @new.parent.visible_to) (blank @new.members_visible_to)))
; a group's managers name the group under it of those who lead it, or take it away
(set GROUP.leads (and (runs_list @old) (or (blank @new.leads) (= @new.leads.parent @old.id))))
; a group's managers give it a leads group: open, under it, joined by nobody by themselves, it and who is in it seen as it is
(insert GROUP
  (and (= @new.kind "group") (runs_list @new.parent) (= @new.status "open") (= @new.added_by @viewer)
       (blank @new.managed_by) (blank @new.url) (blank @new.join) (blank @new.eligible)
       (or (blank @new.visible_to) (= @new.visible_to @new.parent.visible_to))
       (or (blank @new.members_visible_to) (= @new.members_visible_to @new.parent.visible_to))))
; a group's managers place it among the groups beside it
(set GROUP.order (runs_list @old))
; a group's managers move it under another group they run
(set GROUP.parent (and (runs_list @old) (not (blank @new.parent)) (runs_list @new.parent)))
; a group's managers add a group or heading under it, live, seen as it is or by its managers alone
(insert GROUP
  (and (runs_list @new.parent) (child_kind @new) (= @new.status "open") (= @new.added_by @viewer)
       (blank @new.managed_by) (blank @new.url)
       (or (blank @new.visible_to) (= @new.visible_to @new.parent.visible_to))
       (or (blank @new.members_visible_to) (= @new.members_visible_to @new.parent.members_visible_to))))
; anyone who sees a group that takes additions adds one under it, live or waiting for its managers as its adding says, seen as it is
(insert GROUP
  (and (visible @new.parent) (in @new.parent.adding "anyone" "approval") (!= @new.kind "category") (child_kind @new)
       (!= @viewer.source "guest") (= @new.added_by @viewer) (blank @new.managed_by) (blank @new.url)
       (= @new.visible_to @new.parent.visible_to) (= @new.members_visible_to @new.parent.members_visible_to)
       (or (and (= @new.parent.adding "anyone") (= @new.status "open")) (= @new.status "pending"))))
; anyone but a guest adds an event under a category, live, that nobody finds on the calendar: invite-only, seen by its managers alone until they show it to its guests, or unlisted, reached by its link
(insert GROUP
  (and (= @new.kind "event") (= @new.parent.kind "category") (!= @viewer.source "guest") (= @new.added_by @viewer)
       (blank @new.managed_by) (blank @new.url) (= @new.status "open")
       (or (blank @new.visible_to) (and (= @new.visible_to @new.parent.visible_to) (not @new.listed)))))
; whoever added an event or party that has no managers gives it a plain group of their own to manage it, so they host it
(set GROUP.managed_by
  (and (in @old.kind "event" "party") (= @old.added_by @viewer) (blank @old.managed_by)
       (= @new.managed_by.kind "group") (= @new.managed_by.added_by @viewer) (= @new.managed_by.managed_by @new.managed_by)
       (not (exists GROUP (= managed_by @new.managed_by) (!= id @new.managed_by) (!= id @old.id)))))
; an event's hosts take it off the calendar, or put it on to wait for approval
(set GROUP.listed (and (= @old.kind "event") (runs_list @old) (or (not @new.listed) (= @new.status "pending"))))
; an event's hosts send it for approval as they put it on the calendar
(set GROUP.status (and (= @old.kind "event") (runs_list @old) (= @new.status "pending") @new.listed (not @old.listed)))
; an event's hosts open one waiting for approval as they take it off the calendar
(set GROUP.status (and (= @old.kind "event") (runs_list @old) (= @old.status "pending") (= @new.status "open") (not @new.listed)))
; an event's hosts name the group under it of those it is for
(set GROUP.audience (and (= @old.kind "event") (runs_list @old) (= @new.audience.parent @old.id)))
; a group's managers remove one under it that holds no one and nothing
(delete GROUP
  (and (not (blank @old.parent)) (runs_list @old.parent)
       (not (exists GROUP (= parent @old))) (not (exists MEMBER (= group @old))) (not (exists RULE (= group @old)))))
; whoever runs a group runs the plain group that manages it: makes someone a co-chair, or stops them being one
(insert MEMBER
  (and (= @new.group.kind "group") (not (role_group @new.group)) (= @new.member "yes")
       (exists GROUP @s (= managed_by @new.group) (!= id @new.group) (runs_list @s))))
; and takes someone out of it
(delete MEMBER
  (and (= @old.group.kind "group") (not (role_group @old.group))
       (exists GROUP @s (= managed_by @old.group) (!= id @old.group) (runs_list @s))))
; a group's managers give it a plain group of its own to manage it, one managing nothing else
(set GROUP.managed_by
  (and (runs_list @old) (= @new.managed_by.kind "group") (not (role_group @new.managed_by))
       (not (exists GROUP (= managed_by @new.managed_by) (!= id @new.managed_by) (!= id @old.id)))))
; Team admins mark an activity the community most needs hands for
(set GROUP.priority (and (admin_of "team") (= @old.kind "activity")))
; a mail list's managers give it a rule naming only groups and people whose members they may see, and reaching under a group only one they manage
(insert RULE
  (and (runs_list @new.group)
       (or (blank @new.target) (sees_members @new.target))
       (or (not @new.under) (manages @new.target) (super_admin))
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
; whoever runs a group adds someone outside the directory, to put on one by hand, as does anyone but a student who sees a party they could buy a place at or wait for, to bring as their guest
(insert PERSON
  (and (= @new.source "guest") (!= @viewer.source "guest") (brings_guests)))
; and gives them their address
(insert PERSON_EMAIL
  (and (= @new.source "guest") (= @new.person.source "guest") (!= @viewer.source "guest") (brings_guests)))
; a mail list's managers give it another address
(insert ALIAS (and (not @new.hidden) (exists GROUP @l (= id @new.target) (runs_list @l))))
; a mail list's managers take one of its other addresses away, but never a past ID
(delete ALIAS (and (not @old.hidden) (exists GROUP @l (= id @old.target) (runs_list @l))))

;; Home admins

; every plain group and admins group
(read GROUP (and (admin_of "home") (in kind "group" "admins")))
; the members of plain groups and admins groups, but who hid something
(read MEMBER (and (admin_of "home") (in group.kind "group" "admins") (not (hides group))))
; the effective members of plain groups and admins groups, but who hid something
(read EFFECTIVE_MEMBER (and (admin_of "home") (in group.kind "group" "admins") (not (hides group))))
; the rules that pick plain groups' and admins groups' members
(read RULE (and (admin_of "home") (in group.kind "group" "admins")))
; every app, whoever it is open to
(read APP (admin_of "home"))
; every front-page widget, whoever it is open to
(read WIDGET (admin_of "home"))
; rename an app, change its subtitle, its place, or whom it is open to
(set APP.name (admin_of "home"))
; and its subtitle
(set APP.subtitle (admin_of "home"))
; and its place in the app switch
(set APP.order (admin_of "home"))
; and whom it is open to
(set APP.visible_to (admin_of "home"))
; add a section of links
(insert WIDGET (and (admin_of "home") (= @new.key "links")))
; take a section of links away
(delete WIDGET (and (admin_of "home") (= @old.key "links")))
; a widget's heading
(set WIDGET.name (admin_of "home"))
; its mark
(set WIDGET.icon (admin_of "home"))
; how its links or apps are laid out
(set WIDGET.style (admin_of "home"))
; whether descriptions show
(set WIDGET.descriptions (admin_of "home"))
; whether it shows in the rail
(set WIDGET.sidebar (admin_of "home"))
; whom it shows to
(set WIDGET.visible_to (admin_of "home"))
; its place on the page
(set WIDGET.order (admin_of "home"))
; the front page's row layout
(insert SETTING (and (admin_of "home") (= @new.app "home") (= @new.key "layout")))
; and change it
(set SETTING.value (and (admin_of "home") (= @old.app "home") (= @old.key "layout")))
; start a plain group: a section's group, a link in a section, or the group a thing on the page shows to
(insert GROUP (and (admin_of "home") (= @new.kind "group") (blank @new.slug) (or (blank @new.parent) (home_section @new.parent))))
; a section's or a link's name
(set GROUP.name (and (admin_of "home") (home_thing @old)))
; a link's description
(set GROUP.description (and (admin_of "home") (home_thing @old)))
; a link's address
(set GROUP.url (and (admin_of "home") (home_thing @old)))
; whom a section or a link shows to
(set GROUP.visible_to (and (admin_of "home") (home_thing @old)))
; a link's place in its section
(set GROUP.order (and (admin_of "home") (home_thing @old)))
; move a link into another section
(set GROUP.parent (and (admin_of "home") (home_section @old.parent) (home_section @new.parent)))
; take a link away
(delete GROUP (and (admin_of "home") (= @old.kind "group") (home_section @old.parent)))
; take away a plain group nothing names any longer: a section after its widget, or the group a thing on the page showed to
(delete GROUP (and (admin_of "home") (= @old.kind "group") (blank @old.parent) (blank @old.slug) (not @old.listed) (blank @old.managed_by)))
; a rule of the group a thing on the page shows to
(insert RULE (and (admin_of "home") (home_audience @new.group)))
; and take it away
(delete RULE (and (admin_of "home") (home_audience @old.group)))
; someone named by hand in the group a thing on the page shows to
(insert MEMBER (and (admin_of "home") (home_audience @new.group) (= @new.member "yes")))
; and take them off
(delete MEMBER (and (admin_of "home") (home_audience @old.group)))
; a section's or a link's picture
(insert PHOTO (and (admin_of "home") (or (home_section @new.group) (home_section @new.group.parent))))
; take it away
(delete PHOTO (and (admin_of "home") (or (home_section @old.group) (home_section @old.group.parent))))

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
(read ALIAS (id alias target hidden) true)
; the wiki's, Team's and Celebrate's old page addresses, so an old link still finds its page
(read REDIRECT (in app "wiki" "team" "celebrate"))
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
	free    bool
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
		free := !form.list[2].isList && form.list[2].kind == atomName && form.list[2].text == "true"
		into[form.list[1].text] = append(into[form.list[1].text], grant{cond: c, comment: notes[i].comment, gated: head == "read" && actor != nil, free: free})
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
	g := grant{cond: c, comment: d.Comment, gated: head == "read" && actor != nil, free: true}
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
					out.read[t.Name+"."+col.Name] = append(out.read[t.Name+"."+col.Name], grant{cond: c, comment: comment, every: true, free: true})
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
		out.read[key] = append(out.read[key], grant{cond: c, comment: comment, free: open})
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
	if r.system != "" || r.everyRow(t, "") {
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
	if r.everyRow(t, c.Name) {
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

func (r *run) everyRow(t *Table, column string) bool {
	key := cellKey{table: t.Name, column: column}
	if held, ok := r.every[key]; ok {
		return held
	}
	name, policy := t.Name, t.Name
	if column != "" {
		name, policy = t.Name+" columns", t.Name+"."+column
	}
	free := []grant{}
	for _, g := range policies.read[policy] {
		if g.free {
			free = append(free, g)
		}
	}
	held := r.check(name, free, &frame{run: r})
	r.every[key] = held
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
	Table  string
	Old    store.Row
	New    store.Row
	Before []Change
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
