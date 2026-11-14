// Package sqlsplit splits a PostgreSQL SQL script (typically the output of
// pg_dump --schema-only) into statements. It understands comments, string
// literals, quoted identifiers and dollar-quoted bodies, so semicolons inside
// function bodies do not end a statement.
package sqlsplit

import "strings"

// Statement is one SQL statement from the script.
type Statement struct {
	// Text is the statement without its trailing semicolon. Comments are
	// replaced by spaces; newlines are kept so offsets map to lines.
	Text string
	// Outer is like Text, but the contents of dollar-quoted bodies are
	// removed. Use it to look at a statement's clauses without matching
	// code inside a function body.
	Outer string
	// Line is the 1-based line on which the statement starts.
	Line int
}

// LineOf returns the 1-based source line of byte offset off within s.Text.
func (s Statement) LineOf(off int) int {
	if off > len(s.Text) {
		off = len(s.Text)
	}
	return s.Line + strings.Count(s.Text[:off], "\n")
}

// Split splits src into statements.
func Split(src string) []Statement {
	s := &splitter{src: src, line: 1}
	s.run()
	return s.out
}

type splitter struct {
	src         string
	i, line     int
	text, outer strings.Builder
	started     bool
	startLine   int
	out         []Statement
}

func isIdent(b byte) bool {
	return b == '_' || b == '$' || b >= 0x80 ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// put appends one byte to the current statement. Leading whitespace is
// skipped; toOuter controls whether the byte is also part of Outer.
func (s *splitter) put(b byte, toOuter bool) {
	if !s.started {
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\f' {
			if b == '\n' {
				s.line++
			}
			return
		}
		s.started = true
		s.startLine = s.line
	}
	s.text.WriteByte(b)
	if toOuter {
		s.outer.WriteByte(b)
	}
	if b == '\n' {
		s.line++
	}
}

func (s *splitter) peek(k int) byte {
	if s.i+k < len(s.src) {
		return s.src[s.i+k]
	}
	return 0
}

func (s *splitter) emit() {
	if s.started {
		t := strings.TrimRight(s.text.String(), " \t\r\n\f")
		if strings.TrimSpace(t) != "" {
			s.out = append(s.out, Statement{
				Text:  t,
				Outer: strings.TrimRight(s.outer.String(), " \t\r\n\f"),
				Line:  s.startLine,
			})
		}
	}
	s.text.Reset()
	s.outer.Reset()
	s.started = false
}

// dollarTag returns the dollar-quote tag ("$$" or "$name$") starting at s.i.
func (s *splitter) dollarTag() (string, bool) {
	j := s.i + 1
	for j < len(s.src) && isIdent(s.src[j]) && s.src[j] != '$' {
		if j == s.i+1 && s.src[j] >= '0' && s.src[j] <= '9' {
			return "", false // $1 is a parameter, not a tag
		}
		j++
	}
	if j < len(s.src) && s.src[j] == '$' {
		return s.src[s.i : j+1], true
	}
	return "", false
}

func (s *splitter) run() {
	n := len(s.src)
	for s.i < n {
		c := s.src[s.i]
		switch {
		case c == '-' && s.peek(1) == '-':
			for s.i < n && s.src[s.i] != '\n' {
				s.put(' ', true)
				s.i++
			}
		case c == '/' && s.peek(1) == '*':
			depth := 0
			for s.i < n {
				if s.src[s.i] == '/' && s.peek(1) == '*' {
					depth++
					s.put(' ', true)
					s.put(' ', true)
					s.i += 2
					continue
				}
				if s.src[s.i] == '*' && s.peek(1) == '/' {
					depth--
					s.put(' ', true)
					s.put(' ', true)
					s.i += 2
					if depth == 0 {
						break
					}
					continue
				}
				if s.src[s.i] == '\n' {
					s.put('\n', true)
				} else {
					s.put(' ', true)
				}
				s.i++
			}
		case c == '\'':
			escape := s.i > 0 && (s.src[s.i-1] == 'E' || s.src[s.i-1] == 'e') &&
				(s.i < 2 || !isIdent(s.src[s.i-2]))
			s.quoted('\'', escape)
		case c == '"':
			s.quoted('"', false)
		case c == '$' && (s.i == 0 || !isIdent(s.src[s.i-1])):
			tag, ok := s.dollarTag()
			if !ok {
				s.put(c, true)
				s.i++
				continue
			}
			for k := 0; k < len(tag); k++ {
				s.put(tag[k], true)
			}
			s.i += len(tag)
			end := strings.Index(s.src[s.i:], tag)
			if end < 0 {
				end = n - s.i // unterminated body: consume the rest
			}
			for k := 0; k < end; k++ {
				s.put(s.src[s.i+k], false)
			}
			s.i += end
			if s.i < n {
				for k := 0; k < len(tag); k++ {
					s.put(tag[k], true)
				}
				s.i += len(tag)
			}
		case c == ';':
			s.i++
			s.emit()
		default:
			s.put(c, true)
			s.i++
		}
	}
	s.emit()
}

// quoted consumes a quoted literal or identifier starting at s.i.
func (s *splitter) quoted(q byte, backslashEscapes bool) {
	n := len(s.src)
	s.put(q, true)
	s.i++
	for s.i < n {
		ch := s.src[s.i]
		if backslashEscapes && ch == '\\' && s.i+1 < n {
			s.put(ch, true)
			s.put(s.src[s.i+1], true)
			s.i += 2
			continue
		}
		if ch == q {
			if s.peek(1) == q { // doubled quote
				s.put(ch, true)
				s.put(ch, true)
				s.i += 2
				continue
			}
			s.put(ch, true)
			s.i++
			return
		}
		s.put(ch, true)
		s.i++
	}
}
