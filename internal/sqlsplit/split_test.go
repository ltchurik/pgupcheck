package sqlsplit

import (
	"strings"
	"testing"
)

func TestSplitDollarQuotesAndComments(t *testing.T) {
	src := `-- header comment; with a semicolon
SET client_encoding = 'UTF8';

/* block ; comment /* nested */ still comment */
CREATE FUNCTION f() RETURNS void
    LANGUAGE plpgsql
    AS $_$
BEGIN
  PERFORM 1; -- ; inside body
END;
$_$;

SELECT 'it''s; fine', E'back\'slash;', "we;ird", $1;
`
	got := Split(src)
	if len(got) != 3 {
		for _, s := range got {
			t.Logf("line %d: %q", s.Line, s.Text)
		}
		t.Fatalf("want 3 statements, got %d", len(got))
	}
	if got[0].Line != 2 || !strings.HasPrefix(got[0].Text, "SET client_encoding") {
		t.Errorf("stmt 0: line %d text %q", got[0].Line, got[0].Text)
	}
	if got[1].Line != 5 || !strings.Contains(got[1].Text, "PERFORM 1;") {
		t.Errorf("stmt 1: line %d text %q", got[1].Line, got[1].Text)
	}
	if strings.Contains(got[1].Outer, "PERFORM") {
		t.Errorf("Outer should not contain the body: %q", got[1].Outer)
	}
	if !strings.Contains(got[1].Outer, "LANGUAGE plpgsql") {
		t.Errorf("Outer lost clauses: %q", got[1].Outer)
	}
	if got[2].Line != 13 {
		t.Errorf("stmt 2 line = %d, want 13", got[2].Line)
	}
	if idx := strings.Index(got[1].Text, "PERFORM"); got[1].LineOf(idx) != 9 {
		t.Errorf("LineOf(PERFORM) = %d, want 9", got[1].LineOf(idx))
	}
	if strings.Contains(got[0].Text, "header") {
		t.Errorf("comments must be blanked: %q", got[0].Text)
	}
}

func TestSplitUnterminatedDollarQuote(t *testing.T) {
	got := Split("CREATE FUNCTION f() AS $$ SELECT 1; ")
	if len(got) != 1 {
		t.Fatalf("want 1 statement, got %d", len(got))
	}
}
