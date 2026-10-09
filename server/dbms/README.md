# Sprint 0 SQL DBMS

This component implements issue #31: a standalone C++17 executor for `CREATE TABLE`,
`INSERT INTO ... VALUES`, and `SELECT ... FROM ... [WHERE column = value]` from
`spec/sql.md`. `UPDATE` belongs to the overall SQL contract, but is outside this increment.

Tables exist only in memory. Closing `tacdb` discards them. The command-line tool
is a development entry point, not a settled Target Service communication protocol.
Persistent storage, deployment, indexes, transactions, and client transport remain
open questions in `docs/ai-context/02-open-questions.md`.

## Build and test only this component

From the repository root on Linux, with CMake 3.16+ and a C++17 compiler:

```bash
cmake -S server/dbms -B build-dbms -DCMAKE_BUILD_TYPE=Debug
cmake --build build-dbms -j2
ctest --test-dir build-dbms --output-on-failure
./build-dbms/tacdb
```

Enable ASan/UBSan with `-DTACOS_SANITIZE=ON` at configuration time.
The isolated build avoids requiring the Kotlin compiler to run DBMS tests.

For the complete monorepo build, follow the root README. The DBMS tests are
registered in CTest with the `dbms` label:

```bash
ctest --test-dir build -L dbms --output-on-failure
```

## Try a session

Enter the following statements in the same running process:

```sql
CREATE TABLE targets (target_id TEXT, latitude REAL, last_seen_at INTEGER);
INSERT INTO targets (target_id, latitude, last_seen_at) VALUES ('t1', 59.95, 1732000000000);
SELECT target_id, last_seen_at FROM targets WHERE target_id = 't1';
```

`CREATE` and `INSERT` print `OK`. `SELECT` prints a tab-separated header, rows,
and the number of matching rows. TEXT output uses single quotes and doubled
quotes; control characters and backslashes are escaped for display. This display
format is not an SQL import or client protocol. REAL output may show additional
decimal digits to preserve double precision.

Statements can span several input lines or share a line. Only a semicolon
outside a TEXT literal ends a statement. End input with Ctrl+D on Linux, or
redirect an SQL file to stdin. An incomplete final statement is an error.

Errors go to stderr with a one-based byte position relative to the current
statement, including leading whitespace. After a complete failed statement,
the CLI continues with subsequent statements. Its final exit code is 1 if any
statement failed, and 0 otherwise. An unterminated TEXT literal consumes the
remaining input and is reported at EOF.

## Execution rules proposed with this increment

The additions to `spec/sql.md` accompanying the PR explicitly describe these rules:

- Unquoted identifiers use `[A-Za-z_][A-Za-z0-9_]*` and are case-sensitive.
- Table names and column names within one table must be unique.
- INSERT supplies every declared column exactly once, in any order. NULL and
  default values are unsupported.
- Values must have the exact declared type, including WHERE literals. No implicit
  INTEGER-to-REAL conversion occurs: use `30.0` for a REAL column.
- REAL values use finite C++ `double` values; overflow is an error.
- Failed parsing or validation does not partially create a table or insert a row.
- Repeated projected SELECT columns are allowed. Without ORDER BY, callers must
  not rely on row order; this implementation scans its rows in insertion order.

The in-memory representation is an implementation detail of this increment,
not a decision about the persistent table format.

## Code map

- `include/tacos/dbms.hpp`: local executor, typed values, query result, error API.
- `src/lexer.cpp`: lexical rules, including decoded TEXT literals.
- `src/parser.cpp`: supported statement grammar and numeric range checks.
- `src/database.cpp`: schema validation, insertion, projection, equality filtering.
- `src/main.cpp`: input framing and display.
- `tests/dbms`: behavior tests and an end-to-end CLI scenario, using CTest without
  selecting a project-wide third-party unit-test framework.

`Database::execute` accepts exactly one semicolon-terminated statement.
Its `QueryError::position()` is a zero-based byte offset. The API returns copies
of selected values so callers cannot mutate stored rows through the result.
