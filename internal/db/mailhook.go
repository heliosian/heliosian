package db

import (
	"bytes"
	"log/slog"
	"net/http"
	netmail "net/mail"
	"slices"
	"strings"
	"time"

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

func newsletterMailer(address string) bool {
	domain := strings.ToLower(address[strings.LastIndex(address, "@")+1:])
	for _, host := range newsletterMailers {
		if domain == host || strings.HasSuffix(domain, "."+host) {
			return true
		}
	}
	return false
}

func broadcastChannel(listID, from string) (channel string, newsletter, ok bool) {
	if list := listName(listID); list != "" {
		name, cut := strings.CutSuffix(list, "."+schoolMailDomain)
		return name, false, cut && slices.Contains(broadcastLists, name)
	}
	if newsletterMailer(mail.AddressOf(from)) {
		return "newsletter", true, true
	}
	return "", false, false
}

func vouch(lines []mail.HeaderLine, channel string, newsletter bool) string {
	if newsletter {
		return mail.Authenticated(lines)
	}
	local, at, _ := strings.Cut(strings.ToLower(mail.SealedSender(lines)), "@")
	if strings.HasPrefix(local, channel+"+") && at == schoolMailDomain {
		return ""
	}
	return "the forwarding mailbox's sealed results do not show the list sending it"
}

func RegisterMailHook(mux *http.ServeMux, s *Store, pics *Pictures, signingKey string) {
	mux.HandleFunc("POST /hooks/mail/mime", func(w http.ResponseWriter, r *http.Request) {
		fields, err := mail.Notification(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := mail.VerifyNotification(signingKey, fields, time.Now()); err != nil {
			slog.WarnContext(r.Context(), "mail hook: inbound call refused", "error", err)
			http.Error(w, "signature", http.StatusNotAcceptable)
			return
		}
		raw := []byte(fields["body-mime"])
		if len(raw) == 0 {
			slog.WarnContext(r.Context(), "mail hook: inbound call carries no body-mime", "from", fields["from"], "subject", fields["subject"])
			w.WriteHeader(http.StatusOK)
			return
		}
		msg, err := netmail.ReadMessage(bytes.NewReader(raw))
		if err != nil {
			slog.WarnContext(r.Context(), "mail hook: mail not readable", "from", fields["from"], "subject", fields["subject"], "error", err)
			w.WriteHeader(http.StatusOK)
			return
		}
		from, subject, id := msg.Header.Get("From"), msg.Header.Get("Subject"), msg.Header.Get("Message-Id")
		if strings.Contains(strings.ToLower(from), "<forwarding-noreply@google.com>") {
			slog.InfoContext(r.Context(), "mail hook: forwarding confirmation", "from", from, "subject", subject)
		}
		channel, newsletter, ok := broadcastChannel(msg.Header.Get("List-Id"), from)
		if !ok {
			slog.InfoContext(r.Context(), "mail hook: mail not the community's", "id", id, "from", from, "subject", subject, "list", msg.Header.Get("List-Id"))
			w.WriteHeader(http.StatusOK)
			return
		}
		lines, _ := mail.SplitMessage(raw)
		if reason := vouch(lines, channel, newsletter); reason != "" {
			slog.WarnContext(r.Context(), "mail hook: mail not vouched for", "id", id, "from", from, "subject", subject, "reason", reason)
			w.WriteHeader(http.StatusOK)
			return
		}
		s.Hold()
		defer s.Release()
		if _, err := FileMail(r.Context(), s, pics, raw); err != nil {
			slog.ErrorContext(r.Context(), "mail hook: mail not filed", "id", id, "subject", subject, "error", err)
			http.Error(w, "not filed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
