# SQL specification

Status: approved by the SQL DBMS owner (@chatty-king) for the Sprint 0 minimal SQL subset

This document defines the minimal SQL subset the DBMS must support for the Target Service (see
`docs/ai-context/01-architecture.md` → "DBMS").

## Types

- `INTEGER`
- `REAL`
- `TEXT`

## Literals and lexical rules

The supported value literals are:

| Type | Syntax | Examples |
| --- | --- | --- |
| `INTEGER` | An optional `-` followed by one or more decimal digits, without a decimal point | `30`, `-30` |
| `REAL` | An optional `-`, one or more decimal digits, `.`, and one or more decimal digits | `30.0`, `-30.5` |
| `TEXT` | Characters enclosed in single quotes | `'t1'`, `'O''Reilly'` |

A single quote inside a `TEXT` literal is written as two consecutive single
quotes (`''`). A backslash is not an escape character. The contents and case
of a `TEXT` literal are preserved.

`30` is an `INTEGER` literal; `30.0` is a `REAL` literal. Negative numbers,
including `-30` and `-30.5`, are supported.

`INTEGER` values are signed 64-bit integers, so they can hold millisecond
timestamps such as `1732000000000`. An `INTEGER` literal outside this range is
an error.

SQL keywords are case-insensitive: `SELECT`, `select`, and `Select` have the
same meaning. Every statement must end with `;`. A semicolon inside a quoted
`TEXT` literal does not end the statement.

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

## Sprint 0 executor behavior

The first implementation (issue #31) supports `CREATE TABLE`, `INSERT`, and
`SELECT` in memory. `UPDATE` remains part of the overall SQL contract and is
deferred to a later increment. Data is discarded when the executor is closed.
The command-line entry point does not settle the Target Service client protocol.

The following execution rules are proposed for review in the implementation PR:

- Unquoted table and column identifiers follow `[A-Za-z_][A-Za-z0-9_]*`. Their
  spelling and case are preserved; identifier lookup is case-sensitive.
- A table name must be unique. Column names must be unique within their table.
- INSERT lists every declared column exactly once, in any order, and supplies
  one value per listed column. There are no NULL literals or default values.
- INSERT values and WHERE literals must have the exact column type. There is no
  implicit INTEGER-to-REAL conversion: `30.0` is required for a REAL column.
- REAL values use finite double-precision floating-point values. Overflow is an
  error. WHERE uses equality of the stored typed values.
- Unknown tables, unknown columns, and unsupported SQL produce errors. Errors
  in parsing or validation leave table contents unchanged.
- SELECT can project the same column more than once. Row order is unspecified
  without ORDER BY, which is unsupported in this subset.

The persistent storage format, indexes, transactions, client transport, and
deployment model remain open questions.

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
