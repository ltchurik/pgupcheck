package check

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/ltchurik/pgupcheck/catalog"
)

func load(t *testing.T) []catalog.Entry {
	t.Helper()
	all, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func fixture(t *testing.T) Input {
	t.Helper()
	schema, err := os.ReadFile("../../testdata/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	conf, err := os.ReadFile("../../testdata/postgresql.conf")
	if err != nil {
		t.Fatal(err)
	}
	return Input{SchemaFile: "schema.sql", Schema: string(schema), ConfFile: "postgresql.conf", Conf: string(conf)}
}

func ids(fs []Finding) map[string]int {
	m := map[string]int{}
	for _, f := range fs {
		m[f.Rule.ID]++
	}
	return m
}

func TestCatalogBuiltinsExist(t *testing.T) {
	for _, e := range load(t) {
		if e.Detect.Builtin != "" {
			if _, ok := Builtins[e.Detect.Builtin]; !ok {
				t.Errorf("%s: unknown builtin %q", e.ID, e.Detect.Builtin)
			}
		}
	}
}

func TestFixture13To18(t *testing.T) {
	fs, err := Run(catalog.InRange(load(t), 13, 18), fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	got := ids(fs)
	want := map[string]int{
		"PG14-postfix-operators":             1,
		"PG14-factorial-operators":           1,
		"PG14-array-functions-anycompatible": 1,
		"PG15-stats-temp-directory":          1,
		"PG15-exclusive-backup-functions":    1,
		"PG15-plpython2":                     1,
		"PG15-public-schema-create":          1,
		"PG16-vacuum-defer-cleanup-age":      1,
		"PG16-force-parallel-mode":           1,
		"PG17-old-snapshot-threshold":        1,
		"PG17-adminpack":                     1,
		"PG17-maintenance-search-path":       1,
		"PG18-md5-passwords":                 2, // postgresql.conf + ALTER ROLE ... SET
		"PG18-data-checksums-default":        1,
	}
	for id, n := range want {
		if got[id] != n {
			t.Errorf("%s: got %d findings, want %d", id, got[id], n)
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("unexpected finding %s", id)
		}
	}
	if t.Failed() {
		for _, f := range fs {
			t.Logf("%s %s:%d %s", f.Rule.ID, f.File, f.Line, f.Evidence)
		}
	}
}

func TestLinesAndEvidence(t *testing.T) {
	fs, err := Run(catalog.InRange(load(t), 13, 18), fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]string{}
	for _, f := range fs {
		lines[f.Rule.ID+"@"+f.File] = f.Evidence
		switch f.Rule.ID {
		case "PG15-exclusive-backup-functions":
			if f.Line != 46 {
				t.Errorf("pg_start_backup line = %d, want 46", f.Line)
			}
		case "PG17-old-snapshot-threshold":
			if f.File != "postgresql.conf" || f.Line != 6 {
				t.Errorf("old_snapshot_threshold at %s:%d, want postgresql.conf:6", f.File, f.Line)
			}
		case "PG17-maintenance-search-path":
			if !strings.HasPrefix(f.Evidence, "norm_email()") {
				t.Errorf("search_path evidence = %q", f.Evidence)
			}
		}
	}
}

func TestRangeBoundaries(t *testing.T) {
	all := load(t)
	// from is exclusive: a 13 -> 14 upgrade does not include 13's changes.
	fs, _ := Run(catalog.InRange(all, 13, 14), fixture(t))
	if ids(fs)["PG13-wal-keep-segments"] != 0 {
		t.Error("PG13 change reported for an upgrade starting at 13")
	}
	// to is inclusive.
	fs, _ = Run(catalog.InRange(all, 12, 13), fixture(t))
	if ids(fs)["PG13-wal-keep-segments"] != 1 {
		t.Error("PG13 change missing for a 12 -> 13 upgrade")
	}
	fs, _ = Run(catalog.InRange(all, 16, 18), fixture(t))
	for id := range ids(fs) {
		if strings.HasPrefix(id, "PG14") || strings.HasPrefix(id, "PG15") || strings.HasPrefix(id, "PG16") {
			t.Errorf("16 -> 18 should not report %s", id)
		}
	}
}

func TestSortedBySeverity(t *testing.T) {
	fs, _ := Run(catalog.InRange(load(t), 12, 18), fixture(t))
	if !sort.SliceIsSorted(fs, func(i, j int) bool {
		return fs[i].Rule.Severity.Rank() < fs[j].Rule.Severity.Rank()
	}) {
		t.Error("findings are not sorted by severity")
	}
}

func TestParseConf(t *testing.T) {
	got := parseConf("a = 1\n  B='x # y'  # c\n#c = 2\nd off\n")
	if len(got) != 3 {
		t.Fatalf("got %d lines: %+v", len(got), got)
	}
	if got[1].Name != "b" || got[1].Value != "x # y" || got[1].Line != 2 {
		t.Errorf("line 2 = %+v", got[1])
	}
	if got[2].Name != "d" || got[2].Value != "off" {
		t.Errorf("line 4 = %+v", got[2])
	}
}
