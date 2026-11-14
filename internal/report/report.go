// Package report renders findings as text, JSON or SARIF 2.1.0.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ltchurik/pgupcheck/catalog"
	"github.com/ltchurik/pgupcheck/internal/check"
)

// Meta describes the run.
type Meta struct {
	Version      string
	From, To     int
	RulesInRange int
}

// Counts returns the number of findings per severity.
func Counts(fs []check.Finding) map[catalog.Severity]int {
	c := map[catalog.Severity]int{}
	for _, f := range fs {
		c[f.Rule.Severity]++
	}
	return c
}

func location(f check.Finding) string {
	if f.File == "" {
		return "(cluster)"
	}
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return f.File
}

// Text writes a human-readable report.
func Text(w io.Writer, m Meta, fs []check.Finding) error {
	c := Counts(fs)
	fmt.Fprintf(w, "pgupcheck: PostgreSQL %d -> %d, %d catalog rules in range\n\n", m.From, m.To, m.RulesInRange)
	for _, f := range fs {
		fmt.Fprintf(w, "%-7s %s  %s\n", strings.ToUpper(string(f.Rule.Severity)), f.Rule.ID, location(f))
		fmt.Fprintf(w, "        %s (PG %d)\n", f.Rule.Title, f.Rule.IntroducedIn)
		if f.Evidence != "" {
			fmt.Fprintf(w, "        found: %s\n", f.Evidence)
		}
		fmt.Fprintf(w, "        fix:   %s\n", f.Rule.Remediation)
		fmt.Fprintf(w, "        docs:  %s\n\n", f.Rule.URL)
	}
	_, err := fmt.Fprintf(w, "%d errors, %d warnings, %d info\n", c[catalog.Error], c[catalog.Warning], c[catalog.Info])
	return err
}

type jsonFinding struct {
	catalog.Entry
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

// JSON writes a machine-readable report.
func JSON(w io.Writer, m Meta, fs []check.Finding) error {
	out := struct {
		Tool     string                   `json:"tool"`
		Version  string                   `json:"version"`
		From     int                      `json:"from"`
		To       int                      `json:"to"`
		Summary  map[catalog.Severity]int `json:"summary"`
		Findings []jsonFinding            `json:"findings"`
	}{Tool: "pgupcheck", Version: m.Version, From: m.From, To: m.To, Summary: Counts(fs), Findings: []jsonFinding{}}
	for _, f := range fs {
		out.Findings = append(out.Findings, jsonFinding{Entry: f.Rule, File: f.File, Line: f.Line, Evidence: f.Evidence})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// SARIF writes a SARIF 2.1.0 log, e.g. for GitHub code scanning.
func SARIF(w io.Writer, m Meta, fs []check.Finding) error {
	type msg struct {
		Text string `json:"text"`
	}
	type rule struct {
		ID               string `json:"id"`
		ShortDescription msg    `json:"shortDescription"`
		FullDescription  msg    `json:"fullDescription"`
		HelpURI          string `json:"helpUri"`
		Help             msg    `json:"help"`
	}
	type region struct {
		StartLine int `json:"startLine"`
	}
	type artifact struct {
		URI string `json:"uri"`
	}
	type physical struct {
		ArtifactLocation artifact `json:"artifactLocation"`
		Region           *region  `json:"region,omitempty"`
	}
	type loc struct {
		PhysicalLocation physical `json:"physicalLocation"`
	}
	type result struct {
		RuleID    string `json:"ruleId"`
		Level     string `json:"level"`
		Message   msg    `json:"message"`
		Locations []loc  `json:"locations,omitempty"`
	}
	var rules []rule
	seen := map[string]bool{}
	var results []result
	for _, f := range fs {
		if !seen[f.Rule.ID] {
			seen[f.Rule.ID] = true
			rules = append(rules, rule{
				ID: f.Rule.ID, ShortDescription: msg{f.Rule.Title}, FullDescription: msg{f.Rule.Description},
				HelpURI: f.Rule.URL, Help: msg{f.Rule.Remediation},
			})
		}
		level := map[catalog.Severity]string{catalog.Error: "error", catalog.Warning: "warning", catalog.Info: "note"}[f.Rule.Severity]
		text := f.Rule.Title
		if f.Evidence != "" {
			text += ": " + f.Evidence
		}
		r := result{RuleID: f.Rule.ID, Level: level, Message: msg{text}}
		if f.File != "" {
			p := physical{ArtifactLocation: artifact{URI: f.File}}
			if f.Line > 0 {
				p.Region = &region{StartLine: f.Line}
			}
			r.Locations = []loc{{PhysicalLocation: p}}
		}
		results = append(results, r)
	}
	if rules == nil {
		rules = []rule{}
	}
	if results == nil {
		results = []result{}
	}
	log := map[string]any{
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"version": "2.1.0",
		"runs": []any{map[string]any{
			"tool": map[string]any{"driver": map[string]any{
				"name": "pgupcheck", "version": m.Version,
				"informationUri": "https://github.com/ltchurik/pgupcheck", "rules": rules,
			}},
			"results": results,
		}},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}
