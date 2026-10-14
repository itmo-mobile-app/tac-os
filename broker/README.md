# TCP Broker (Go)

A minimal publish/subscribe TCP server following `spec/broker-protocol.md`.

## Run

From the `broker/` directory:

```sh
go run . 8080
```

## Test

```sh
go test ./...
go test -race ./...
```

## Protocol

- `SUB <topic>\n` subscribes the connection to a topic.
- `PUB <topic> <payload>\n` delivers `MSG <topic> <payload>\n` to subscribers.
- Topics cannot contain whitespace; payload is all text after the second space.
- Payload is opaque to the Broker; no JSON parsing occurs.
- Messages are not persisted or retried.
- Closing a connection removes its subscriptions.

The port must be supplied as a command-line argument. No default is assumed.
