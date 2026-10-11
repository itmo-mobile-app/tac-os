package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func startTestBroker(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() { _ = newBroker().serve(listener) }()
	return listener.Addr().String()
}

func connect(t *testing.T, address string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func sendLine(t *testing.T, c net.Conn, line string) {
	t.Helper()
	if _, err := fmt.Fprint(c, line+"\n"); err != nil {
		t.Fatal(err)
	}
}

func readLine(t *testing.T, c net.Conn) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(line, "\n")
}

func TestDeliveryAndNoSubscribers(t *testing.T) {
	addr := startTestBroker(t)
	subscriber := connect(t, addr)
	publisher := connect(t, addr)
	sendLine(t, subscriber, "SUB observations")
	// Ensure the subscription was processed before publishing.
	// A second subscription and a small wait are sufficient for this network-level test.
	time.Sleep(30 * time.Millisecond)
	payload := `{"latitude": 55.75, "longitude": -37.61}`
	sendLine(t, publisher, "PUB observations "+payload)
	want := "MSG observations " + payload
	if got := readLine(t, subscriber); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	sendLine(t, publisher, `PUB nobody {"a": 1}`)
	sendLine(t, publisher, "PUB observations "+payload)
	if got := readLine(t, subscriber); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDisconnectRemovesSubscriptions(t *testing.T) {
	b := newBroker()
	a, other := net.Pipe()
	c := &client{conn: a}
	b.subscribe(c, "observations")
	if len(b.subscribers["observations"]) != 1 {
		t.Fatal("missing subscription")
	}
	b.remove(c)
	if len(b.subscribers) != 0 {
		t.Fatal("subscription was not removed")
	}
	_ = a.Close()
	_ = other.Close()
}

func TestValidTopic(t *testing.T) {
	for _, topic := range []string{"", "two words", "tab\there"} {
		if validTopic(topic) {
			t.Fatalf("accepted invalid topic %q", topic)
		}
	}
	if !validTopic("targets.updated") {
		t.Fatal("rejected valid topic")
	}
}
