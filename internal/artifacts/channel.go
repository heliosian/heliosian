package artifacts

import (
	"slices"
	"strings"
)

var domains = []string{"heliosschool.org", "heliosns.org"}

var broadcast = []string{
	"parentsandstaff", "parentsonly", "parentsandstudents", "community", "parents", "chat", "chat2",
	"newstudentfamilies", "new.parents",
	"hummingbirds.parents", "falcons.parents", "hawks.parents", "jays.parents", "ravens.parents",
	"condors.parents", "ospreys.parents", "egrets.parents", "herons.parents",
	"hummingbirds.students", "falcons.students", "hawks.students", "jays.students", "ravens.students",
	"condors.students", "ospreys.students", "egrets.students", "herons.students",
	"hawksandfalcons", "jaysandravens", "condorsandospreys",
}

// Prefixes, so board-finance and boardoftrustees are both the board's.
var withheld = []string{"board", "hca", "room.parents", "staff", "administrators", "leadership", "treasurer", "admissions", "registrar"}

var mailers = []string{"veracross.com", "benchmarkemail.com", "bmetrack.com"}

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
