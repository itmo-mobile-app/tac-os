package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tac-target-service/aggregation"
	"tac-target-service/dbclient"
)

type mockDB struct {
	statements []string
	fail       string
	malformed  bool
	rows       map[string]bool
}

func (m *mockDB) Exec(_ context.Context, sql string) (dbclient.Result, error) {
	m.statements = append(m.statements, sql)
	if m.fail != "" && strings.Contains(sql, m.fail) {
		return dbclient.Result{}, errors.New("disconnected mid-response")
	}
	if strings.HasPrefix(sql, "SELECT target_id") {
		if m.malformed {
			return dbclient.Result{OK: true, Columns: []dbclient.Column{{Name: "bad", Type: "TEXT"}}}, nil
		}
		r := dbclient.Result{OK: true, Columns: []dbclient.Column{{Name: "target_id", Type: "TEXT"}}}
		for id := range m.rows {
			if strings.Contains(sql, dbclient.SQLString(id)) {
				r.Rows = [][]any{{id}}
			}
		}
		return r, nil
	}
	if strings.HasPrefix(sql, "INSERT INTO targets") {
		if m.rows == nil {
			m.rows = map[string]bool{}
		}
		if strings.Contains(sql, "'a'") {
			m.rows["a"] = true
		}
	}
	return dbclient.Result{OK: true}, nil
}
func obs(id string, confidence float64, ts int64) string {
	b, _ := json.Marshal(aggregation.Observation{TargetID: "a", ObserverID: id, Latitude: 0, Longitude: 0, Confidence: confidence, Timestamp: ts})
	return string(b)
}
func TestPersistenceDuplicateAndDecay(t *testing.T) {
	now := time.UnixMilli(1800000000000)
	m := &mockDB{}
	var updates []aggregation.Target
	s := newService(m, func(t aggregation.Target) error { updates = append(updates, t); return nil }, func() time.Time { return now })
	ctx := context.Background()
	if err := s.initSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id    string
		score float64
	}{{"o1", 0.8}, {"o2", 0.6}} {
		ok, err := s.observation(ctx, obs(tc.id, tc.score, now.UnixMilli()))
		if err != nil || !ok {
			t.Fatalf("ingest: %v %t", err, ok)
		}
	}
	if math.Abs(updates[len(updates)-1].Probability-0.92) > 1e-9 {
		t.Fatalf("probability %+v", updates)
	}
	n := len(m.statements)
	accepted, err := s.observation(ctx, obs("o1", 1, now.UnixMilli()))
	if err != nil || accepted || len(m.statements) != n {
		t.Fatalf("duplicate was persisted %v %v", err, accepted)
	}
	accepted, err = s.observation(ctx, obs("o1", 1, now.UnixMilli()-1))
	if err != nil || accepted || len(m.statements) != n {
		t.Fatal("older report was persisted")
	}
	now = now.Add(30 * time.Second)
	if err := s.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if math.Abs(updates[len(updates)-1].Probability-0.58) > 1e-9 {
		t.Fatalf("decay: %+v", updates[len(updates)-1])
	}
	now = now.Add(90 * time.Second)
	if err := s.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if updates[len(updates)-1].Probability != 0 {
		t.Fatal("missing stale transition")
	}
	n = len(updates)
	if err := s.tick(ctx); err != nil || len(updates) != n {
		t.Fatalf("stale emitted again: %v", err)
	}
}
func TestFailureDoesNotPublish(t *testing.T) {
	for _, fail := range []string{"INSERT INTO observations", "SELECT target_id", "INSERT INTO targets"} {
		t.Run(fail, func(t *testing.T) {
			m := &mockDB{fail: fail}
			publishes := 0
			s := newService(m, func(aggregation.Target) error { publishes++; return nil }, func() time.Time { return time.UnixMilli(1800000000000) })
			_, err := s.observation(context.Background(), obs("o1", 0.8, 1800000000000))
			if err == nil || publishes != 0 {
				t.Fatalf("expected fail-stop, got err %v publications %d", err, publishes)
			}
		})
	}
	m := &mockDB{malformed: true}
	s := newService(m, func(aggregation.Target) error { t.Fatal("published on invalid SELECT"); return nil }, func() time.Time { return time.UnixMilli(1800000000000) })
	if _, err := s.observation(context.Background(), obs("o1", 0.8, 1800000000000)); err == nil {
		t.Fatal("accepted invalid DBMS response")
	}
}
func TestMalformedMessagesAndQuotedIDs(t *testing.T) {
	m := &mockDB{}
	updates := 0
	now := time.UnixMilli(1800000000000)
	s := newService(m, func(aggregation.Target) error { updates++; return nil }, func() time.Time { return now })
	for _, payload := range []string{"{}", "bad", `{"targetId":"a","observerId":"x","latitude":0,"longitude":0,"confidence":9,"timestamp":1800000000000}`, obs("x", .8, now.UnixMilli()+6000), obs("x", .8, now.UnixMilli()-120000)} {
		_, err := s.observation(context.Background(), payload)
		if err == nil {
			t.Fatalf("accepted %s", payload)
		}
	}
	if updates != 0 || len(m.statements) != 0 {
		t.Fatal("invalid observation reached persistence")
	}
	valid := strings.Replace(obs("o1", 0.8, now.UnixMilli()), `"a"`, `"O'Reilly"`, 1)
	if _, err := s.observation(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.statements[0], "'O''Reilly'") {
		t.Fatalf("SQL not escaped: %s", m.statements[0])
	}
}
func TestUnavailableBroker(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	err = run(context.Background(), config{broker: addr}, log.New(&bytes.Buffer{}, "", 0), nil)
	if err == nil || !strings.Contains(err.Error(), "connect to Broker") {
		t.Fatalf("got %v", err)
	}
}
func TestUnavailableDBMSDoesNotSubscribe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		b := make([]byte, 128)
		n, _ := conn.Read(b)
		received <- string(b[:n])
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = run(ctx, config{broker: ln.Addr().String(), socket: filepath.Join(t.TempDir(), "missing-db.sock")}, log.New(&bytes.Buffer{}, "", 0), nil)
	if err == nil || !strings.Contains(err.Error(), "connect DBMS") {
		t.Fatalf("should fail on unavailable DBMS: %v", err)
	}
	select {
	case msg := <-received:
		if msg != "" {
			t.Fatalf("subscribed or published without DBMS: %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish")
	}
}

func startBinary(t *testing.T, dir, pkg string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "component")
	build := exec.Command("go", "build", "-o", bin, pkg)
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v %s", pkg, err, out)
	}
	cmd := exec.Command(bin, args...)
	logs := &bytes.Buffer{}
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd, logs
}
func TestSystemRealBrokerAndDBMS(t *testing.T) {
	if testing.Short() {
		t.Skip("system E2E")
	}
	root := filepath.Join("..")
	if _, err := os.Stat(filepath.Join(root, "server", "dbms", "go.mod")); err != nil {
		t.Skip("DBMS server source not found")
	}
	socket := filepath.Join(t.TempDir(), "db.sock")
	_, dbLogs := startBinary(t, filepath.Join(root, "server", "dbms"), "./cmd/tacdb-server", "-socket", socket)
	var db *dbclient.Client
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(socket); err == nil {
			db = dbclient.New(socket, 1*time.Second)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if db == nil {
		t.Fatalf("DBMS not listening: %s", dbLogs)
	}
	defer db.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	brokerCmd, brokerLogs := startBinary(t, filepath.Join(root, "broker"), ".", fmt.Sprint(port))
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	var pub net.Conn
	for i := 0; i < 100; i++ {
		pub, err = net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("broker unavailable: %s %v", brokerLogs, err)
	}
	defer pub.Close()
	sub, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	fmt.Fprint(sub, "SUB targets.updated\n")
	// Ensure Broker has processed the subscription before sending observations.
	time.Sleep(80 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	var logs bytes.Buffer
	go func() {
		done <- run(ctx, config{broker: addr, socket: socket, interval: 100 * time.Millisecond}, log.New(&logs, "", 0), ready)
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("target failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("target startup timeout")
	}
	// Give subscription time to be installed by real Broker.
	time.Sleep(80 * time.Millisecond)
	now := time.Now().UnixMilli()
	for _, o := range []string{obs("one", .8, now), obs("two", .6, now)} {
		if _, err := fmt.Fprintf(pub, "PUB observations %s\n", o); err != nil {
			t.Fatal(err)
		}
		time.Sleep(80 * time.Millisecond)
	}
	scanner := bufio.NewScanner(sub)
	deadline := time.Now().Add(4 * time.Second)
	received := 0
	found92 := false
	foundDecay := false
	for time.Now().Before(deadline) {
		_ = sub.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				break
			}
			break
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "MSG targets.updated ") {
			continue
		}
		var target aggregation.Target
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "MSG targets.updated ")), &target); err != nil {
			t.Fatal(err)
		}
		received++
		if target.Probability > .919 && target.Probability < .921 {
			found92 = true
		}
		if found92 && target.Probability < .92 {
			foundDecay = true
			break
		}
	}
	if !found92 || !foundDecay {
		t.Fatalf("expected 2-observer confidence and timer decay; found92=%v decay=%v received=%d", found92, foundDecay, received)
	}
	r, err := db.Exec(context.Background(), "SELECT * FROM observations;")
	if err != nil || len(r.Rows) != 2 {
		t.Fatalf("DB observations: %v %+v", err, r)
	}
	r, err = db.Exec(context.Background(), "SELECT * FROM targets WHERE target_id = 'a';")
	if err != nil || len(r.Rows) != 1 {
		t.Fatalf("DB target: %v %+v", err, r)
	}
	// Broker loss must be visible to the service (not silently ignored).
	_ = brokerCmd.Process.Kill()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "Broker") {
			t.Fatalf("broker loss should fail: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("service did not notice Broker disconnect")
	}
}

// TestAmbiguousWriteNoReplay complements the client's wire-level no-replay test.
func TestAmbiguousWriteNoReplay(t *testing.T) {
	for _, failedSQL := range []string{"INSERT INTO observations", "INSERT INTO targets", "UPDATE targets"} {
		t.Run(failedSQL, func(t *testing.T) {
			db := &mockDB{}
			publications := 0
			s := newService(db, func(aggregation.Target) error { publications++; return nil }, func() time.Time { return time.UnixMilli(1800000000000) })
			if failedSQL == "UPDATE targets" {
				if _, err := s.observation(context.Background(), obs("first", 0.8, 1800000000000)); err != nil {
					t.Fatal(err)
				}
			}
			before := publications
			db.statements = nil
			db.fail = failedSQL
			if _, err := s.observation(context.Background(), obs("second", 0.6, 1800000000000)); err == nil {
				t.Fatal("ambiguous write reported success")
			}
			writes := 0
			for _, sql := range db.statements {
				if strings.HasPrefix(sql, failedSQL) {
					writes++
				}
			}
			if writes != 1 || publications != before {
				t.Fatalf("replay or publish: writes=%d publications=%d->%d", writes, before, publications)
			}
		})
	}
}
