# Target Service (Sprint 2, issue #68)

A standalone Go TCP client for the Broker. Subscribes to `observations`, validates
single-line JSON Observation messages, and retains the latest received Observation
for each `targetId` in memory. It logs validation errors without crashing.
No target aggregation, probability formula, `targets.updated` publication, or DBMS access.

## Run

Terminal 1 (from repository root):

```sh
cd broker
go run . 8080
```

Terminal 2:

```sh
cd target-service
go run . -broker 127.0.0.1:8080
```

Terminal 3 (optional manual publish, using netcat):

```sh
printf 'PUB observations {"targetId":"t1","observerId":"o1","latitude":55.7,"longitude":37.6,"confidence":0.8,"timestamp":1760000000000}\n' | nc 127.0.0.1 8080
```

## Tests

```sh
cd target-service
go test ./...           # includes integration test when ../broker exists
go test -short ./...    # unit tests without Broker
```

The integration test starts the actual Go Broker process from `../broker`.
If Broker source is absent, the integration test is skipped.

The service exits with an explicit error when the Broker cannot be reached or
when the connection closes. Reconnection policy remains an open question.
