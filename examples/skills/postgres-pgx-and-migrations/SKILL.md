---
name: postgres-pgx-and-migrations
description: "Use when writing SQL, sqlc/pgx data-access code, transactions, indexes, or schema migrations for PostgreSQL from Go."
---

# Postgres, pgx and migrations

**Queries**
- Parameterize everything (`$1`); never build SQL with string formatting of user input. If the repo uses `sqlc`, write SQL in the query files and regenerate (`sqlc generate`); don't hand-edit generated code.
- Select only needed columns; no `SELECT *` in application code. Always `ORDER BY` with a unique tie-breaker when paginating. Bound every list (`LIMIT`).
- Watch for N+1: fetch related rows in one query (`= ANY($1)`, joins, or `jsonb_agg`) rather than a query per row.
- Use `pgx.ErrNoRows` → domain not-found at the data layer. Close rows (`defer rows.Close()`) and check `rows.Err()`.

**Transactions**
- Use `pgx.BeginFunc` (or equivalent) so commit/rollback is automatic. Keep transactions short: no network calls or LLM/CLI work inside one.
- Multi-step invariants (check-then-insert, counters, state machines) need either a unique constraint, `SELECT ... FOR UPDATE`, or an atomic `UPDATE ... WHERE state = $old RETURNING`. Don't rely on read-then-write in app code. Handle serialization/deadlock errors (`40001`, `40P01`) with a bounded retry.
- Enforce rules in the schema too: `NOT NULL`, `CHECK`, foreign keys, unique indexes. Map constraint violations (`23505` unique, `23503` FK) to domain errors.

**Indexes**
- Index foreign keys and columns you filter/sort on; match composite index column order to the query (equality columns first, then range/sort). Check with `EXPLAIN (ANALYZE, BUFFERS)` on realistic data. On big tables create with `CREATE INDEX CONCURRENTLY`.

**Migrations**
- One logical change per migration, forward-only and idempotent where practical; never edit a migration that has shipped: add a new one.
- Make them **safe for rolling deploys**: *expand → migrate → contract*. Add nullable columns / new tables first, deploy code that writes both, backfill in batches, then add constraints/drop old columns in a later release. Avoid long `ACCESS EXCLUSIVE` locks: no table rewrites in one shot (`ALTER COLUMN TYPE`, adding a column with a volatile default on old Postgres), add `NOT NULL`/FK as `NOT VALID` then `VALIDATE CONSTRAINT`.
- Backfills run in batches with a sleep/limit, not one giant `UPDATE`.
- Test the migration on a fresh database *and* on a copy with existing data. Applying a migration to any shared environment goes through `request_approval` / the gated tool: never run it directly.
- Use `timestamptz`, `uuid`/`bigint` keys consistently with the existing schema; store money as integer minor units or `numeric`, never floats.
