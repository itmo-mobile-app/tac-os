# DBMS client protocol

Status: approved design for issue #61; socket server/client implementation pending.

This contract connects the **Target Service** (client) and the custom Go **DBMS**
(server), running as separate processes on the same central Linux node. It does not
define a new SQL language, a remote DBMS endpoint, an ORM, or a storage format.
SQL syntax and value types are defined by `spec/sql.md`.

## Transport and framing

- Use a Unix-domain **stream** socket. Default path: `/tmp/tac-os-dbms.sock`,
  configurable when starting the DBMS server and Target Service client.
- The DBMS listens and accepts connections; Target Service connects.
  The socket should be accessible only to the local service account. The server
  removes only a stale socket it owns and removes its path on orderly shutdown.
- Both requests **and responses** use identical binary length-prefix framing:

```text
+--------------------------+-------------------------------+
| 4 bytes: uint32 LE = N   | N bytes: message payload      |
+--------------------------+-------------------------------+
```

- `N` is the number of payload **bytes**, excluding the four-byte prefix.
  The unsigned 32-bit prefix is **little-endian**. It is not a character count.
- `1 <= N <= 1048576` (1 MiB). No terminating `\0`, newline, or additional
  delimiter is written after the payload. Read exactly four header bytes and
  then exactly `N` bytes; one socket `read` is not guaranteed to return a
  complete frame.
- A persistent connection can carry many request/response pairs. There is
  exactly **one outstanding request per connection**: send one request,
  receive its full response, then send the next. No request IDs, pipelining,
  subscription, streaming query results or version negotiation in v1.
- A zero/oversized declared length, truncated header/body, socket EOF mid-frame
  or read timeout is a transport/protocol failure; close that connection
  without interpreting partial bytes as a SQL request or response.
- Initial response wait timeout is **5 seconds** at the client; failed or
  disconnected connections can be reestablished. A write whose outcome is
  unknown must **not be automatically retried**, because a previous
  `INSERT` or `UPDATE` might already have executed.

## Request payload: raw UTF-8 SQL

The request payload is **plain UTF-8 SQL text**, not JSON. It contains exactly
one complete, semicolon-terminated statement supported by `spec/sql.md`:

```sql
SELECT target_id, probability FROM targets WHERE target_id = '17';
```

The SQL text may contain whitespace or embedded newlines; the binary length
prefix establishes the request boundary. Semicolons inside quoted SQL strings
are handled by the existing SQL parser. No SQL batch or multiple statements
per frame. Empty text and invalid UTF-8 produce a `BAD_REQUEST` response if a
complete, valid-size frame was received. Invalid SQL produces `SQL_ERROR`.

The initial SQL grammar is `CREATE TABLE`, `INSERT`, `SELECT` and `UPDATE` as
defined in `spec/sql.md`. The in-memory executor initially lacks `UPDATE`
and will gain it in issue #67. There is no new `JOIN`, `DELETE`, transaction,
`UPSERT` or prepared-statement syntax in this protocol.

Target Service is responsible for producing valid SQL and escaping apostrophes
inside `TEXT` literals by doubling them (`'` becomes `''`); there is no SQL
parameter-binding feature in v1. A small DBMS client helper can centralize
this escaping and SQL construction, without introducing an ORM.

## Response payload: UTF-8 JSON

Every completely received and processed request receives **exactly one**
length-prefixed UTF-8 JSON object response. Unlike the request, the response
payload is JSON. No newline or pretty-printing is required.

### Successful CREATE, INSERT or UPDATE

```json
{"ok":true}
```

No row data or affected-row count is required in this increment.

### Successful SELECT

```json
{
  "ok": true,
  "columns": [
    {"name": "target_id", "type": "TEXT"},
    {"name": "probability", "type": "REAL"},
    {"name": "last_seen_at", "type": "INTEGER"}
  ],
  "rows": [
    ["17", 0.85, 1792255200000]
  ]
}
```

The actual payload is compact JSON; the formatted example above is for clarity.

- `columns` is an ordered array, one descriptor for each projected column;
  the descriptor has exactly `name` (string) and `type` (one of `INTEGER`,
  `REAL` or `TEXT`). Repeated projected column names remain allowed.
- `rows` is an ordered array of arrays. Each row has exactly as many values
  as `columns`. Row ordering is not guaranteed because `ORDER BY` is absent
  from the SQL subset.
- `TEXT` values are JSON strings, `REAL` values finite JSON numbers, and
  `INTEGER` values signed 64-bit JSON integer numbers. No null, booleans or
  implicit conversions. The column type is authoritative when the same
  number could represent either `INTEGER` or `REAL`.
- **Do not decode JSON integers through float64**. Both client and server
  are written in Go: encode signed int64 as JSON numbers and decode with
  typed structs, `json.Decoder.UseNumber()` or equivalent handling that
  preserves the full signed-64-bit range.
- A successful SELECT of zero rows still includes descriptors:
  `{"ok":true,"columns":[{"name":"target_id","type":"TEXT"}],"rows":[]}`.
- The v1 server must materialize the response inside the 1 MiB frame
  limit. If it cannot, return a small `RESULT_TOO_LARGE` error instead of
  sending a partial result. Pagination is not part of this contract.

### SQL error

```json
{"ok":false,"error":{"code":"SQL_ERROR","message":"Unknown table: missing","position":14}}
```

This example corresponds to `SELECT * FROM missing;`, where the table name
starts at **zero-based UTF-8 byte offset 14**. For `SQL_ERROR`, `position`
is a required nonnegative integer measured from the first byte of the SQL
payload, matching the current Go `dbms.QueryError.Position` (not the CLI's
one-based display position).

### Other errors

```json
{"ok":false,"error":{"code":"BAD_REQUEST","message":"SQL payload is empty"}}
```

Error responses have exactly `ok:false` and an `error` object. The
`error` object always has `code` and `message`, and has `position` only
for `SQL_ERROR`. Initially recognized codes:

| Code | Meaning |
| --- | --- |
| `SQL_ERROR` | SQL parsing, validation or execution failure with byte offset |
| `BAD_REQUEST` | Complete frame with empty/invalid UTF-8 SQL payload |
| `RESULT_TOO_LARGE` | Query result would exceed the maximum frame length |
| `INTERNAL_ERROR` | Server cannot process an otherwise valid request |

Socket connection failures, timeouts and truncated/invalid length headers do
**not** have a JSON response; the client treats them as transport errors.

## Complete exchange example

Each statement below is encoded in a **separate frame** with its own
4-byte little-endian length header. Each response is another frame:

| Target Service sends (plain SQL) | DBMS sends (JSON) |
| --- | --- |
| `CREATE TABLE targets (target_id TEXT, probability REAL);` | `{"ok":true}` |
| `INSERT INTO targets (target_id, probability) VALUES ('17', 0.85);` | `{"ok":true}` |
| `SELECT target_id, probability FROM targets WHERE target_id = '17';` | `{"ok":true,"columns":[{"name":"target_id","type":"TEXT"},{"name":"probability","type":"REAL"}],"rows":[["17",0.85]]}` |
| `SELECT * FROM missing;` | `{"ok":false,"error":{"code":"SQL_ERROR","message":"Unknown table: missing","position":14}}` |

The sample is illustrative: each SQL statement uses the exact supported
grammar and each reply is a single JSON payload, not JSON Lines.

## Execution boundaries

- The DBMS preserves `spec/sql.md` SQL semantics and uses its own parser
  and executor. It never interprets `Target`, `Observation`, or match rules.
- The current Go `dbms.Database` implementation is **not concurrency-safe**.
  Until concurrency support is explicitly added, all connections must
  execute statements against one database serially (e.g., one server-side
  mutex), not concurrently.
- DBMS remains a separate process from Target Service. A persistently stored
  database on disk is part of the target architecture but is **not
  implemented or specified by this wire protocol**.
- Client-server connection retry after a clear disconnect is allowed, but
  blindly replaying a failed write is not. SQL statements are not assumed
  idempotent.
- Tests for the socket implementation should exercise the four exchange
  cases above, mixed chunk boundaries, back-to-back frames, missing
  results, large payloads, 64-bit INTEGER round trips, malformed header,
  connection loss and timeouts.

This protocol does not change Broker, Analyzer-to-Map IPC, or Runtime-to-detector IPC.
