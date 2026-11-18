# Target Service — Broker + DBMS integration (issue #137)

This service subscribes to `observations` on the **real TCP Broker**, validates the
six-field Observation JSON, aggregates readings per `(targetId, observerId)` using
`aggregation/` (issue #130), saves accepted observations and five-field targets
through the existing framed Unix-socket `dbclient/`, and publishes
`PUB targets.updated {single-line-five-field-JSON}`. A timer recalculates
probability without new observations; once all positive evidence expires, exactly
one stale (`probability: 0`) update is generated. It does not change the Broker,
DBMS, SQL, or shared JSON wire contracts.

## Run (three separate processes, Linux / Ubuntu)

From the repository root, use **three separate terminals**:

```sh
# Terminal 1: actual DBMS socket server, IN-MEMORY data
cd server/dbms
go run ./cmd/tacdb-server -socket /tmp/tac-os-dbms.sock
```

```sh
# Terminal 2: actual Broker
cd broker
go run . 8080
```

```sh
# Terminal 3: actual Target Service
cd target-service
go run . -broker 127.0.0.1:8080 -dbms /tmp/tac-os-dbms.sock -interval 5s
```

Sample new observation (use **current** Unix milliseconds, not an old sample
from the original README):

```sh
NOW_MS=$(date +%s%3N)
printf 'PUB observations {"targetId":"17","observerId":"devA","latitude":55.7,"longitude":37.6,"confidence":0.8,"timestamp":%s}\n' "$NOW_MS" | nc 127.0.0.1 8080
```

To see published updates, subscribe **before** sending the observation:

```sh
printf 'SUB targets.updated\n' | nc 127.0.0.1 8080
```

Some `nc` implementations exit on standard-input EOF; use an interactive TCP
client or `nc` with an appropriate keep-open option if necessary.

## Data and error semantics

The service initializes the following SQL v1 tables on start:

- `observations(target_id TEXT, observer_id TEXT, latitude REAL, longitude REAL, confidence REAL, timestamp INTEGER)`
- `targets(target_id TEXT, latitude REAL, longitude REAL, probability REAL, last_seen_at INTEGER)`

Valid, fresh, strictly newer observations from a particular observer are
inserted. Duplicate and older reports are ignored in memory, not inserted.
Target rows are inserted after a successful SELECT found zero matches, or
updated when one existing row is found. All SQL uses the existing DBMS client
with **4-byte little-endian framed requests** and strict framed responses.
SQL text is escaped with `dbclient.SQLString`, and REAL values are formatted
without exponent notation.

**Fail-stop**: startup fails if DBMS schema creation fails (including when the
schema already exists in a still-running DBMS). Any DBMS query/transport error,
malformed SELECT response, or Broker connection/write error terminates the
service and does not emit the affected target update. Invalid observations are
logged and dropped without terminating the process. The DBMS client never
retries writes with unknown outcomes. There is no invented fallback publish.

**Important limitations of the approved contracts:** DBMS v1 holds tables in
memory only, has **no transactions**, no constraints, no `DELETE`, no `UPSERT`,
and no affected-row count. Consequently the Observation INSERT and Target
INSERT/UPDATE are **not atomic**: a DBMS error between statements can leave a
partial record. This prototype does not implement restart recovery, a
persistent history, exactly-once processing across restarts, or rollback.
Do not restart Target Service against a nonempty DBMS session without resetting
that DBMS instance; no safe schema adoption/reconciliation is specified.
Broker delivery is **at most once**. A successful TCP write cannot prove a
subscriber consumed an update, and there are no Broker acknowledgments.

## Verification

```sh
cd target-service
gofmt -l main.go main_test.go aggregation/aggregation_test.go
go vet ./...
go test -race -count=1 ./...
```

The default tests include a system E2E that builds and starts the **real Broker
and real DBMS** in separate processes, connects Target Service to both, verifies
multi-observer updates and timer decay via the Broker, and checks DBMS records.
Unit tests cover duplicates, older/expired/invalid reports, stale-once, SQL
quoting, and no publication after DBMS failures or malformed responses.
`go test -short ./...` omits the external-process E2E.
