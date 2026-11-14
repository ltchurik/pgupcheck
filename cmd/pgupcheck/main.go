// Command pgupcheck reports PostgreSQL major-version breaking changes that
// affect a schema dump and a postgresql.conf.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ltchurik/pgupcheck/catalog"
	"github.com/ltchurik/pgupcheck/internal/check"
	"github.com/ltchurik/pgupcheck/internal/report"
)

var version = "dev"

const usage = `pgupcheck: will your PostgreSQL major upgrade break anything?

Usage:
  pgupcheck --from 13 --to 18 [--schema schema.sql] [--conf postgresql.conf] [flags]
  pgupcheck --list [--from 13 --to 18]

Get the schema with:  pg_dump --schema-only --no-owner mydb > schema.sql

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// majorVersion accepts "13", "13.4" or "13.4.1" and returns 13.
func majorVersion(s string) (int, error) {
	head, _, _ := strings.Cut(strings.TrimSpace(s), ".")
	v, err := strconv.Atoi(head)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q", s)
	}
	return v, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("pgupcheck", flag.ContinueOnError)
	fl.SetOutput(stderr)
	fl.Usage = func() {
		fmt.Fprint(stderr, usage)
		fl.PrintDefaults()
	}
	from := fl.String("from", "", "current major version (e.g. 13)")
	to := fl.String("to", strconv.Itoa(catalog.MaxTo), "target major version")
	schema := fl.String("schema", "", "schema-only dump (pg_dump --schema-only)")
	conf := fl.String("conf", "", "postgresql.conf of the current cluster")
	format := fl.String("format", "text", "output format: text, json or sarif")
	failOn := fl.String("fail-on", "error", "exit with code 1 on findings of this severity or worse: error, warning, info or never")
	list := fl.Bool("list", false, "list catalog rules (in the --from/--to range if given) and exit")
	showVersion := fl.Bool("version", false, "print version and exit")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "pgupcheck", version)
		return 0
	}

	all, err := catalog.Load()
	if err != nil {
		fmt.Fprintln(stderr, "pgupcheck: catalog:", err)
		return 2
	}

	toV, err := majorVersion(*to)
	if err != nil {
		fmt.Fprintln(stderr, "pgupcheck:", err)
		return 2
	}
	fromV := catalog.MinFrom
	if *from != "" {
		if fromV, err = majorVersion(*from); err != nil {
			fmt.Fprintln(stderr, "pgupcheck:", err)
			return 2
		}
	} else if !*list {
		fmt.Fprintln(stderr, "pgupcheck: --from is required")
		fl.Usage()
		return 2
	}
	if fromV < catalog.MinFrom || toV > catalog.MaxTo || fromV >= toV {
		fmt.Fprintf(stderr, "pgupcheck: supported upgrades are between %d and %d with --from lower than --to\n", catalog.MinFrom, catalog.MaxTo)
		return 2
	}
	rules := catalog.InRange(all, fromV, toV)

	if *list {
		for _, r := range rules {
			fmt.Fprintf(stdout, "%-38s PG%d  %-7s %s\n", r.ID, r.IntroducedIn, r.Severity, r.Title)
		}
		return 0
	}

	if *schema == "" && *conf == "" {
		fmt.Fprintln(stderr, "pgupcheck: give at least one of --schema or --conf")
		return 2
	}
	in := check.Input{SchemaFile: *schema, ConfFile: *conf}
	if *schema != "" {
		b, err := os.ReadFile(*schema)
		if err != nil {
			fmt.Fprintln(stderr, "pgupcheck:", err)
			return 2
		}
		in.Schema = string(b)
	}
	if *conf != "" {
		b, err := os.ReadFile(*conf)
		if err != nil {
			fmt.Fprintln(stderr, "pgupcheck:", err)
			return 2
		}
		in.Conf = string(b)
	}

	findings, err := check.Run(rules, in)
	if err != nil {
		fmt.Fprintln(stderr, "pgupcheck:", err)
		return 2
	}

	meta := report.Meta{Version: version, From: fromV, To: toV, RulesInRange: len(rules)}
	switch *format {
	case "text":
		err = report.Text(stdout, meta, findings)
	case "json":
		err = report.JSON(stdout, meta, findings)
	case "sarif":
		err = report.SARIF(stdout, meta, findings)
	default:
		fmt.Fprintf(stderr, "pgupcheck: unknown format %q\n", *format)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "pgupcheck:", err)
		return 2
	}

	threshold := map[string]int{"error": 0, "warning": 1, "info": 2}
	limit, ok := threshold[*failOn]
	if *failOn == "never" {
		return 0
	}
	if !ok {
		fmt.Fprintf(stderr, "pgupcheck: unknown --fail-on %q\n", *failOn)
		return 2
	}
	for _, f := range findings {
		if f.Rule.Severity.Rank() <= limit {
			return 1
		}
	}
	return 0
}
