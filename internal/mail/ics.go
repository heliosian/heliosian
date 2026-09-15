package mail

import "strings"

// The pieces of a calendar invite that every app writes the same way.

// Address is the bare address inside "Name <address>".
func Address(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		return strings.TrimSuffix(strings.TrimSpace(from[i+1:]), ">")
	}
	return strings.TrimSpace(from)
}

// ICSEscape and ICSFold write text the way RFC 5545 wants it: commas,
// semicolons, backslashes and newlines escaped, lines folded at 75 octets.
func ICSEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)
	return r.Replace(s)
}

func ICSFold(line string) string {
	var b strings.Builder
	count := 0
	for _, r := range line {
		size := len(string(r))
		if count+size > 75 {
			b.WriteString("\r\n ")
			count = 1
		}
		b.WriteRune(r)
		count += size
	}
	return b.String()
}
