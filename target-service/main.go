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
	"strconv"
	"strings"
	"syscall"
	"time"

	"tac-target-service/aggregation"
	"tac-target-service/dbclient"
	"tac-target-service/validation"
)

// SQL executor is deliberately the existing framed DBMS client, not a database driver.
var errBadObservation = errors.New("invalid observation")

type sqlExecutor interface {
	Exec(context.Context, string) (dbclient.Result, error)
}

type service struct {
	db  sqlExecutor
	agg *aggregation.Aggregator
	// Duplicate suppression is kept in memory like the aggregation state.
	seen    map[string]map[string]int64
	publish func(aggregation.Target) error
	now     func() time.Time
}

func newService(db sqlExecutor, publish func(aggregation.Target) error, now func() time.Time) *service {
	return &service{db: db, agg: aggregation.NewAggregator(), seen: make(map[string]map[string]int64), publish: publish, now: now}
}

func (s *service) exec(ctx context.Context, sql string) (dbclient.Result, error) {
	r, err := s.db.Exec(ctx, sql)
	if err != nil {
		return r, fmt.Errorf("DBMS SQL %q: %w", sql, err)
	}
	if !r.OK {
		return r, fmt.Errorf("DBMS rejected SQL %q", sql)
	}
	return r, nil
}

// initSchema must succeed before subscribing or emitting any updates. DBMS v1
// is process-local and in-memory: these tables intentionally are not durable.
func (s *service) initSchema(ctx context.Context) error {
	for _, stmt := range []string{
		"CREATE TABLE observations (target_id TEXT, observer_id TEXT, latitude REAL, longitude REAL, confidence REAL, timestamp INTEGER);",
		"CREATE TABLE targets (target_id TEXT, latitude REAL, longitude REAL, probability REAL, last_seen_at INTEGER);",
	} {
		if _, err := s.exec(ctx, stmt); err != nil {
			return fmt.Errorf("initialize schema: %w", err)
		}
	}
	return nil
}

func sqlReal(x float64) string {
	// Fixed-point formatting avoids exponent syntax unsupported by SQL v1.
	if math.IsNaN(x) || math.IsInf(x, 0) {
		panic("nonfinite SQL REAL")
	}
	v := strconv.FormatFloat(x, 'f', -1, 64)
	if !strings.Contains(v, ".") {
		v += ".0"
	}
	if v == "-0.0" {
		return "0.0"
	}
	return v
}
func (s *service) saveObservation(ctx context.Context, o aggregation.Observation) error {
	stmt := fmt.Sprintf("INSERT INTO observations (target_id, observer_id, latitude, longitude, confidence, timestamp) VALUES (%s, %s, %s, %s, %s, %d);", dbclient.SQLString(o.TargetID), dbclient.SQLString(o.ObserverID), sqlReal(o.Latitude), sqlReal(o.Longitude), sqlReal(o.Confidence), o.Timestamp)
	_, err := s.exec(ctx, stmt)
	return err
}
func (s *service) persistTarget(ctx context.Context, t aggregation.Target) error {
	key := dbclient.SQLString(t.TargetID)
	r, err := s.exec(ctx, "SELECT target_id FROM targets WHERE target_id = "+key+";")
	if err != nil {
		return err
	}
	if len(r.Columns) != 1 || r.Columns[0].Name != "target_id" || r.Columns[0].Type != "TEXT" {
		return errors.New("unexpected DBMS target SELECT columns")
	}
	if len(r.Rows) > 1 {
		return errors.New("multiple target rows for same target_id")
	}
	for _, row := range r.Rows {
		if len(row) != 1 || row[0] != t.TargetID {
			return errors.New("unexpected DBMS target SELECT row")
		}
	}
	values := fmt.Sprintf("%s, %s, %s, %d", sqlReal(t.Latitude), sqlReal(t.Longitude), sqlReal(t.Probability), t.LastSeenAt)
	var stmt string
	if len(r.Rows) == 0 {
		stmt = fmt.Sprintf("INSERT INTO targets (target_id, latitude, longitude, probability, last_seen_at) VALUES (%s, %s);", key, values)
	} else {
		stmt = fmt.Sprintf("UPDATE targets SET latitude = %s, longitude = %s, probability = %s, last_seen_at = %d WHERE target_id = %s;", sqlReal(t.Latitude), sqlReal(t.Longitude), sqlReal(t.Probability), t.LastSeenAt, key)
	}
	_, err = s.exec(ctx, stmt)
	return err
}
func asAggregation(o validation.Observation) aggregation.Observation {
	return aggregation.Observation{TargetID: o.TargetID, ObserverID: o.ObserverID, Latitude: o.Latitude, Longitude: o.Longitude, Confidence: o.Confidence, Timestamp: o.Timestamp}
}
func (s *service) observation(ctx context.Context, payload string) (bool, error) {
	v, err := validation.ParseObservation([]byte(payload))
	if err != nil {
		return false, fmt.Errorf("%w: %v", errBadObservation, err)
	}
	o := asAggregation(v)
	now := s.now().UnixMilli()
	if o.Timestamp <= 0 || o.Timestamp > now+aggregation.FutureToleranceMs || now-o.Timestamp >= aggregation.MaxAgeMs {
		return false, fmt.Errorf("%w: timestamp out of range", errBadObservation)
	}
	norm := o.Timestamp
	if norm > now {
		norm = now
	}
	if m := s.seen[o.TargetID]; m != nil && norm <= m[o.ObserverID] {
		return false, nil
	}
	t, err := s.agg.ProcessObservation(o, now)
	if err != nil {
		return false, fmt.Errorf("%w: %v", errBadObservation, err)
	}
	// A failed statement is fatal: no publish or automatic retries after unknown outcome.
	if err := s.saveObservation(ctx, o); err != nil {
		return false, err
	}
	if s.seen[o.TargetID] == nil {
		s.seen[o.TargetID] = make(map[string]int64)
	}
	s.seen[o.TargetID][o.ObserverID] = norm
	if t != nil {
		if err := s.persistTarget(ctx, *t); err != nil {
			return false, err
		}
		if err := s.publish(*t); err != nil {
			return false, fmt.Errorf("publish target: %w", err)
		}
	}
	return true, nil
}
func (s *service) tick(ctx context.Context) error {
	targets, _ := s.agg.RecalculateAll(s.now().UnixMilli())
	for _, t := range targets {
		if err := s.persistTarget(ctx, t); err != nil {
			return err
		}
		if err := s.publish(t); err != nil {
			return fmt.Errorf("publish decay: %w", err)
		}
	}
	return nil
}

type config struct {
	broker, socket string
	interval       time.Duration
}

func run(ctx context.Context, c config, logger *log.Logger, ready chan<- struct{}) error {
	db := dbclient.New(c.socket, dbclient.DefaultTimeout)
	defer db.Close()
	dialer := net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", c.broker)
	if err != nil {
		return fmt.Errorf("connect to Broker %s: %w", c.broker, err)
	}
	defer conn.Close()
	// A dead Broker on publication is a hard error, never a simulated success.
	pub := func(t aggregation.Target) error {
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		if err = conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
			return err
		}
		msg := "PUB targets.updated " + string(b) + "\n"
		n, err := io.WriteString(conn, msg)
		if err == nil && n != len(msg) {
			return io.ErrShortWrite
		}
		return err
	}
	s := newService(db, pub, time.Now)
	if err := s.initSchema(ctx); err != nil {
		return err
	}
	if err = conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	if _, err = io.WriteString(conn, "SUB observations\n"); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	// Dedicated reader lets the main loop recalculate even without new observations.
	messages := make(chan string, 32)
	readErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			select {
			case messages <- strings.TrimSuffix(scanner.Text(), "\r"):
			case <-ctx.Done():
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		if err := scanner.Err(); err != nil {
			readErr <- fmt.Errorf("Broker read: %w", err)
		} else {
			readErr <- errors.New("Broker disconnected")
		}
	}()
	if c.interval <= 0 {
		c.interval = time.Duration(aggregation.RecalcIntervalMs) * time.Millisecond
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	logger.Printf("connected to Broker %s and DBMS %s; subscribed to observations", c.broker, c.socket)
	if ready != nil {
		close(ready)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErr:
			return err
		case line := <-messages:
			if !strings.HasPrefix(line, "MSG observations ") {
				logger.Printf("ignoring unexpected message: %q", line)
				continue
			}
			accepted, err := s.observation(ctx, strings.TrimPrefix(line, "MSG observations "))
			if err != nil {
				if errors.Is(err, errBadObservation) {
					logger.Printf("%v", err)
					continue
				}
				return err
			}
			if accepted {
				logger.Printf("observation processed")
			}
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				return err
			}
		}
	}
}
func main() {
	broker := flag.String("broker", "127.0.0.1:8080", "Broker TCP host:port")
	db := flag.String("dbms", dbclient.DefaultSocket, "DBMS Unix socket")
	interval := flag.Duration("interval", time.Duration(aggregation.RecalcIntervalMs)*time.Millisecond, "time decay recalculation interval")
	flag.Parse()
	if flag.NArg() != 0 || *interval <= 0 {
		fmt.Fprintln(os.Stderr, "usage: target-service [-broker host:port] [-dbms socket] [-interval duration]")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, config{*broker, *db, *interval}, log.Default(), nil); err != nil {
		log.Fatal(err)
	}
}
