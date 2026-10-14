# Sprint 0 SQL DBMS

This component implements issue #31 in Go, matching the course technology stack.
It uses only the standard library and its own SQL lexer, parser, and executor.
It supports `CREATE TABLE`, `INSERT INTO ... VALUES`, and
`SELECT ... FROM ... [WHERE column = value]` from `spec/sql.md`.
`UPDATE` belongs to the overall contract but is outside this increment.

Tables exist only in memory. Closing `tacdb` discards them. The CLI is a development
entry point, not the Target Service wire protocol. Persistence and socket communication
are deferred to subsequent increments. The local Go API is not a deployment decision.

## Build and test only this component

From the repository root on Linux, with Go 1.22+:

```bash
cd server/dbms
go test -count=1 ./...
go vet ./...
go build -buildvcs=false -o /tmp/tacdb ./cmd/tacdb
/tmp/tacdb
```

There are no third-party dependencies to download. Unit tests cover all commands,
literal boundaries, errors, independent databases, result copies, and failed writes.
CLI tests cover statement framing, error recovery, multiline and long TEXT, and I/O errors.
The unchanged fixtures in `tests/dbms/` also check the actual executable through CTest.

Run the race detector separately with `go test -race -count=1 ./...`.
It requires a C compiler on supported platforms. This does not make Database safe
for concurrent use; the current CLI executes statements sequentially.

## CMake integration

From the repository root, with CMake 3.16+ and Go 1.22+:

```bash
cmake -S server/dbms -B "$HOME/tac-os-build/dbms-go"
cmake --build "$HOME/tac-os-build/dbms-go" -j2
ctest --test-dir "$HOME/tac-os-build/dbms-go" --output-on-failure
"$HOME/tac-os-build/dbms-go/tacdb"
```

The isolated build registers ten DBMS checks. It does not require Kotlin or the VM.
For the complete monorepo build, follow the root README. The CLI then lives at
`build/server/dbms/tacdb`. DBMS checks carry the `dbms` label:

```bash
ctest --test-dir build -L dbms --output-on-failure
```

CMake stores the Go build cache in its binary directory. For a nonstandard Go
installation, configure with `-DTACOS_GO_EXECUTABLE=/absolute/path/to/go`.
ASan/UBSan apply to the C++ VM and Runtime, not to Go. CI checks Go formatting,
runs `go vet` and Go tests with the race detector, then builds and tests the monorepo.

## Try a session

Enter the following statements in the same running process:

```sql
CREATE TABLE targets (target_id TEXT, latitude REAL, last_seen_at INTEGER);
INSERT INTO targets (target_id, latitude, last_seen_at) VALUES ('t1', 59.95, 1732000000000);
SELECT target_id, last_seen_at FROM targets WHERE target_id = 't1';
```

CREATE and INSERT print `OK`. SELECT prints a tab-separated header, rows, and a
row count. TEXT uses single quotes and doubled quotes; control characters and
backslashes are escaped for display. This display format is not an SQL import or
client protocol. REAL uses the shortest decimal form that round-trips to its
float64 value: `30.0` can display as `30`, while its stored type remains REAL.

Statements can span lines or share a line. Only a semicolon outside TEXT ends a
statement. End input with Ctrl+D on Linux, or redirect an SQL file to stdin.
An incomplete final statement is an error. An unterminated TEXT consumes the
remaining input and is reported at EOF.

Errors go to stderr with a one-based byte position relative to the current statement,
including leading whitespace. After a complete failed statement, the CLI continues
with subsequent statements. Its exit code is 1 if any statement or I/O failed, else 0.

## Execution rules

The rules proposed in the implementation PR are documented in `spec/sql.md`:

- Unquoted identifiers use `[A-Za-z_][A-Za-z0-9_]*` and are case-sensitive.
- Table names and column names within one table must be unique.
- INSERT supplies every declared column exactly once, in any order; no NULL/defaults.
- INSERT and WHERE require the exact declared type. Use `30.0` for REAL, not `30`.
- INTEGER uses signed int64; REAL uses finite IEEE-754 float64, with overflow rejected.
- Failed parsing or validation does not partially create a table or insert a row.
- Repeated SELECT columns are allowed. Row order is unspecified by the contract.

The table representation is an internal detail, not a persistent storage format.

## Code map

- `value.go`: typed values, copied results, query errors.
- `lexer.go`: lexical rules and TEXT decoding.
- `parser.go`: command grammar and numeric checks.
- `database.go`: schema validation, INSERT, projection, equality filtering.
- `cmd/tacdb/main.go`: process entry point.
- `internal/cli/`: stdin framing, display, error reporting, CLI unit tests.
- `database_test.go`: executor behavior tests using Go's testing package.
- `tests/dbms/`: CTest registration and executable-level SQL fixtures.

The zero value of `Database` is ready to use. `Execute` accepts one semicolon-terminated
statement. `QueryError.Position` is a zero-based byte offset. Results own copies of rows.
A used Database must not be copied or used concurrently.
