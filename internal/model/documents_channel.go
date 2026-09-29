package model

import (
	"slices"
	"strings"

	"heliosian/internal/mail"
)

const schoolMailDomain = "heliosschool.org"

var broadcastLists = []string{
	"parentsandstaff", "parentsonly", "parentsandstudents", "community", "parents", "chat", "chat2",
	"newstudentfamilies", "new.parents",
	"hummingbirds.parents", "falcons.parents", "hawks.parents", "jays.parents", "ravens.parents",
	"condors.parents", "ospreys.parents", "egrets.parents", "herons.parents",
	"hummingbirds.students", "falcons.students", "hawks.students", "jays.students", "ravens.students",
	"condors.students", "ospreys.students", "egrets.students", "herons.students",
	"hawksandfalcons", "jaysandravens", "condorsandospreys",
}

var newsletterMailers = []string{"veracross.com", "campsite-mail.com"}

func MailChannel(listID, from string) (channel, kind string, ok bool) {
	list := listName(listID)
	if list != "" {
		if name, cut := strings.CutSuffix(list, "."+schoolMailDomain); cut && slices.Contains(broadcastLists, name) {
			return name, DocumentKindList, true
		}
		return list, "", false
	}
	if newsletterMailer(mail.AddressOf(from)) {
		return "newsletter", DocumentKindNewsletter, true
	}
	return "", "", false
}

func listName(listID string) string {
	if i := strings.LastIndex(listID, "<"); i >= 0 {
		listID = listID[i+1:]
	}
	return strings.ToLower(strings.Trim(strings.TrimSpace(listID), "<> "))
}

func newsletterMailer(address string) bool {
	domain := strings.ToLower(address[strings.LastIndex(address, "@")+1:])
	for _, host := range newsletterMailers {
		if domain == host || strings.HasSuffix(domain, "."+host) {
			return true
		}
	}
	return false
}

func vouch(lines []mail.HeaderLine, m DocumentMessage) string {
	channel, kind, ok := m.Broadcast()
	if !ok {
		return ""
	}
	switch kind {
	case DocumentKindNewsletter:
		return mail.Authenticated(lines)
	case DocumentKindList:
		local, at, _ := strings.Cut(strings.ToLower(mail.SealedSender(lines)), "@")
		if strings.HasPrefix(local, channel+"+") && at == schoolMailDomain {
			return ""
		}
		return "the forwarding mailbox's sealed results do not show the list sending it"
	}
	return "no proof is known for kind " + kind
}
