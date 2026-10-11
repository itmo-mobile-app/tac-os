package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

type client struct {
	conn    net.Conn
	writeMu sync.Mutex
}

func (c *client) send(message string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := io.WriteString(c.conn, message+"\n")
	return err
}

type broker struct {
	mu          sync.RWMutex
	subscribers map[string]map[*client]struct{}
}

func newBroker() *broker {
	return &broker{subscribers: make(map[string]map[*client]struct{})}
}

func validTopic(topic string) bool {
	return topic != "" && !strings.ContainsAny(topic, " \t\r\n")
}

func (b *broker) subscribe(c *client, topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subscribers[topic] == nil {
		b.subscribers[topic] = make(map[*client]struct{})
	}
	b.subscribers[topic][c] = struct{}{}
}

func (b *broker) remove(c *client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for topic, clients := range b.subscribers {
		delete(clients, c)
		if len(clients) == 0 {
			delete(b.subscribers, topic)
		}
	}
}

func (b *broker) publish(topic, payload string) {
	b.mu.RLock()
	recipients := make([]*client, 0, len(b.subscribers[topic]))
	for c := range b.subscribers[topic] {
		recipients = append(recipients, c)
	}
	b.mu.RUnlock()

	message := "MSG " + topic + " " + payload
	for _, c := range recipients {
		if err := c.send(message); err != nil {
			// A failed connection will also be removed when its reader exits.
			_ = c.conn.Close()
			b.remove(c)
		}
	}
}

func (b *broker) handle(conn net.Conn) {
	c := &client{conn: conn}
	defer func() {
		b.remove(c)
		_ = conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.HasPrefix(line, "SUB ") {
			topic := strings.TrimPrefix(line, "SUB ")
			if validTopic(topic) {
				b.subscribe(c, topic)
			}
			continue
		}
		if strings.HasPrefix(line, "PUB ") {
			// Split at the second space only; payload is opaque text.
			rest := strings.TrimPrefix(line, "PUB ")
			separator := strings.IndexByte(rest, ' ')
			if separator < 1 {
				continue
			}
			topic, payload := rest[:separator], rest[separator+1:]
			if validTopic(topic) && payload != "" {
				b.publish(topic, payload)
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
		log.Printf("client read error: %v", err)
	}
}

func (b *broker) serve(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go b.handle(conn)
	}
}

func main() {
	flag.Usage = func() { fmt.Fprintf(os.Stderr, "Usage: %s <port>\n", os.Args[0]) }
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	port, err := strconv.Atoi(flag.Arg(0))
	if err != nil || port < 1 || port > 65535 {
		fmt.Fprintln(os.Stderr, "port must be an integer from 1 to 65535")
		os.Exit(2)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	log.Printf("Broker listening on %s", listener.Addr())
	if err := newBroker().serve(listener); err != nil {
		log.Fatal(err)
	}
}
