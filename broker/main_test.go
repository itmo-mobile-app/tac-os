package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func startTestBroker(t *testing.T) (string, *broker) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	b := newBroker()
	go func() { _ = b.serve(listener) }()
	return listener.Addr().String(), b
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

func waitForSubscribers(t *testing.T, b *broker, topic string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.RLock()
		got := len(b.subscribers[topic])
		b.mu.RUnlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("subscription count for %q never reached %d", topic, want)
}

func TestDeliveryAndNoSubscribers(t *testing.T) {
	addr, b := startTestBroker(t)
	subscriber := connect(t, addr)
	publisher := connect(t, addr)
	sendLine(t, subscriber, "SUB observations")
	waitForSubscribers(t, b, "observations", 1)
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
	addr, b := startTestBroker(t)
	subscriber := connect(t, addr)
	sendLine(t, subscriber, "SUB observations")
	waitForSubscribers(t, b, "observations", 1)
	if err := subscriber.Close(); err != nil {
		t.Fatal(err)
	}
	waitForSubscribers(t, b, "observations", 0)
}

func TestSlowSubscriberDoesNotBlockHealthySubscriber(t *testing.T) {
	b := newBroker()
	slowServer, slowPeer := net.Pipe()
	fastServer, fastPeer := net.Pipe()
	defer slowServer.Close()
	defer slowPeer.Close()
	defer fastServer.Close()
	defer fastPeer.Close()
	b.subscribe(&client{conn: slowServer}, "observations")
	b.subscribe(&client{conn: fastServer}, "observations")

	delivered := make(chan string, 1)
	go func() {
		_ = fastPeer.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, err := bufio.NewReader(fastPeer).ReadString('\n')
		if err == nil {
			delivered <- line
		}
	}()

	published := make(chan struct{})
	go func() {
		b.publish("observations", "{}")
		close(published)
	}()
	select {
	case got := <-delivered:
		if got != "MSG observations {}\n" {
			t.Fatalf("unexpected delivery: %q", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("a stalled subscriber delayed delivery to a healthy one")
	}
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("publication remained blocked on a stalled subscriber")
	}
	waitForSubscribers(t, b, "observations", 1)
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
