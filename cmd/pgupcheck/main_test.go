package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"--from", "13", "--to", "18", "--schema", "../../testdata/schema.sql", "--conf", "../../testdata/postgresql.conf"}, 1},
		{[]string{"--from", "13", "--to", "18", "--conf", "../../testdata/postgresql.conf", "--fail-on", "never"}, 0},
		{[]string{"--from", "17.2", "--to", "18", "--schema", "../../testdata/schema.sql", "--fail-on", "error"}, 0},
		{[]string{"--from", "17", "--to", "18", "--schema", "../../testdata/schema.sql", "--fail-on", "warning"}, 1},
		{[]string{"--from", "18", "--to", "13", "--schema", "x"}, 2},
		{[]string{"--to", "18", "--schema", "x"}, 2},
		{[]string{"--from", "13", "--to", "18"}, 2},
		{[]string{"--list"}, 0},
		{[]string{"--version"}, 0},
	}
	for _, c := range cases {
		var out, errOut bytes.Buffer
		if got := run(c.args, &out, &errOut); got != c.code {
			t.Errorf("%v: exit %d, want %d\nstdout: %s\nstderr: %s", c.args, got, c.code, out.String(), errOut.String())
		}
	}
}

func TestListShowsAllVersions(t *testing.T) {
	var out, errOut bytes.Buffer
	run([]string{"--list"}, &out, &errOut)
	for _, v := range []string{"PG13-", "PG14-", "PG15-", "PG16-", "PG17-", "PG18-"} {
		if !strings.Contains(out.String(), v) {
			t.Errorf("--list output lacks %s rules", v)
		}
	}
}
