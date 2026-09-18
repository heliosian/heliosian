package artifacts

import (
	"slices"
	"strings"
)

// domains are the school's mail domains: the one it uses and the one it used
// before.
var domains = []string{"heliosschool.org", "heliosns.org"}

// broadcast are the addresses a whole school or a whole class is written to,
// whether the mail carries a list header or not: the school's announcement
// lists, the chat everyone is on, each classroom's parents and students,
// each grade band, and the reading and maths groups a teacher writes to.
var broadcast = []string{
	"parentsandstaff", "parentsonly", "parentsandstudents", "community", "parents", "chat", "chat2",
	"newstudentfamilies", "new.parents",
	"hummingbirds.parents", "falcons.parents", "hawks.parents", "jays.parents", "ravens.parents",
	"condors.parents", "ospreys.parents", "egrets.parents", "herons.parents",
	"hummingbirds.students", "falcons.students", "hawks.students", "jays.students", "ravens.students",
	"condors.students", "ospreys.students", "egrets.students", "herons.students",
	"hawksandfalcons", "jaysandravens", "condorsandospreys",
}

// withheld is what a committee or a role keeps to itself: the board and
// every committee of it, the HCA's teams, the room parents, the staff,
// admissions, the registrar. None of it was sent to the community, and
// Helios Ask answers whoever asks, so none of it may enter the corpus.
// Matched against the front of a list's name, so board-finance and
// boardoftrustees are both the board's.
var withheld = []string{"board", "hca", "room.parents", "staff", "administrators", "leadership", "treasurer", "admissions", "registrar"}

// mailers are the hosts the office has sent the newsletter through: Benchmark
// Email until 2024, Veracross since.
var mailers = []string{"veracross.com", "benchmarkemail.com", "bmetrack.com"}

// Channel is where a message went out and what kind of thing it is: the list
// it came through, one of the newsletter mailers, or the broadcast address it
// was written to. A message a committee kept to itself, and one nothing marks
// as sent to everyone, belong to no channel and are no part of the corpus.
//
// listID is the List-Id header as the mail carries it, from the sender's
// address, and recipients every address in To and Cc.
func Channel(listID, from string, recipients []string) (channel, kind string, ok bool) {
	list := listName(listID)
	if withheldList(list) {
		return list, "", false
	}
	if list != "" {
		for _, domain := range domains {
			if name, cut := strings.CutSuffix(list, "."+domain); cut {
				return name, KindList, true
			}
		}
		if mailer(list) {
			return "newsletter", KindNewsletter, true
		}
		return list, "", false
	}
	if mailer(strings.ToLower(from)) {
		return "newsletter", KindNewsletter, true
	}
	for _, address := range recipients {
		local, domain, _ := strings.Cut(strings.ToLower(strings.TrimSpace(address)), "@")
		if slices.Contains(domains, domain) && slices.Contains(broadcast, local) {
			return local, KindAnnouncement, true
		}
	}
	return "", "", false
}

// listName is a list's bare name, from a List-Id header however it is
// written: a name in angle brackets, with or without a description in front.
func listName(listID string) string {
	if i := strings.LastIndex(listID, "<"); i >= 0 {
		listID = listID[i+1:]
	}
	return strings.ToLower(strings.Trim(strings.TrimSpace(listID), "<> "))
}

func withheldList(list string) bool {
	for _, prefix := range withheld {
		if strings.HasPrefix(list, prefix) {
			return true
		}
	}
	return false
}

func mailer(address string) bool {
	for _, host := range mailers {
		if strings.Contains(address, host) {
			return true
		}
	}
	return false
}
