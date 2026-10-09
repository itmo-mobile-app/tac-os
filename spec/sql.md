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
- client protocol between Target Service and DBMS: TCP or Unix socket, how request boundaries are marked,
  how result rows and errors are encoded.

The DBMS runs as a separate server process and the Target Service connects to it as a client (see
`docs/ai-context/01-architecture.md` → "DBMS").
