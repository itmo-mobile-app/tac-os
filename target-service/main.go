package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Observation follows spec/data-model.md. No aggregation is performed.
type Observation struct {
	TargetID   string  `json:"targetId"`
	ObserverID string  `json:"observerId"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Confidence float64 `json:"confidence"`
	Timestamp  int64   `json:"timestamp"`
}

type Store struct{ latest map[string]Observation }

func newStore() *Store              { return &Store{latest: make(map[string]Observation)} }
func (s *Store) save(o Observation) { s.latest[o.TargetID] = o }

func parseObservation(payload string) (Observation, error) {
	var o Observation
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return o, fmt.Errorf("invalid JSON: %w", err)
	}
	if len(raw) != 6 {
		return o, errors.New("Observation must contain exactly six fields")
	}
	for _, key := range []string{"targetId", "observerId", "latitude", "longitude", "confidence", "timestamp"} {
		if _, ok := raw[key]; !ok {
			return o, fmt.Errorf("missing field %s", key)
		}
	}
	if err := json.Unmarshal([]byte(payload), &o); err != nil {
		return o, fmt.Errorf("invalid field type: %w", err)
	}
	if strings.TrimSpace(o.TargetID) == "" || strings.TrimSpace(o.ObserverID) == "" {
		return o, errors.New("targetId and observerId must be nonempty")
	}
	if math.IsNaN(o.Latitude) || o.Latitude < -90 || o.Latitude > 90 || math.IsNaN(o.Longitude) || o.Longitude < -180 || o.Longitude > 180 {
		return o, errors.New("coordinates out of range")
	}
	if math.IsNaN(o.Confidence) || o.Confidence < 0 || o.Confidence > 1 {
		return o, errors.New("confidence out of range")
	}
	if o.Timestamp <= 0 {
		return o, errors.New("timestamp must be positive Unix milliseconds")
	}
	return o, nil
}

func handleMessage(line string, store *Store, logger *log.Logger) {
	const prefix = "MSG observations "
	if !strings.HasPrefix(line, prefix) {
		logger.Printf("ignoring unexpected message: %q", line)
		return
	}
	o, err := parseObservation(strings.TrimPrefix(line, prefix))
	if err != nil {
		logger.Printf("invalid observation: %v", err)
		return
	}
	store.save(o)
	logger.Printf("observation stored targetId=%q observerId=%q timestamp=%d (targets in memory=%d)", o.TargetID, o.ObserverID, o.Timestamp, len(store.latest))
}

func run(ctx context.Context, address string, logger *log.Logger, ready chan<- struct{}) error {
	dialer := net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect to Broker %s: %w", address, err)
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "SUB observations\n"); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	logger.Printf("connected to Broker %s; subscribed to observations", address)
	if ready != nil {
		close(ready)
	}
	go func() { <-ctx.Done(); _ = conn.Close() }()
	store := newStore()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		handleMessage(strings.TrimSuffix(scanner.Text(), "\r"), store, logger)
	}
	if ctx.Err() != nil {
		return nil
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Broker read: %w", err)
	}
	return errors.New("Broker disconnected")
}

func main() {
	address := flag.String("broker", "127.0.0.1:8080", "Broker TCP address host:port")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: target-service [-broker host:port]")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *address, log.Default(), nil); err != nil {
		log.Fatal(err)
	}
}
