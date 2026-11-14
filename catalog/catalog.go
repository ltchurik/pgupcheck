// Package catalog holds the embedded catalog of PostgreSQL major-version
// breaking changes. Each YAML file describes the changes introduced in one
// major version; every entry must cite the official release notes.
package catalog

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed *.yaml
var files embed.FS

// Supported version range for upgrade checks.
const (
	MinFrom = 12
	MaxTo   = 18
)

// Severity of a catalog entry.
type Severity string

const (
	Error   Severity = "error"   // the upgrade or the application will break
	Warning Severity = "warning" // likely breakage or a deprecation to act on
	Info    Severity = "info"    // behavior change worth knowing about
)

// Rank orders severities: error < warning < info.
func (s Severity) Rank() int {
	switch s {
	case Error:
		return 0
	case Warning:
		return 1
	default:
		return 2
	}
}

// ConfMatch matches a server parameter in postgresql.conf, or set through
// ALTER SYSTEM / ALTER DATABASE / ALTER ROLE ... SET in the schema.
type ConfMatch struct {
	Param string `yaml:"param"`
	// ValueRegex, if set, must match the (unquoted) value, case-insensitive.
	ValueRegex string `yaml:"value_regex,omitempty"`
}

// Detect describes how an entry is detected. Any non-empty field applies.
type Detect struct {
	Conf     []ConfMatch `yaml:"conf,omitempty"`
	SQLRegex []string    `yaml:"sql_regex,omitempty"` // RE2, matched case-insensitively per statement
	Builtin  string      `yaml:"builtin,omitempty"`   // name of a detector implemented in Go
	Always   bool        `yaml:"always,omitempty"`    // applies to every cluster crossing this version
}

// Entry is one breaking change.
type Entry struct {
	ID           string   `yaml:"id" json:"id"`
	IntroducedIn int      `yaml:"introduced_in" json:"introduced_in"`
	Severity     Severity `yaml:"severity" json:"severity"`
	Title        string   `yaml:"title" json:"title"`
	Description  string   `yaml:"description" json:"description"`
	Detect       Detect   `yaml:"detect" json:"-"`
	Remediation  string   `yaml:"remediation" json:"remediation"`
	URL          string   `yaml:"url" json:"url"`
}

var idRe = regexp.MustCompile(`^PG1[3-8]-[a-z0-9-]+$`)

// Load parses and validates the embedded catalog, sorted by version and ID.
func Load() ([]Entry, error) {
	paths, err := fs.Glob(files, "*.yaml")
	if err != nil {
		return nil, err
	}
	var all []Entry
	seen := map[string]bool{}
	for _, p := range paths {
		b, err := files.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var entries []Entry
		if err := yaml.Unmarshal(b, &entries); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		for _, e := range entries {
			if err := validate(e); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", p, e.ID, err)
			}
			if seen[e.ID] {
				return nil, fmt.Errorf("%s: duplicate id %s", p, e.ID)
			}
			seen[e.ID] = true
			e.Description = strings.TrimSpace(e.Description)
			e.Remediation = strings.TrimSpace(e.Remediation)
			all = append(all, e)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].IntroducedIn != all[j].IntroducedIn {
			return all[i].IntroducedIn < all[j].IntroducedIn
		}
		return all[i].ID < all[j].ID
	})
	return all, nil
}

func validate(e Entry) error {
	if !idRe.MatchString(e.ID) {
		return fmt.Errorf("id must look like PG15-some-name")
	}
	if want := fmt.Sprintf("PG%d-", e.IntroducedIn); !strings.HasPrefix(e.ID, want) {
		return fmt.Errorf("id prefix does not match introduced_in %d", e.IntroducedIn)
	}
	switch e.Severity {
	case Error, Warning, Info:
	default:
		return fmt.Errorf("invalid severity %q", e.Severity)
	}
	if e.Title == "" || e.Remediation == "" {
		return fmt.Errorf("title and remediation are required")
	}
	if want := fmt.Sprintf("https://www.postgresql.org/docs/release/%d.", e.IntroducedIn); !strings.HasPrefix(e.URL, want) {
		return fmt.Errorf("url must point to the official release notes (%s…)", want)
	}
	d := e.Detect
	if len(d.Conf) == 0 && len(d.SQLRegex) == 0 && d.Builtin == "" && !d.Always {
		return fmt.Errorf("detect is empty")
	}
	for _, c := range d.Conf {
		if c.Param == "" {
			return fmt.Errorf("conf match without param")
		}
		if c.ValueRegex != "" {
			if _, err := regexp.Compile(c.ValueRegex); err != nil {
				return err
			}
		}
	}
	for _, r := range d.SQLRegex {
		if _, err := regexp.Compile(r); err != nil {
			return err
		}
	}
	return nil
}

// InRange returns the entries relevant to an upgrade from major version
// from to major version to: from < introduced_in <= to.
func InRange(entries []Entry, from, to int) []Entry {
	var out []Entry
	for _, e := range entries {
		if e.IntroducedIn > from && e.IntroducedIn <= to {
			out = append(out, e)
		}
	}
	return out
}
