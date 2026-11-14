// Package check runs catalog entries against a schema dump and a
// postgresql.conf and produces findings.
package check

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ltchurik/pgupcheck/catalog"
	"github.com/ltchurik/pgupcheck/internal/sqlsplit"
)

// Finding is one detected problem.
type Finding struct {
	Rule     catalog.Entry
	File     string // empty for findings that apply to every cluster
	Line     int
	Evidence string
}

// Input holds the files to analyze. Either may be empty.
type Input struct {
	SchemaFile, Schema string
	ConfFile, Conf     string
}

type builtinFunc func(rule catalog.Entry, stmts []sqlsplit.Statement, file string) []Finding

// Builtins are detectors implemented in Go, referenced from the catalog
// by detect.builtin.
var Builtins = map[string]builtinFunc{
	"postfix_operator":       postfixOperator,
	"index_func_search_path": indexFuncSearchPath,
}

// Run checks rules against the input and returns findings sorted by
// severity, version, file and line.
func Run(rules []catalog.Entry, in Input) ([]Finding, error) {
	stmts := sqlsplit.Split(in.Schema)
	conf := parseConf(in.Conf)
	var out []Finding
	for _, r := range rules {
		d := r.Detect
		if d.Always {
			out = append(out, Finding{Rule: r, Evidence: fmt.Sprintf("applies to every cluster upgraded to PostgreSQL %d or later", r.IntroducedIn)})
		}
		for _, cm := range d.Conf {
			fs, err := confFindings(r, cm, conf, in.ConfFile, stmts, in.SchemaFile)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", r.ID, err)
			}
			out = append(out, fs...)
		}
		for _, expr := range d.SQLRegex {
			re, err := regexp.Compile("(?is)" + expr)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", r.ID, err)
			}
			for _, s := range stmts {
				for _, m := range re.FindAllStringIndex(s.Text, -1) {
					out = append(out, Finding{Rule: r, File: in.SchemaFile, Line: s.LineOf(m[0]), Evidence: evidence(s.Text[m[0]:m[1]])})
				}
			}
		}
		if d.Builtin != "" {
			fn, ok := Builtins[d.Builtin]
			if !ok {
				return nil, fmt.Errorf("%s: unknown builtin detector %q", r.ID, d.Builtin)
			}
			out = append(out, fn(r, stmts, in.SchemaFile)...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Rule.Severity.Rank() != b.Rule.Severity.Rank() {
			return a.Rule.Severity.Rank() < b.Rule.Severity.Rank()
		}
		if a.Rule.IntroducedIn != b.Rule.IntroducedIn {
			return a.Rule.IntroducedIn < b.Rule.IntroducedIn
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	return out, nil
}

var spaceRe = regexp.MustCompile(`\s+`)

// evidence collapses whitespace and truncates a matched snippet.
func evidence(s string) string {
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	if r := []rune(s); len(r) > 120 {
		s = string(r[:117]) + "…"
	}
	return s
}

type confLine struct {
	Name, Value, Raw string
	Line             int
}

// parseConf parses postgresql.conf syntax: name = value or name value,
// '#' comments outside quotes. include directives are not followed.
func parseConf(src string) []confLine {
	var out []confLine
	for i, raw := range strings.Split(src, "\n") {
		line := stripConfComment(raw)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, rest := line, ""
		if k := strings.IndexAny(line, "= \t"); k >= 0 {
			name, rest = line[:k], line[k:]
		}
		rest = strings.TrimSpace(rest)
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "="))
		out = append(out, confLine{
			Name:  strings.ToLower(strings.TrimSpace(name)),
			Value: unquote(rest),
			Raw:   line,
			Line:  i + 1,
		})
	}
	return out
}

func stripConfComment(s string) string {
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return s[:i]
			}
		}
	}
	return s
}

func unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '\'' && v[len(v)-1] == '\'' || v[0] == '"' && v[len(v)-1] == '"') {
		v = v[1 : len(v)-1]
	}
	return v
}

func confFindings(r catalog.Entry, cm catalog.ConfMatch, conf []confLine, confFile string, stmts []sqlsplit.Statement, schemaFile string) ([]Finding, error) {
	var valRe *regexp.Regexp
	if cm.ValueRegex != "" {
		var err error
		if valRe, err = regexp.Compile("(?i)" + cm.ValueRegex); err != nil {
			return nil, err
		}
	}
	param := strings.ToLower(cm.Param)
	var out []Finding
	for _, c := range conf {
		if c.Name == param && (valRe == nil || valRe.MatchString(c.Value)) {
			out = append(out, Finding{Rule: r, File: confFile, Line: c.Line, Evidence: evidence(c.Raw)})
		}
	}
	// ALTER SYSTEM / DATABASE / ROLE ... SET param = value in the schema.
	setRe := regexp.MustCompile(`(?i)\bset\s+"?` + regexp.QuoteMeta(param) + `"?\s*(?:=|\bto\b)\s*([^\n;]*)`)
	for _, s := range stmts {
		for _, m := range setRe.FindAllStringSubmatchIndex(s.Text, -1) {
			val := unquote(strings.TrimSpace(s.Text[m[2]:m[3]]))
			if valRe != nil && !valRe.MatchString(val) {
				continue
			}
			out = append(out, Finding{Rule: r, File: schemaFile, Line: s.LineOf(m[0]), Evidence: evidence(s.Text[m[0]:m[1]])})
		}
	}
	return out, nil
}

var (
	createOperatorRe = regexp.MustCompile(`(?is)^\s*create\s+operator\s+(\S+)`)
	leftArgRe        = regexp.MustCompile(`(?i)\bleftarg\s*=`)
	rightArgRe       = regexp.MustCompile(`(?i)\brightarg\s*=`)
)

// postfixOperator flags CREATE OPERATOR statements with LEFTARG but no
// RIGHTARG (postfix operators).
func postfixOperator(r catalog.Entry, stmts []sqlsplit.Statement, file string) []Finding {
	var out []Finding
	for _, s := range stmts {
		m := createOperatorRe.FindStringSubmatch(s.Outer)
		if m == nil {
			continue
		}
		if w := strings.ToLower(m[1]); w == "class" || w == "family" {
			continue
		}
		if leftArgRe.MatchString(s.Outer) && !rightArgRe.MatchString(s.Outer) {
			out = append(out, Finding{Rule: r, File: file, Line: s.Line, Evidence: evidence("CREATE OPERATOR " + m[1] + " (LEFTARG only)")})
		}
	}
	return out
}

var (
	createFunctionRe = regexp.MustCompile(`(?is)^\s*create\s+(?:or\s+replace\s+)?function\s+(?:(?:"[^"]+"|[\w$]+)\.)?("[^"]+"|[\w$]+)\s*\(`)
	setSearchPathRe  = regexp.MustCompile(`(?i)\bset\s+search_path\b`)
	langCRe          = regexp.MustCompile(`(?i)\blanguage\s+'?(?:c|internal)'?\b`)
	indexOrMatviewRe = regexp.MustCompile(`(?is)^\s*create\s+(?:(?:unique\s+)?index\b|materialized\s+view\b)`)
	callRe           = regexp.MustCompile(`("[^"]+"|[A-Za-z_][\w$]*)\s*\(`)
)

func identName(s string) string {
	if strings.HasPrefix(s, `"`) {
		return strings.Trim(s, `"`)
	}
	return strings.ToLower(s)
}

// indexFuncSearchPath flags expression indexes and materialized views that
// call a user-defined SQL/PL function which does not set its own
// search_path (PostgreSQL 17 runs maintenance with a safe search_path).
func indexFuncSearchPath(r catalog.Entry, stmts []sqlsplit.Statement, file string) []Finding {
	unsafe := map[string]bool{} // function name -> lacks SET search_path
	for _, s := range stmts {
		m := createFunctionRe.FindStringSubmatch(s.Outer)
		if m == nil || langCRe.MatchString(s.Outer) {
			continue
		}
		name := identName(m[1])
		if !setSearchPathRe.MatchString(s.Outer) {
			unsafe[name] = true
		} else if _, ok := unsafe[name]; !ok {
			unsafe[name] = false
		}
	}
	var out []Finding
	for _, s := range stmts {
		if !indexOrMatviewRe.MatchString(s.Outer) {
			continue
		}
		seen := map[string]bool{}
		for _, m := range callRe.FindAllStringSubmatchIndex(s.Outer, -1) {
			name := identName(s.Outer[m[2]:m[3]])
			if unsafe[name] && !seen[name] {
				seen[name] = true
				head := s.Outer
				if k := strings.IndexByte(head, '\n'); k >= 0 {
					head = head[:k]
				}
				out = append(out, Finding{Rule: r, File: file, Line: s.Line, Evidence: evidence(fmt.Sprintf("%s() used by: %s", name, head))})
			}
		}
	}
	return out
}
