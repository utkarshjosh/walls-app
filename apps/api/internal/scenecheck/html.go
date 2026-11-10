package scenecheck

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// Policy is the one Content-Security-Policy a scene may carry. With it the
// page can run its own inline script and styles and use embedded data:
// pictures and fonts, and can load or send nothing else.
const Policy = `default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; base-uri 'none'; form-action 'none'`

// PolicyLine is the policy tag as the standard prints it. Other spellings of
// the same tag pass too (see checkPolicy); this is the one to copy.
const PolicyLine = `<meta http-equiv="Content-Security-Policy" content="` + Policy + `">`

// Problem is one reason a scene fails the gate.
type Problem struct {
	File    string // "scene.html", "poster.jpg", "meta.json", or "" for the folder itself
	Line    int    // line in the file, or 0 when a line makes no sense
	Code    string // short stable name for the rule, e.g. "forbidden"
	Message string
}

// Where is the "file:line" label for a problem.
func (p Problem) Where() string {
	switch {
	case p.File == "":
		return "folder"
	case p.Line > 0:
		return fmt.Sprintf("%s:%d", p.File, p.Line)
	}
	return p.File
}

func (p Problem) String() string { return p.Where() + ": " + p.Message }

// Words that are forbidden wherever they stand as a whole identifier.
var forbiddenWords = map[string]bool{
	"fetch": true, "XMLHttpRequest": true, "WebSocket": true, "EventSource": true, "sendBeacon": true,
	"importScripts": true, "eval": true, "localStorage": true, "sessionStorage": true, "indexedDB": true,
	"Notification": true, "postMessage": true,
}

// allowedNamespaces are the three fixed names a scene needs in order to build
// SVG from script (a clock face, say). They look like web addresses but are
// labels that nothing ever fetches. Only these exact strings are let through;
// anything longer that merely starts with one is an ordinary address.
var allowedNamespaces = []string{
	"http://www.w3.org/2000/svg",
	"http://www.w3.org/1999/xlink",
	"http://www.w3.org/1999/xhtml",
}

// The only navigator properties a scene may read.
var allowedNavigator = map[string]bool{"hardwareConcurrency": true, "userAgent": true}

// Elements a scene may not contain at all.
var forbiddenTags = []string{"link", "iframe", "object", "embed", "form", "base"}

// CheckHTML checks the text of a scene.html against the standard: the policy
// line, the forbidden constructs, and the two things every scene must do
// (read location.hash, handle paused). It returns every problem it finds.
//
// This is a check of the text, not a JavaScript interpreter. It catches a
// scene that plainly uses something it must not; it cannot catch code that
// goes out of its way to disguise itself (building "fetch" from pieces, say).
// The policy line is what actually stops such code at run time, and a person
// reads every scene before it is published.
func CheckHTML(src []byte) []Problem {
	clean, scripts := blankComments(src)
	lines := newLineIndex(src)
	toks := tokenize(clean)
	var problems []Problem
	add := func(code string, pos int, format string, args ...any) {
		line := 0
		if pos >= 0 {
			line = lines.of(pos)
		}
		problems = append(problems, Problem{File: "scene.html", Line: line, Code: code, Message: fmt.Sprintf(format, args...)})
	}

	checkPolicy(src, lines, add)

	// --- Forbidden constructs ------------------------------------------------
	found := map[string][]int{} // construct → lines, in order
	var order []string
	forbid := func(construct string, pos int) {
		if _, seen := found[construct]; !seen {
			order = append(order, construct)
		}
		line := lines.of(pos)
		if l := found[construct]; len(l) == 0 || l[len(l)-1] != line {
			found[construct] = append(found[construct], line)
		}
	}

	for _, name := range forbiddenTags {
		for at := findTag(clean, name, 0); at >= 0; at = findTag(clean, name, at+1) {
			forbid("<"+name+">", at)
		}
	}
	// A script that loads from elsewhere (SVG scripts do it through href),
	// and links out of the page.
	for at := findTag(clean, "script", 0); at >= 0; at = findTag(clean, "script", at+1) {
		if tag, ok := parseStartTag(clean, at); ok && (tag.has("src") || tag.has("href") || tag.has("xlink:href")) {
			forbid("<script src>", at)
		}
	}
	for at := findTag(clean, "a", 0); at >= 0; at = findTag(clean, "a", at+1) {
		if tag, ok := parseStartTag(clean, at); ok && (tag.has("href") || tag.has("xlink:href")) {
			forbid("<a href>", at)
		}
	}

	readsHash, mentionsPaused := false, false
	for i, t := range toks {
		if !t.ident {
			continue
		}
		w := toks.word(clean, i)
		switch {
		case forbiddenWords[w]:
			forbid(w, t.pos)
		case w == "paused":
			mentionsPaused = true
		case w == "import" && toks.punct(clean, i+1, '('):
			forbid("import(", t.pos)
		case w == "Function" && (toks.word(clean, i-1) == "new" || toks.punct(clean, i+1, '(')):
			forbid("new Function", t.pos)
		case w == "cookie" && toks.owner(clean, i) == "document":
			forbid("document.cookie", t.pos)
		case w == "open" && toks.owner(clean, i) == "window":
			forbid("window.open", t.pos)
		case w == "navigator":
			if m := toks.member(clean, i); m >= 0 {
				if prop := toks.word(clean, m); !allowedNavigator[prop] {
					forbid("navigator."+prop, t.pos)
				}
			} else if toks.punct(clean, i+1, '[') {
				forbid("navigator[…]", t.pos)
			}
		case w == "top" || w == "parent":
			switch owner := toks.owner(clean, i); {
			case owner == "window" || owner == "self" || owner == "globalThis":
				forbid("window."+w, t.pos)
			case owner == "" && !toks.punct(clean, i-1, '.') && toks.member(clean, i) >= 0:
				forbid(w+".", t.pos)
			}
		case w == "location":
			// Follow the property chain: location.hash.slice …
			last, props := i, []string(nil)
			for {
				m := toks.member(clean, last)
				if m < 0 {
					break
				}
				props = append(props, toks.word(clean, m))
				last = m
			}
			if len(props) > 0 && props[0] == "hash" {
				readsHash = true
			}
			target := "location"
			if len(props) > 0 {
				target += "." + props[0]
			}
			after := last + 1
			if len(props) == 0 && toks.punct(clean, after, '[') { // location["href"] = …
				after = toks.closing(clean, after) + 1
				target = "location[…]"
			}
			switch {
			case len(props) == 1 && (props[0] == "assign" || props[0] == "replace") && toks.punct(clean, after, '('):
				forbid("location."+props[0]+"()", t.pos)
			case toks.assigns(clean, after):
				forbid("assignment to "+target, t.pos)
			}
		default:
			if lw := strings.ToLower(w); (lw == "http" || lw == "https" || lw == "ws" || lw == "wss") &&
				toks.punct(clean, i+1, ':') && toks[i+1].pos == t.end && !isNamespaceName(clean, t.pos, scripts) {
				forbid(lw+": URL", t.pos)
			}
		}
	}
	for _, construct := range order {
		at := found[construct]
		msg := "forbidden: " + construct
		if construct == "top." || construct == "parent." {
			msg += " (reaches the window above; if this is your own variable, rename it)"
		}
		if len(at) > 1 {
			rest := at[1:]
			more := ""
			if len(rest) > 5 {
				rest, more = rest[:5], ", …"
			}
			var s []string
			for _, l := range rest {
				s = append(s, fmt.Sprint(l))
			}
			lineWord := "line"
			if len(s) > 1 {
				lineWord = "lines"
			}
			msg += fmt.Sprintf(" (also on %s %s%s)", lineWord, strings.Join(s, ", "), more)
		}
		problems = append(problems, Problem{File: "scene.html", Line: at[0], Code: "forbidden", Message: msg})
	}

	// --- Things every scene must do ---------------------------------------------
	if !readsHash {
		add("no-location-hash", -1, "it never reads location.hash, which is how the app tells a scene its size and position")
	}
	if !mentionsPaused {
		add("no-paused", -1, "it never handles paused, so the app could not freeze it to save energy")
	}

	sort.SliceStable(problems, func(a, b int) bool {
		la, lb := problems[a].Line, problems[b].Line
		if (la == 0) != (lb == 0) {
			return lb == 0 // problems with a line first, whole-file ones after
		}
		return la < lb
	})
	return problems
}

// isNamespaceName reports whether the text at pos is one of the allowed
// namespace names standing exactly on its own, in one of the two places such
// a name belongs:
//
//   - in script, as a whole quoted string: "http://www.w3.org/2000/svg";
//   - in markup, as the whole value of an xmlns or xmlns:xlink attribute,
//     quoted or not (this also covers an <svg> written inside a string or a
//     data: picture).
//
// So src="http://www.w3.org/2000/svg" on an image is still an address, and
// so is the same text with anything after it.
func isNamespaceName(text []byte, pos int, scripts [][2]int) bool {
	for _, name := range allowedNamespaces {
		if !bytes.HasPrefix(text[pos:], []byte(name)) {
			continue
		}
		end := pos + len(name)
		at := func(i int) byte {
			if i < 0 || i >= len(text) {
				return 0
			}
			return text[i]
		}
		before, after := at(pos-1), at(end)
		quoted := (before == '"' || before == '\'' || before == '`') && after == before
		// The same quote written with a backslash on both sides, as in
		// "<svg xmlns=\"…\">".
		escaped := (before == '"' || before == '\'') && at(pos-2) == '\\' && after == '\\' && at(end+1) == before

		inScript := false
		for _, s := range scripts {
			inScript = inScript || (pos >= s[0] && pos < s[1])
		}
		if inScript && (quoted || escaped) {
			return true
		}

		// An attribute value: find the "=" before it and the name before that.
		k := pos - 1
		switch {
		case quoted:
			k = pos - 2
		case escaped:
			k = pos - 3
		case after == '>' || isSpace(after):
		default:
			return false
		}
		for k >= 0 && isSpace(text[k]) {
			k--
		}
		if at(k) != '=' {
			return false
		}
		k--
		for k >= 0 && isSpace(text[k]) {
			k--
		}
		for _, attr := range []string{"xmlns:xlink", "xmlns"} {
			start := k + 1 - len(attr)
			if start >= 0 && bytes.EqualFold(text[start:k+1], []byte(attr)) {
				// It must be the whole attribute name, not the tail of another.
				if c := at(start - 1); !isIdent(c) && c != '-' && c != ':' && c != '.' {
					return true
				}
			}
		}
		return false
	}
	return false
}

// checkPolicy applies the rules for the policy line. They are the same rules
// the Mac app applies when a scene arrives (apps/desktop/src-tauri/src/scene.rs),
// reading tags the same way, so the gate cannot pass a scene the app would
// then refuse:
//
//  1. exactly one <meta http-equiv="Content-Security-Policy"> anywhere in the
//     text, counting ones inside comments, templates and strings;
//  2. its content is the standard's, byte for byte;
//  3. the tag has only http-equiv and content, once each (a browser reads the
//     first of a repeated attribute, so a decoy must fail). Letter case,
//     attribute order and quoting are free;
//  4. nothing comes before it except the doctype, <html>, <head> and
//     <meta charset="utf-8">. A browser applies a policy only to what follows
//     it, and ignores it entirely once earlier content has pushed the page
//     out of <head>;
//  5. every byte up to the end of the tag is plain ASCII, so no choice of
//     character encoding can make a browser read those bytes differently.
func checkPolicy(src []byte, lines lineIndex, add func(code string, pos int, format string, args ...any)) {
	start := 0
	if bytes.HasPrefix(src, []byte("\xEF\xBB\xBF")) { // a UTF-8 byte-order mark may open the file
		start = 3
	}

	type found struct {
		at  int
		tag startTag
	}
	var policies []found
	for at := findTag(src, "meta", 0); at >= 0; at = findTag(src, "meta", at+1) {
		if tag, ok := parseStartTag(src, at); ok {
			if v, has := tag.attr("http-equiv"); has && strings.EqualFold(v, "content-security-policy") {
				policies = append(policies, found{at, tag})
			}
		}
	}
	if len(policies) == 0 {
		add("policy-missing", -1, "the Content-Security-Policy line is missing; it must be the first thing in <head> after the charset: %s", PolicyLine)
		return
	}
	if len(policies) > 1 {
		var at []string
		for _, p := range policies {
			at = append(at, fmt.Sprint(lines.of(p.at)))
		}
		add("policy-multiple", policies[1].at, "there are %d Content-Security-Policy tags (lines %s); exactly one is allowed anywhere in the file, comments and strings included",
			len(policies), strings.Join(at, ", "))
	}

	p := policies[0]
	var extra []string
	for _, a := range p.tag.attrs {
		if a.name != "http-equiv" && a.name != "content" {
			extra = append(extra, a.name)
		}
	}
	content, _ := p.tag.attr("content")
	switch {
	case p.tag.repeated:
		add("policy-wrong", p.at, "the policy tag repeats an attribute; a browser reads only the first, so http-equiv and content must each appear once")
	case len(extra) > 0:
		add("policy-wrong", p.at, "the policy tag has an extra attribute (%s); only http-equiv and content are allowed", strings.Join(extra, ", "))
	case content != Policy:
		add("policy-wrong", p.at, "the policy text differs from the standard's; content must be exactly: %s", Policy)
	}

	if script := findTag(src, "script", start); script >= 0 && script < p.at {
		add("policy-order", script, "a <script> comes before the policy line (line %d); the policy must come before any script", lines.of(p.at))
	} else if pos, what := beforePolicy(src, start, p.at); pos >= 0 {
		add("policy-place", pos, "%s comes before the policy line (line %d); only the doctype, <html>, <head> and <meta charset=\"utf-8\"> may come first, or a browser may ignore the policy",
			what, lines.of(p.at))
	}
	for i := start; i < p.tag.end; i++ {
		if c := src[i]; !(c >= 0x20 && c <= 0x7E) && c != '\t' && c != '\n' && c != '\r' && c != '\f' {
			add("policy-place", i, "a byte that is not plain ASCII (0x%02X) comes before the end of the policy line; everything up to there must be plain ASCII", c)
			break
		}
	}
}

// beforePolicy returns the position and a description of the first thing in
// src[from:to] that may not come before the policy, or -1 if there is none.
func beforePolicy(src []byte, from, to int) (int, string) {
	at := from
	for {
		for at < to && isSpace(src[at]) {
			at++
		}
		if at >= to {
			return -1, ""
		}
		if hasPrefixFold(src[at:to], "<!doctype") {
			// A doctype runs to the next ">" and holds no markup of its own.
			end := bytes.IndexByte(src[at:to], '>')
			if end < 0 || bytes.IndexByte(src[at+1:at+end], '<') >= 0 {
				return at, "a malformed doctype"
			}
			at += end + 1
			continue
		}
		if bytes.HasPrefix(src[at:to], []byte("<!--")) {
			return at, "a comment"
		}
		tag, ok := parseStartTag(src, at)
		if !ok {
			if src[at] == '<' {
				return at, "markup that is not an opening tag"
			}
			return at, "text"
		}
		allowed := !tag.repeated && tag.end <= to
		switch tag.name {
		case "html", "head":
		case "meta":
			charset, has := tag.attr("charset")
			allowed = allowed && has && len(tag.attrs) == 1 &&
				(strings.EqualFold(charset, "utf-8") || strings.EqualFold(charset, "utf8"))
		default:
			allowed = false
		}
		if !allowed {
			return at, "<" + tag.name + ">"
		}
		at = tag.end
	}
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r' }

// findTag returns the index of the first "<name" at or after from that really
// is that tag (the name is followed by white space, "/" or ">"), ignoring
// letter case, or -1.
func findTag(src []byte, name string, from int) int {
	for at := from; at+1+len(name) < len(src); at++ {
		if src[at] == '<' && hasPrefixFold(src[at+1:], name) {
			if c := src[at+1+len(name)]; isSpace(c) || c == '/' || c == '>' {
				return at
			}
		}
	}
	return -1
}

// startTag is an opening tag as a browser reads it.
type startTag struct {
	name     string // lowercase
	attrs    []tagAttr
	repeated bool // some attribute name appeared more than once
	end      int  // index just past the closing ">"
}

type tagAttr struct {
	name  string // lowercase
	value string // exactly as written, without the quotes
}

// attr returns the first attribute of that name, which is the one a browser
// uses.
func (t startTag) attr(name string) (string, bool) {
	for _, a := range t.attrs {
		if a.name == name {
			return a.value, true
		}
	}
	return "", false
}

func (t startTag) has(name string) bool {
	_, ok := t.attr(name)
	return ok
}

// parseStartTag reads the opening tag at src[at] == '<', following the HTML
// tokenizer's rules for where attribute names and values begin and end. It
// reports false if this is not an opening tag or the text ends before its
// ">".
func parseStartTag(src []byte, at int) (startTag, bool) {
	n := len(src)
	if at+1 >= n || src[at] != '<' || !isLetter(src[at+1]) {
		return startTag{}, false
	}
	cur := at + 1
	for cur < n && src[cur] != '/' && src[cur] != '>' && !isSpace(src[cur]) {
		cur++
	}
	tag := startTag{name: strings.ToLower(string(src[at+1 : cur]))}
	for {
		// Between attributes: white space and stray slashes are skipped.
		for cur < n && (isSpace(src[cur]) || src[cur] == '/') {
			cur++
		}
		if cur >= n {
			return startTag{}, false
		}
		if src[cur] == '>' {
			tag.end = cur + 1
			return tag, true
		}
		nameStart := cur
		cur++ // a name may even begin with "=", as in the tokenizer
		for cur < n && src[cur] != '/' && src[cur] != '>' && src[cur] != '=' && !isSpace(src[cur]) {
			cur++
		}
		name := strings.ToLower(string(src[nameStart:cur]))
		for cur < n && isSpace(src[cur]) {
			cur++
		}
		value := ""
		if cur < n && src[cur] == '=' {
			cur++
			for cur < n && isSpace(src[cur]) {
				cur++
			}
			if cur >= n {
				return startTag{}, false
			}
			switch c := src[cur]; {
			case c == '"' || c == '\'':
				valueStart := cur + 1
				end := bytes.IndexByte(src[valueStart:], c)
				if end < 0 {
					return startTag{}, false
				}
				value = string(src[valueStart : valueStart+end])
				cur = valueStart + end + 1
			case c == '>':
			default:
				valueStart := cur
				for cur < n && src[cur] != '>' && !isSpace(src[cur]) {
					cur++
				}
				value = string(src[valueStart:cur])
			}
		}
		if tag.has(name) {
			tag.repeated = true
		} else {
			tag.attrs = append(tag.attrs, tagAttr{name, value})
		}
	}
}

// token is an identifier or a single punctuation character in the cleaned
// text.
type token struct {
	pos, end int
	ident    bool
}

type tokens []token

// tokenize splits the text into identifiers and punctuation. Only ASCII
// letters, digits, "_" and "$" form identifiers; anything else separates
// them. That errs on the side of flagging: an exotic identifier that merely
// ends in "fetch" may be reported, but a real fetch can never hide behind an
// unusual space character.
func tokenize(clean []byte) tokens {
	var toks tokens
	for i := 0; i < len(clean); {
		c := clean[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
		case isIdent(c):
			j := i
			for j < len(clean) && isIdent(clean[j]) {
				j++
			}
			toks = append(toks, token{i, j, !(c >= '0' && c <= '9')}) // a number is not an identifier
			i = j
		default:
			toks = append(toks, token{i, i + 1, false})
			i++
		}
	}
	return toks
}

// word returns the identifier at index i, or "" if there is none.
func (t tokens) word(clean []byte, i int) string {
	if i < 0 || i >= len(t) || !t[i].ident {
		return ""
	}
	return string(clean[t[i].pos:t[i].end])
}

// punct reports whether token i is the punctuation character c.
func (t tokens) punct(clean []byte, i int, c byte) bool {
	return i >= 0 && i < len(t) && !t[i].ident && t[i].end == t[i].pos+1 && clean[t[i].pos] == c
}

// member returns the index of the property name if token i is followed by
// ".name" or "?.name", else -1.
func (t tokens) member(clean []byte, i int) int {
	j := i + 1
	if t.punct(clean, j, '?') && t.punct(clean, j+1, '.') {
		j++
	}
	if t.punct(clean, j, '.') && t.word(clean, j+1) != "" {
		return j + 1
	}
	return -1
}

// owner returns the identifier that token i is a property of ("document" for
// the cookie in document.cookie), or "".
func (t tokens) owner(clean []byte, i int) string {
	j := i - 1
	if !t.punct(clean, j, '.') {
		return ""
	}
	j--
	if t.punct(clean, j, '?') {
		j--
	}
	return t.word(clean, j)
}

// closing returns the index of the "]" matching the "[" at i, or the last
// token if it is never closed.
func (t tokens) closing(clean []byte, i int) int {
	depth := 0
	for j := i; j < len(t); j++ {
		switch {
		case t.punct(clean, j, '['):
			depth++
		case t.punct(clean, j, ']'):
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(t) - 1
}

// assigns reports whether the tokens at i form an assignment operator: "="
// (but not "==" or "=>"), or a compound one such as "+=".
func (t tokens) assigns(clean []byte, i int) bool {
	if t.punct(clean, i, '=') {
		return !t.punct(clean, i+1, '=') && !t.punct(clean, i+1, '>')
	}
	if i >= 0 && i+1 < len(t) && !t[i].ident && strings.IndexByte("+-*/%&|^", clean[t[i].pos]) >= 0 {
		return t.punct(clean, i+1, '=') && t[i+1].pos == t[i].end && !t.punct(clean, i+2, '=')
	}
	return false
}

// lineIndex turns a byte offset into a 1-based line number.
type lineIndex []int // offsets of each line start

func newLineIndex(src []byte) lineIndex {
	idx := lineIndex{0}
	for i, c := range src {
		if c == '\n' {
			idx = append(idx, i+1)
		}
	}
	return idx
}

func (l lineIndex) of(pos int) int {
	return sort.Search(len(l), func(i int) bool { return l[i] > pos })
}
