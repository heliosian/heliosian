package id

import "strings"

const Alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

const Length = 13

func Parse(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) != Length {
		return "", false
	}
	for _, r := range s {
		if !strings.ContainsRune(Alphabet, r) {
			return "", false
		}
	}
	return s, true
}
