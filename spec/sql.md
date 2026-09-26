# SQL specification

Status: draft — proposed by Architecture & Integration ahead of Sprint 0, pending review and approval by
the SQL DBMS owner (see `docs/ai-context/03-team-ownership.md`) in a follow-up Pull Request.

This document defines the minimal SQL subset the DBMS must support for the Target Service (see
`docs/ai-context/01-architecture.md` → "DBMS").

## Types

- `INTEGER`
- `REAL`
- `TEXT`

## Statements

```sql
CREATE TABLE <table> (<column> <type>, ...);

INSERT INTO <table> (<column>, ...) VALUES (<value>, ...);

SELECT <column>, ... FROM <table> [WHERE <column> = <value>];
SELECT * FROM <table> [WHERE <column> = <value>];

UPDATE <table> SET <column> = <value>, ... WHERE <column> = <value>;
```

- `WHERE` supports only a single `<column> = <value>` condition (no `AND`/`OR`, no other operators) in
  this minimal version.
- No `JOIN`, no `DELETE`, no indexes, no transactions.

## Example

```sql
CREATE TABLE targets (target_id TEXT, latitude REAL, longitude REAL, probability REAL, last_seen_at INTEGER);

INSERT INTO targets (target_id, latitude, longitude, probability, last_seen_at)
  VALUES ('t1', 59.95, 30.31, 0.8, 1732000000000);

SELECT * FROM targets WHERE target_id = 't1';

UPDATE targets SET probability = 0.9 WHERE target_id = 't1';
```

## Open questions

Not decided here; see `docs/ai-context/02-open-questions.md` → "DBMS":

- whether indexes are required;
- whether transactions are required;
- storage format;
- client protocol between Target Service and DBMS (in-process library call vs. separate process).
