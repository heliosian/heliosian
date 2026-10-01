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
	PersonPrefix       = "per"
	GroupPrefix        = "grp"
	PurchasePrefix     = "pur"
	DocumentPrefix     = "doc"
	MessagePrefix      = "msg"
	RecipientPrefix    = "tok"
	FeedPrefix         = "fed"
	ReportPrefix       = "rpt"
	CategoryPrefix     = "cat"
	CharityPrefix      = "chr"
	BirthdayYearPrefix = "bdy"
	GreetingPrefix     = "grt"
)

var prefixes = map[string]bool{
	PersonPrefix:       true,
	GroupPrefix:        true,
	PurchasePrefix:     true,
	DocumentPrefix:     true,
	MessagePrefix:      true,
	RecipientPrefix:    true,
	FeedPrefix:         true,
	ReportPrefix:       true,
	CategoryPrefix:     true,
	CharityPrefix:      true,
	BirthdayYearPrefix: true,
	GreetingPrefix:     true,
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
