# Contributing to pgupcheck

Thanks for helping. Users will trust a pgupcheck report before they upgrade production, so accuracy matters more than coverage.

## Adding a catalog entry

1. Find the change in the **official release notes** for the version that introduced it: `https://www.postgresql.org/docs/release/<major>.0/` (usually the *Migration to Version N* section).
2. Add the entry to `catalog/pg<major>.yaml`:

   ```yaml
   - id: PG16-promote-trigger-file          # PG<major>-<kebab-case-name>
     introduced_in: 16
     severity: error                         # error | warning | info
     title: promote_trigger_file was removed
     description: >
       What changed and why it breaks things.
     detect:
       conf:                                 # server parameter in postgresql.conf or ALTER … SET
         - param: promote_trigger_file
           # value_regex: '^md5$'            # optional, case-insensitive
       # sql_regex: ['\bsome_function\s*\(']   # RE2, case-insensitive, per SQL statement
       # builtin: postfix_operator            # Go detector in internal/check
       # always: true                         # applies to every cluster crossing this version
     remediation: >
       What the user should do.
     url: https://www.postgresql.org/docs/release/16.0/
   ```

   Severity guide: `error` means the restore, the upgrade or the application fails. `warning` means likely breakage or a deprecation that needs action. `info` is a behavior change worth knowing about.

3. Extend `testdata/schema.sql` or `testdata/postgresql.conf` so the entry is detected, and add its expected count in `internal/check/check_test.go`. If a false positive is easy to hit, add a negative case too (see the commented-out setting in the fixture).
4. Run the checks:

   ```bash
   gofmt -l . && go vet ./... && go test ./...
   ```

   No local Go? Use Docker: `docker run --rm -v "$PWD":/src -w /src golang:1.24 go test ./...`

In the pull request, quote the release-note sentence you relied on. If you are not sure an item is a real incompatibility, open an issue first.
