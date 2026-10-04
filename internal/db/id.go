package db

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
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
	PersonPrefix          = "per"
	PersonEmailPrefix     = "eml"
	PhotoPrefix           = "pho"
	PersonSettingPrefix   = "pst"
	BirthdayYearPrefix    = "bdy"
	CollectionPrefix      = "col"
	FeedTokenPrefix       = "fed"
	GroupPrefix           = "grp"
	GroupSourcePrefix     = "src"
	MemberPrefix          = "mem"
	EffectiveMemberPrefix = "efm"
	RulePrefix            = "rul"
	DocumentPrefix        = "doc"
	DocumentGroupPrefix   = "dgr"
	InboxPrefix           = "inb"
	ReportPrefix          = "rpt"
	MessagePrefix         = "msg"
	RecipientPrefix       = "rcp"
	MailTokenPrefix       = "tok"
	SettingPrefix         = "set"
	CharityPrefix         = "chr"
	AppPrefix             = "app"
	WidgetPrefix          = "wdg"
	GeocodePrefix         = "geo"
	AliasPrefix           = "als"
	RedirectPrefix        = "rdr"
	InviteServicePrefix   = "isv"
	InviteTemplatePrefix  = "itp"
	GreetingPrefix        = "grt"
	ChangePrefix          = "chg"
)

var prefixes = map[string]bool{
	PersonPrefix: true, PersonEmailPrefix: true, PhotoPrefix: true, PersonSettingPrefix: true,
	BirthdayYearPrefix: true, CollectionPrefix: true, FeedTokenPrefix: true,
	GroupPrefix: true, GroupSourcePrefix: true, MemberPrefix: true, EffectiveMemberPrefix: true,
	RulePrefix: true, DocumentPrefix: true, DocumentGroupPrefix: true, InboxPrefix: true,
	ReportPrefix: true, MessagePrefix: true, RecipientPrefix: true, MailTokenPrefix: true,
	SettingPrefix: true, CharityPrefix: true, AppPrefix: true,
	WidgetPrefix: true, GeocodePrefix: true, AliasPrefix: true, RedirectPrefix: true,
	InviteServicePrefix: true, InviteTemplatePrefix: true, GreetingPrefix: true,
	ChangePrefix: true,
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

func Derive(prefix string, parts ...string) string {
	if !prefixes[prefix] {
		panic(fmt.Sprintf("no id prefix %q", prefix))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	n := binary.BigEndian.Uint64(sum[:8])
	out := make([]byte, randomLength)
	for i := range out {
		out[i] = idAlphabet[n%uint64(len(idAlphabet))]
		n /= uint64(len(idAlphabet))
	}
	return prefix + string(out)
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
