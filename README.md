# pgupcheck

**Will your PostgreSQL major upgrade break anything?** `pgupcheck` reads your schema dump and `postgresql.conf` and tells you, file and line, which breaking changes between your current and target major version (PostgreSQL 12 → 18) affect *your* database, and what to do about each.

[![ci](https://github.com/ltchurik/pgupcheck/actions/workflows/ci.yml/badge.svg)](https://github.com/ltchurik/pgupcheck/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

> **Status: 🚧 Early development.** The engine, the three output formats and a first catalog of 19 breaking changes work today. The catalog is far from complete. Treat a clean report as "nothing known found", not as a guarantee, and always test the upgrade on a copy first.

## Why

Every major release notes page has a *Migration* section listing incompatibilities. For a 13 → 18 upgrade that is five pages to read, and you still have to work out which items apply to your schema, functions, extensions and config. The existing checks each cover a slice:

- `pg_upgrade --check` finds catalog-level blockers in a running cluster (for example reg* types or incompatible extensions), not removed functions inside your PL/pgSQL code or removed settings in your config.
- Cloud prechecks (AWS RDS, Google Cloud SQL, Heroku) only work inside their own platforms.
- Migration linters such as [squawk](https://github.com/sbdchd/squawk) check that a *migration* is safe to run, which is a different question.
- [pg18-upgrade-check](https://github.com/atokim4/pg18-upgrade-check) checks Docker/Kubernetes deployments for the move to PostgreSQL 18 and ships a preflight SQL report.

`pgupcheck` is a single static binary that works offline, needs no access to the database server, and runs in CI. Each rule comes from a curated catalog in which **every entry cites the official release notes**.

## Quickstart

```bash
# build (Go 1.24+); release binaries will follow
go install github.com/ltchurik/pgupcheck/cmd/pgupcheck@latest

pg_dump --schema-only --no-owner mydb > schema.sql
pgupcheck --from 13 --to 18 --schema schema.sql --conf /etc/postgresql/13/main/postgresql.conf
```

Example output (from the test fixtures in [`testdata/`](testdata)):

```text
pgupcheck: PostgreSQL 15 -> 17, 7 catalog rules in range

ERROR   PG17-old-snapshot-threshold  testdata/postgresql.conf:6
        old_snapshot_threshold was removed (PG 17)
        found: old_snapshot_threshold = -1
        fix:   Remove old_snapshot_threshold from the configuration.
        docs:  https://www.postgresql.org/docs/release/17.0/

WARNING PG17-maintenance-search-path  testdata/schema.sql:63
        Functions used by indexes and materialized views now run with a safe search_path (PG 17)
        found: norm_email() used by: CREATE INDEX users_email_idx ON public.users USING btree (public.norm_email(email))
        fix:   Add SET search_path to the function (ALTER FUNCTION f(...) SET search_path = myschema, pg_catalog) or schema-qualify the objects it uses.
        docs:  https://www.postgresql.org/docs/release/17.0/

4 errors, 1 warnings, 0 info
```

### Options

| Flag | Meaning |
|---|---|
| `--from`, `--to` | Major versions (`13`, `13.4` and `13.4.1` all mean 13). `--to` defaults to 18. |
| `--schema` | Output of `pg_dump --schema-only`. Function bodies, aggregates, operators, indexes, extensions and `ALTER … SET` are analyzed. |
| `--conf` | `postgresql.conf` of the current cluster. |
| `--format` | `text` (default), `json` or `sarif` (for GitHub code scanning). |
| `--fail-on` | Exit 1 on findings of this severity or worse: `error` (default), `warning`, `info`, `never`. |
| `--list` | Print the catalog rules in the range and exit. |

Exit codes: `0` nothing at or above `--fail-on`, `1` findings, `2` usage or I/O error.

## How it works

1. The schema dump is split into statements by a small lexer that understands comments, quoted strings and dollar-quoted function bodies.
2. The [catalog](catalog/) has one YAML file per major version. Each entry has an id, the version that introduced it, a severity, a title, a detection rule, a remediation and a link to the release notes.
3. Detection is a server parameter match (in `postgresql.conf` or in `ALTER SYSTEM/DATABASE/ROLE … SET`), a regex over SQL statements, or a detector written in Go for things a regex can't express (for example postfix operators, or expression-index functions without their own `search_path`).
4. Only entries with `from < introduced_in <= to` are checked.

Currently covered: removed or renamed settings (`wal_keep_segments`, `operator_precedence_warning`, `vacuum_cleanup_index_scale_factor`, `stats_temp_directory`, `vacuum_defer_cleanup_age`, `promote_trigger_file`, `force_parallel_mode`, `old_snapshot_threshold`, `db_user_namespace`), postfix and factorial operators, aggregates built on the `array_append` family, exclusive backup functions, PL/Python 2, `adminpack`, the PG 17 maintenance `search_path` change, MD5 password deprecation, the PG 18 data checksum default, and the PG 15 `public` schema privilege change. Run `pgupcheck --list` for the full list.

## Roadmap

- [x] Statement splitter, catalog engine, text/JSON/SARIF output, CI exit codes
- [x] First catalog: 19 changes from PostgreSQL 13–18
- [ ] Release binaries (goreleaser) and a GitHub Action
- [ ] Live mode: connect read-only to the old cluster (extensions and versions, `data_checksums`, settings from `pg_settings`, roles with MD5 passwords)
- [ ] Analyze queries from `pg_stat_statements` and application SQL files
- [ ] `pg_hba.conf` checks (for example `md5` auth methods)
- [ ] Extension compatibility matrix (PostGIS, TimescaleDB, pgvector, …)
- [ ] Follow `include` directives in `postgresql.conf`
- [ ] Catalog entries still to verify against the release notes before adding: removed geometric containment operators `@` and `~` (14), `password_encryption` default change to `scram-sha-256` (14), VACUUM/ANALYZE processing inheritance children by default (18), and more items from each release's *Migration* section

## Contributing

The catalog is the heart of the project, and adding an entry is the most useful contribution. Read [CONTRIBUTING.md](CONTRIBUTING.md): every entry has to cite the official release notes and come with a test fixture.

## Author

Created and maintained by **[Ľubomír Tchurik](https://lubomirtchurik.sk)**, a Linux and PostgreSQL infrastructure specialist from Bratislava, Slovakia, working on LLM evaluation and AI-agent security.

[Website](https://lubomirtchurik.sk) · [GitHub](https://github.com/ltchurik) · [LinkedIn](https://www.linkedin.com/in/lubomir-tchurik/) · [ORCID](https://orcid.org/0009-0005-8053-9405)

If this project is useful to you, a ⭐ on GitHub helps others find it.

### More open-source projects by the author

- [sk-llm-eval](https://github.com/ltchurik/sk-llm-eval): Does your LLM write correct Slovak? 207-task language quality benchmark
- [sk-tokenizer-bench](https://github.com/ltchurik/sk-tokenizer-bench): the Slovak token tax across 30+ LLM tokenizers
- [token-tax-index](https://github.com/ltchurik/token-tax-index): how much more every language costs in LLM tokens
- [llm-locale-lint](https://github.com/ltchurik/llm-locale-lint): CLDR-based checker of numbers, dates, money and quotes in LLM output
- [localebench](https://github.com/ltchurik/localebench): do LLMs write like a native? Locale conventions benchmark
- [sk-asr-bench](https://github.com/ltchurik/sk-asr-bench): how well AI understands spoken Slovak
- [mcp-jail](https://github.com/ltchurik/mcp-jail): sandbox for MCP servers: egress allowlist, fs scope, audit log

## License

[Apache-2.0](LICENSE)
