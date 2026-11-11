package records

import (
	"strings"
	"unicode"
)

// NormalizeEmail tidies an address typed into a form and reports whether it
// looks like one that can receive mail. It checks the shape only; whether the
// mailbox exists is found out by writing to it.
//
// The result is lower-case, so the same person signing up twice with
// different capitals is one entry.
func NormalizeEmail(raw string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if len(s) < 6 || len(s) > 254 {
		return "", false
	}
	at := strings.LastIndexByte(s, '@')
	if at < 1 {
		return "", false
	}
	local, domain := s[:at], s[at+1:]
	if len(local) > 64 || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return "", false
	}
	for _, r := range local {
		// Quoted local parts ("john smith"@…) are legal and never seen in
		// practice; refusing them also keeps punctuation that is awkward in
		// a spreadsheet or a mail client out of the list.
		if r <= ' ' || r == 0x7f || unicode.IsSpace(r) || strings.ContainsRune(`"(),:;<>[\]@`, r) {
			return "", false
		}
	}

	labels := strings.Split(domain, ".")
	if len(domain) > 253 || len(labels) < 2 {
		return "", false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", false
		}
		for _, r := range label {
			if r != '-' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				return "", false
			}
		}
	}
	// The last label is a real top-level name: at least two characters, not a number.
	tld := labels[len(labels)-1]
	if len(tld) < 2 || strings.IndexFunc(tld, unicode.IsLetter) < 0 {
		return "", false
	}
	return s, true
}
