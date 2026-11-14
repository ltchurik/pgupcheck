package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ltchurik/pgupcheck/catalog"
	"github.com/ltchurik/pgupcheck/internal/check"
)

var sample = []check.Finding{
	{Rule: catalog.Entry{ID: "PG17-x", IntroducedIn: 17, Severity: catalog.Error, Title: "t", Remediation: "r", URL: "https://www.postgresql.org/docs/release/17.0/"}, File: "postgresql.conf", Line: 3, Evidence: "x = 1"},
	{Rule: catalog.Entry{ID: "PG18-y", IntroducedIn: 18, Severity: catalog.Info, Title: "u", Remediation: "s", URL: "https://www.postgresql.org/docs/release/18.0/"}},
}

func TestSARIFIsValidJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := SARIF(&buf, Meta{Version: "test", From: 13, To: 18}, sample); err != nil {
		t.Fatal(err)
	}
	var v struct {
		Version string `json:"version"`
		Runs    []struct {
			Results []struct {
				RuleID string `json:"ruleId"`
				Level  string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Version != "2.1.0" || len(v.Runs) != 1 || len(v.Runs[0].Results) != 2 || v.Runs[0].Results[1].Level != "note" {
		t.Errorf("unexpected SARIF: %s", buf.String())
	}
}

func TestTextAndJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, Meta{From: 13, To: 18, RulesInRange: 5}, sample); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "postgresql.conf:3") || !strings.Contains(buf.String(), "1 errors, 0 warnings, 1 info") {
		t.Errorf("text report:\n%s", buf.String())
	}
	buf.Reset()
	if err := JSON(&buf, Meta{From: 13, To: 18}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"findings": []`) {
		t.Errorf("empty findings should be an empty array: %s", buf.String())
	}
}
