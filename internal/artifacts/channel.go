package artifacts

import (
	"slices"
	"strings"

	"heliosian/internal/mail"
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

var mailers = []string{"veracross.com"}

func Channel(listID, from string) (channel, kind string, ok bool) {
	list := listName(listID)
	if list != "" {
		for _, domain := range domains {
			if name, cut := strings.CutSuffix(list, "."+domain); cut && slices.Contains(broadcast, name) {
				return name, KindList, true
			}
		}
		return list, "", false
	}
	if mailer(mail.AddressOf(from)) {
		return "newsletter", KindNewsletter, true
	}
	return "", "", false
}

func listName(listID string) string {
	if i := strings.LastIndex(listID, "<"); i >= 0 {
		listID = listID[i+1:]
	}
	return strings.ToLower(strings.Trim(strings.TrimSpace(listID), "<> "))
}

func mailer(address string) bool {
	domain := strings.ToLower(address[strings.LastIndex(address, "@")+1:])
	for _, host := range mailers {
		if domain == host || strings.HasSuffix(domain, "."+host) {
			return true
		}
	}
	return false
}

func vouch(lines []mail.HeaderLine, m Message) string {
	channel, kind, ok := m.Broadcast()
	if !ok {
		return ""
	}
	switch kind {
	case KindNewsletter:
		return mail.Authenticated(lines)
	case KindList:
		local, domain, _ := strings.Cut(strings.ToLower(mail.SealedSender(lines)), "@")
		if strings.HasPrefix(local, channel+"+") && slices.Contains(domains, domain) {
			return ""
		}
		return "the forwarding mailbox's sealed results do not show the list sending it"
	}
	return "no proof is known for kind " + kind
}
