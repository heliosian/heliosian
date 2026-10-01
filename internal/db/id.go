package db

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	idAlphabet   = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	prefixLength = 3
	randomLength = 11
	IDLength     = prefixLength + randomLength
)

const (
	PersonPrefix         = "per"
	PersonEmailPrefix    = "eml"
	PersonPhotoPrefix    = "pho"
	PersonSettingPrefix  = "pst"
	BirthdayYearPrefix   = "bdy"
	SavedViewPrefix      = "svw"
	FeedTokenPrefix      = "fed"
	GroupPrefix          = "grp"
	GroupSourcePrefix    = "src"
	MemberPrefix         = "mem"
	RulePrefix           = "rul"
	GroupCategoryPrefix  = "gct"
	DocumentPrefix       = "doc"
	DocumentGroupPrefix  = "dgr"
	ReportPrefix         = "rpt"
	MessagePrefix        = "msg"
	RecipientPrefix      = "rcp"
	MailTokenPrefix      = "tok"
	SettingPrefix        = "set"
	CategoryPrefix       = "cat"
	CharityPrefix        = "chr"
	AppPrefix            = "app"
	WidgetPrefix         = "wdg"
	GeocodePrefix        = "geo"
	AliasPrefix          = "als"
	RedirectPrefix       = "rdr"
	InviteServicePrefix  = "isv"
	InviteTemplatePrefix = "itp"
	GreetingPrefix       = "grt"
)

var prefixes = map[string]bool{
	PersonPrefix: true, PersonEmailPrefix: true, PersonPhotoPrefix: true, PersonSettingPrefix: true,
	BirthdayYearPrefix: true, SavedViewPrefix: true, FeedTokenPrefix: true,
	GroupPrefix: true, GroupSourcePrefix: true, MemberPrefix: true,
	RulePrefix: true, GroupCategoryPrefix: true, DocumentPrefix: true, DocumentGroupPrefix: true,
	ReportPrefix: true, MessagePrefix: true, RecipientPrefix: true, MailTokenPrefix: true,
	SettingPrefix: true, CategoryPrefix: true, CharityPrefix: true, AppPrefix: true,
	WidgetPrefix: true, GeocodePrefix: true, AliasPrefix: true, RedirectPrefix: true,
	InviteServicePrefix: true, InviteTemplatePrefix: true, GreetingPrefix: true,
}

func Mint(prefix string, taken func(string) bool) string {
	if !prefixes[prefix] {
		panic(fmt.Sprintf("no id prefix %q", prefix))
	}
	for {
		s := prefix + random(randomLength)
		if !taken(s) {
			return s
		}
	}
}

func ParseID(s string) (prefix string, ok bool) {
	if len(s) != IDLength || !prefixes[s[:prefixLength]] {
		return "", false
	}
	for i := prefixLength; i < len(s); i++ {
		if strings.IndexByte(idAlphabet, s[i]) < 0 {
			return "", false
		}
	}
	return s[:prefixLength], true
}

func random(n int) string {
	out := make([]byte, n)
	buf := make([]byte, 1)
	for i := 0; i < n; {
		if _, err := rand.Read(buf); err != nil {
			panic(err)
		}
		if int(buf[0]) >= 256/len(idAlphabet)*len(idAlphabet) {
			continue
		}
		out[i] = idAlphabet[int(buf[0])%len(idAlphabet)]
		i++
	}
	return string(out)
}
