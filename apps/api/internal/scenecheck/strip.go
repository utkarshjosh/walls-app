package scenecheck

import "bytes"

// blankComments returns a copy of src with every comment replaced by spaces:
// HTML comments in markup, // and /* */ comments in scripts, and /* */
// comments in styles. Line breaks are kept and nothing moves, so an offset or
// line number in the result means the same place in the original file.
//
// The scan that follows must see everything a browser would run, so this
// function is deliberately cautious: whenever it is unsure whether something
// is a comment, it leaves the text in place. That can cause a false alarm
// (a forbidden word in an oddly placed comment), never a miss. To stay in
// step with the browser it follows the HTML parsing rules that decide where
// tags, comments and scripts really end.
//
// It also returns where each script's code lies (start and end offsets), for
// checks that treat code and markup differently.
func blankComments(src []byte) (clean []byte, scripts [][2]int) {
	out := bytes.Clone(src)
	n := len(src)
	i := 0
	for i < n {
		if src[i] != '<' {
			i++
			continue
		}
		rest := src[i:]
		switch {
		case bytes.HasPrefix(rest, []byte("<!--")):
			end := htmlCommentEnd(src, i)
			blank(out, i, end)
			i = end
		case hasPrefixFold(rest, "<![CDATA["):
			// Inside inline SVG this is literal text up to "]]>"; elsewhere
			// the browser ends it at the first ">". Skipping to "]]>" covers
			// both without ever hiding anything.
			if j := bytes.Index(rest, []byte("]]>")); j >= 0 {
				i += j + 3
			} else {
				i = after(src, i, '>')
			}
		case len(rest) > 1 && (rest[1] == '!' || rest[1] == '?'):
			// <!doctype …>, <?…> and other "bogus comments" end at the first
			// ">". They are skipped, not blanked, so a "<!--" inside one is
			// not mistaken for the start of a real comment.
			i = after(src, i, '>')
		case len(rest) > 2 && rest[1] == '/' && isLetter(rest[2]):
			i = tagEnd(src, i)
		case len(rest) > 1 && rest[1] == '/':
			i = after(src, i, '>')
		case len(rest) > 1 && isLetter(rest[1]):
			name := tagName(rest[1:])
			start := i
			i = tagEnd(src, i)
			// "<script/>" and "<style/>" mean different things in HTML (the
			// element is still open) and inside inline SVG (it is already
			// closed, so what follows is markup, not code). Rather than guess,
			// leave whatever follows completely un-blanked.
			selfClosed := i >= start+2 && src[i-2] == '/'
			// Elements whose content is not markup. A "<!--" inside them is
			// ordinary text to the browser, so it must not start a comment
			// here either.
			switch name {
			case "script":
				end := scriptEnd(src, i)
				if !selfClosed {
					blankScriptComments(src, out, i, end)
					scripts = append(scripts, [2]int{i, end})
				}
				i = end
			case "style":
				end := rawTextEnd(src, i, name)
				if !selfClosed {
					blankStyleComments(src, out, i, end)
				}
				i = end
			case "title", "textarea", "xmp", "noscript", "noembed", "noframes", "iframe":
				i = rawTextEnd(src, i, name)
			case "plaintext":
				i = n
			}
		default:
			i++
		}
	}
	return out, scripts
}

// blank overwrites out[a:b] with spaces, keeping line breaks.
func blank(out []byte, a, b int) {
	for k := a; k < b && k < len(out); k++ {
		if out[k] != '\n' && out[k] != '\r' {
			out[k] = ' '
		}
	}
}

func isLetter(c byte) bool { return c|0x20 >= 'a' && c|0x20 <= 'z' }

// isIdent reports whether c can be part of a JavaScript identifier.
func isIdent(c byte) bool {
	return isLetter(c) || (c >= '0' && c <= '9') || c == '_' || c == '$'
}

func hasPrefixFold(b []byte, prefix string) bool {
	return len(b) >= len(prefix) && bytes.EqualFold(b[:len(prefix)], []byte(prefix))
}

// after returns the index just past the next c at or after i, or len(src).
func after(src []byte, i int, c byte) int {
	if j := bytes.IndexByte(src[i:], c); j >= 0 {
		return i + j + 1
	}
	return len(src)
}

// tagName reads a lowercased tag name from the bytes after "<".
func tagName(b []byte) string {
	j := 0
	for j < len(b) && !isTagDelim(b[j]) {
		j++
	}
	return string(bytes.ToLower(b[:j]))
}

func isTagDelim(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == '/' || c == '>'
}

// tagEnd returns the index just past the ">" that closes the tag starting at
// i. A ">" inside a quoted attribute value does not close the tag.
func tagEnd(src []byte, i int) int {
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '>':
			return j + 1
		case '"', '\'':
			k := bytes.IndexByte(src[j+1:], src[j])
			if k < 0 {
				return len(src)
			}
			j += k + 1
		}
	}
	return len(src)
}

// htmlCommentEnd returns the index just past the comment that starts at i.
func htmlCommentEnd(src []byte, i int) int {
	start := i + 4
	rest := src[start:]
	switch {
	case bytes.HasPrefix(rest, []byte(">")): // "<!-->" is a complete, empty comment
		return start + 1
	case bytes.HasPrefix(rest, []byte("->")): // so is "<!--->"
		return start + 2
	}
	end := len(src)
	if j := bytes.Index(rest, []byte("-->")); j >= 0 {
		end = start + j + 3
	}
	if j := bytes.Index(rest, []byte("--!>")); j >= 0 && start+j+4 < end { // browsers accept this ending too
		end = start + j + 4
	}
	return end
}

// isTagStart reports whether b begins with the given "<name" or "</name"
// followed by something that ends a tag name.
func isTagStart(b []byte, open string) bool {
	return hasPrefixFold(b, open) && len(b) > len(open) && isTagDelim(b[len(open)])
}

// rawTextEnd returns the index of the "</name" that ends a raw-text element
// whose content starts at i, or len(src).
func rawTextEnd(src []byte, i int, name string) int {
	for j := i; j < len(src); j++ {
		if src[j] == '<' && isTagStart(src[j:], "</"+name) {
			return j
		}
	}
	return len(src)
}

// scriptEnd returns the index of the "</script" that really ends a script
// whose content starts at i. HTML has an old rule that matters here: after
// "<!--" and then "<script" inside a script, the next "</script>" does NOT
// end it. Getting this wrong would let code hide behind what looks like the
// end of the script.
func scriptEnd(src []byte, i int) int {
	const (
		plain = iota
		escaped
		doubleEscaped
	)
	state := plain
	for j := i; j < len(src); j++ {
		rest := src[j:]
		switch {
		case state == plain && bytes.HasPrefix(rest, []byte("<!--")):
			state = escaped
			j++ // continue from the dashes, so "<!-->" is seen to end the state at once
		case state != plain && bytes.HasPrefix(rest, []byte("-->")):
			state = plain
			j += 2
		case src[j] == '<' && isTagStart(rest, "</script"):
			if state != doubleEscaped {
				return j
			}
			state = escaped
		case state == escaped && src[j] == '<' && isTagStart(rest, "<script"):
			state = doubleEscaped
		}
	}
	return len(src)
}

// blankScriptComments blanks // and /* */ comments in src[a:b], treating the
// content as JavaScript (the same lexical rules also fit the GLSL shader
// source that scenes keep in script blocks). Strings, template literals and
// regular-expression literals are stepped over so a "//" inside one is not
// taken for a comment.
//
// Telling a regular expression from a division needs a full parser in rare
// cases; this uses the usual rule of thumb (a "/" starts a regular expression
// only where a value is expected).
func blankScriptComments(src, out []byte, a, b int) {
	var (
		templateDepths []int // brace depth to return to for each open `${`
		depth          int   // unclosed "{" in the current code section
		inTemplate     bool
		prev           byte   // last significant byte of code
		word           []byte // the identifier that ended at prev, if any
	)
	note := func(c byte) { prev = c }

	i := a
	for i < b {
		c := src[i]
		if inTemplate {
			switch {
			case c == '\\':
				i += 2
			case c == '`':
				inTemplate = false
				note('"')
				i++
			case c == '$' && i+1 < b && src[i+1] == '{':
				templateDepths = append(templateDepths, depth)
				depth, inTemplate = 0, false
				note('{')
				i += 2
			default:
				i++
			}
			continue
		}

		next := byte(0)
		if i+1 < b {
			next = src[i+1]
		}
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
		case c == '/' && next == '/':
			end := b
			if j := bytes.IndexByte(src[i:b], '\n'); j >= 0 {
				end = i + j
			}
			blank(out, i, end)
			i = end
		case c == '/' && next == '*':
			end := b
			if j := bytes.Index(src[i+2:b], []byte("*/")); j >= 0 {
				end = i + 2 + j + 2
			}
			blank(out, i, end)
			i = end
		case c == '"' || c == '\'':
			i = stringEnd(src, i, b)
			word = nil
			note('"')
		case c == '`':
			inTemplate = true
			word = nil
			i++
		case c == '/':
			if end, ok := regexEnd(src, i, b); ok && valueExpected(prev, word) {
				i = end
				word = nil
				note('"') // a finished literal is a value, like a string
				continue
			}
			word = nil
			note(c)
			i++
		case c == '{':
			depth++
			word = nil
			note(c)
			i++
		case c == '}':
			if depth == 0 && len(templateDepths) > 0 { // end of a `${ … }`
				depth = templateDepths[len(templateDepths)-1]
				templateDepths = templateDepths[:len(templateDepths)-1]
				inTemplate = true
				i++
				continue
			}
			if depth > 0 {
				depth--
			}
			word = nil
			note(c)
			i++
		case isIdent(c):
			j := i
			for j < b && isIdent(src[j]) {
				j++
			}
			word = src[i:j]
			prev = src[j-1]
			i = j
		default:
			word = nil
			note(c)
			i++
		}
	}
}

// stringEnd returns the index just past the quoted string starting at i. An
// unfinished string ends at the line break.
func stringEnd(src []byte, i, b int) int {
	quote := src[i]
	for j := i + 1; j < b; j++ {
		switch src[j] {
		case '\\':
			j++
		case quote:
			return j + 1
		case '\n':
			return j
		}
	}
	return b
}

// regexEnd looks for the end of a regular-expression literal starting at the
// "/" at i. It reports false if the line ends first, in which case the "/"
// was a division.
func regexEnd(src []byte, i, b int) (int, bool) {
	inClass := false
	for j := i + 1; j < b; j++ {
		switch c := src[j]; {
		case c == '\n':
			return 0, false
		case c == '\\':
			j++
		case inClass:
			inClass = c != ']'
		case c == '[':
			inClass = true
		case c == '/':
			j++
			for j < b && isIdent(src[j]) { // flags
				j++
			}
			return j, true
		}
	}
	return 0, false
}

// valueExpected reports whether the code so far ends somewhere a value may
// start (so a "/" begins a regular expression) rather than after a value (so
// it divides).
func valueExpected(prev byte, word []byte) bool {
	switch {
	case prev == 0:
		return true
	case word != nil:
		switch string(word) {
		case "return", "typeof", "instanceof", "in", "of", "new", "delete", "void", "throw", "case", "do", "else", "yield", "await":
			return true
		}
		return false
	case prev == ')' || prev == ']' || prev == '}' || prev == '"':
		return false
	case prev == '.':
		return false // "1./3." in shader code, or a syntax error
	}
	return true
}

// blankStyleComments blanks /* */ comments in the CSS in src[a:b]. CSS has no
// "//" comments, and a "/*" inside a string or an unquoted url(…) is not a
// comment either.
func blankStyleComments(src, out []byte, a, b int) {
	i := a
	for i < b {
		c := src[i]
		switch {
		case c == '"' || c == '\'':
			i = stringEnd(src, i, b)
		case c == '/' && i+1 < b && src[i+1] == '*':
			end := b
			if j := bytes.Index(src[i+2:b], []byte("*/")); j >= 0 {
				end = i + 2 + j + 2
			}
			blank(out, i, end)
			i = end
		case (c == 'u' || c == 'U') && hasPrefixFold(src[i:b], "url("):
			j := i + 4
			for j < b && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n') {
				j++
			}
			if j < b && (src[j] == '"' || src[j] == '\'') {
				i = j // a quoted URL is an ordinary string
				continue
			}
			if k := bytes.IndexByte(src[j:b], ')'); k >= 0 {
				i = j + k + 1
			} else {
				i = b
			}
		default:
			i++
		}
	}
}
