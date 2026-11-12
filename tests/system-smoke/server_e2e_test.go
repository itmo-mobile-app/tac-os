//go:build server_e2e

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These tests drive executables and public protocols only. The build tag keeps
// unfinished acceptance criteria separate from the existing regression suite.
const acceptanceWait = 8 * time.Second

type wireTarget struct {
	TargetID    string  `json:"targetId"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Probability float64 `json:"probability"`
	LastSeenAt  int64   `json:"lastSeenAt"`
}

type brokerEvent struct {
	line string
	at   time.Time
	err  error
}

type acceptanceFixture struct {
	t       *testing.T
	ctx     context.Context
	s       suite
	dir     string
	address string
	db      string
	peer    net.Conn
	events  chan brokerEvent
}

func TestServerE2E(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	b, err := buildComponents(ctx, root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dbBinary := filepath.Join(t.TempDir(), "tacdb-server")
	cmd := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildvcs=false", "-o", dbBinary, "./cmd/tacdb-server")
	cmd.Dir, cmd.Env = filepath.Join(root, "server/dbms"), offlineEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build DBMS: %v\n%s", err, output)
	}

	t.Run("DBMSProtocol", func(t *testing.T) {
		f := newAcceptanceFixture(t)
		f.startDBMS(dbBinary)
		// A transport control, not evidence of Target Service integration.
		for _, query := range []string{
			"CREATE TABLE probe (id INTEGER, label TEXT, score REAL);",
			"INSERT INTO probe (id, label, score) VALUES (9223372036854775807, 'O''Reilly', 0.8);",
			"UPDATE probe SET score = 0.92 WHERE id = 9223372036854775807;",
		} {
			if r, err := queryDBMS(f.db, query); err != nil || !r.OK {
				t.Fatalf("%s: %+v, %v", query, r, err)
			}
		}
		r, err := queryDBMS(f.db, "SELECT id, label, score FROM probe;")
		if err != nil || !r.OK || len(r.Rows) != 1 || len(r.Rows[0]) != 3 {
			t.Fatalf("SELECT: %+v, %v", r, err)
		}
		if string(r.Rows[0][0]) != "9223372036854775807" || string(r.Rows[0][1]) != `"O'Reilly"` || string(r.Rows[0][2]) != "0.92" {
			t.Fatalf("SQL round trip: %+v", r.Rows)
		}
		r, err = queryDBMS(f.db, "SELECT * FROM missing_probe;")
		if err != nil || r.OK || r.Error.Code != "SQL_ERROR" {
			t.Fatalf("invalid SQL: %+v, %v", r, err)
		}
		t.Log("real DBMS CREATE/INSERT/UPDATE/SELECT, int64, TEXT escaping and SQL_ERROR verified")
	})

	for _, name := range []string{"OneObserver", "TwoObserversAndDuplicate", "InvalidObservation", "TimeDecay", "MapRedraw", "SQLIntegration"} {
		t.Run(name, func(t *testing.T) {
			if name == "SQLIntegration" && (os.Getenv("TACOS_E2E_TARGET_DBMS_ARGS") == "" || os.Getenv("TACOS_E2E_TARGET_SELECT") == "") {
				t.Skip("BLOCKED-CONTRACT: configure documented Target Service DBMS arguments and Target SELECT; spec/dbms-protocol.md defines framing, not service CLI or domain schema")
			}
			f := newAcceptanceFixture(t)
			f.startDBMS(dbBinary)
			_, f.address, err = startBroker(f.ctx, &f.s, b.broker)
			if err != nil {
				t.Fatal(err)
			}
			f.subscribe()
			var m *process
			if name == "MapRedraw" {
				m = f.startMap(root, b)
			}
			args := []string{"-broker", f.address}
			if raw := os.Getenv("TACOS_E2E_TARGET_DBMS_ARGS"); raw != "" {
				var extra []string
				if err := json.Unmarshal([]byte(raw), &extra); err != nil || len(extra) == 0 || !strings.Contains(raw, "{dbms}") {
					t.Fatal("TACOS_E2E_TARGET_DBMS_ARGS must be a nonempty JSON array containing {dbms}; no shell is used")
				}
				for _, arg := range extra {
					args = append(args, strings.ReplaceAll(arg, "{dbms}", f.db))
				}
			}
			if _, err := f.s.start("Target Service", b.target, args...); err != nil {
				t.Fatal(err)
			}
			stamp := time.Now().UnixMilli()
			first, at := f.awaitTarget("17", func() { f.publish(observation("17", "A", 0.8, stamp)) })
			checkTarget(t, first, at, stamp, 0.8)
			switch name {
			case "TwoObserversAndDuplicate":
				f.publish(observation("17", "B", 0.6, stamp))
				second, at := f.nextTarget("17", acceptanceWait, func(v wireTarget) bool { return v.Probability > first.Probability })
				checkTarget(t, second, at, stamp, 0.8, 0.6)
				f.publish(observation("17", "A", 0.8, stamp))
				f.observe(6*time.Second, func(v wireTarget, at time.Time) {
					if v.TargetID == "17" {
						checkTarget(t, v, at, stamp, 0.8, 0.6)
					}
				})
			case "InvalidObservation":
				f.publish(observation("40", "bad-confidence", 1.2, stamp))
				f.publish(strings.Replace(observation("41", "bad-coordinate", 0.8, stamp), `"latitude":59.95`, `"latitude":91`, 1))
				f.publish(observation("42", "future", 0.8, time.Now().Add(time.Minute).UnixMilli()))
				f.publish(observation("43", "expired", 0.8, time.Now().Add(-3*time.Minute).UnixMilli()))
				f.publish(`{"targetId":"44","observerId":`)
				f.publish(observation("18", "healthy-control", 0.8, time.Now().UnixMilli()))
				// A positive control is mandatory: silence from an unimplemented
				// service must never count as successful validation.
				f.nextTarget("18", acceptanceWait, nil)
				f.observe(6*time.Second, func(v wireTarget, _ time.Time) { f.rejectInvalidTarget(v) })
			case "TimeDecay":
				v, at := f.nextTarget("17", acceptanceWait, func(v wireTarget) bool { return v.Probability < first.Probability-0.001 })
				checkTarget(t, v, at, stamp, 0.8)
				if v.LastSeenAt != first.LastSeenAt {
					t.Fatalf("decay changed lastSeenAt: %+v -> %+v", first, v)
				}
			case "MapRedraw":
				if err := waitFor(f.ctx, "Map redraw after server-produced Target", f.s.processes, func() (bool, error) {
					logs := m.logs.String()
					return strings.Contains(logs, "Target updated: 17\n") && strings.Contains(logs, "-> (480, 320), probability ") && strings.Contains(logs, "1 visible, 0 outside map"), nil
				}); err != nil {
					t.Fatal(err)
				}
				if err := m.signal(syscall.SIGINT); err != nil {
					t.Fatal(err)
				}
				if err := m.wait(f.ctx); err != nil {
					t.Fatal(err)
				}
				if err := wmCommand(filepath.Join(f.dir, "wm.sock"), "SHOW", false); err != nil {
					t.Fatalf("Map window release: %v", err)
				}
			case "SQLIntegration":
				query := os.Getenv("TACOS_E2E_TARGET_SELECT")
				if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "SELECT ") {
					t.Fatal("TACOS_E2E_TARGET_SELECT must be a read-only SELECT projecting target ID, latitude, longitude, probability, last-seen timestamp, in that order")
				}
				if err := waitFor(f.ctx, "Target Service writes visible through real SQL", f.s.processes, func() (bool, error) {
					r, err := queryDBMS(f.db, query)
					if err != nil {
						return false, err
					}
					for _, row := range r.Rows {
						if len(row) != 5 || string(row[0]) != `"17"` {
							continue
						}
						var v wireTarget
						for i, dest := range []any{&v.TargetID, &v.Latitude, &v.Longitude, &v.Probability, &v.LastSeenAt} {
							if err := json.Unmarshal(row[i], dest); err != nil {
								return false, err
							}
						}
						// The SQL probe never writes domain data. This row must
						// have been created by the actual Target Service process.
						checkTarget(t, v, time.Now(), stamp, 0.8)
						return true, nil
					}
					return false, nil
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if os.Getenv("TACOS_E2E_TARGET_DBMS_ARGS") == "" || os.Getenv("TACOS_E2E_TARGET_SELECT") == "" {
		t.Error("INCOMPLETE ACCEPTANCE: SQL integration contract is not configured; its SKIP must not produce an overall green acceptance run")
	}
}

func newAcceptanceFixture(t *testing.T) *acceptanceFixture {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "tac-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	f := &acceptanceFixture{t: t, ctx: ctx, dir: dir}
	t.Cleanup(func() {
		cancel()
		if err := f.s.cleanup(); err != nil {
			t.Errorf("CLEANUP: %v", err)
		}
		for _, p := range f.s.processes {
			if err := syscall.Kill(p.cmd.Process.Pid, 0); err != syscall.ESRCH {
				t.Errorf("CLEANUP: %s PID remains: %v", p.name, err)
			}
		}
		if t.Failed() {
			t.Log(f.s.diagnostics())
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
		if err := checkAbsent(dir); err != nil {
			t.Error(err)
		}
		t.Log("cleanup checked: processes waited for, PIDs absent, sockets absent, private directory removed")
	})
	return f
}

func (f *acceptanceFixture) startDBMS(binary string) {
	f.t.Helper()
	f.db = filepath.Join(f.dir, "db.sock")
	f.s.sockets = append(f.s.sockets, f.db)
	if _, err := f.s.start("DBMS", binary, "-socket", f.db); err != nil {
		f.t.Fatal(err)
	}
	if err := waitFor(f.ctx, "real DBMS protocol readiness", f.s.processes, func() (bool, error) {
		if _, err := os.Stat(f.db); err != nil {
			return false, nil
		}
		r, err := queryDBMS(f.db, "SELECT * FROM missing_readiness_probe;")
		return err == nil && !r.OK && r.Error.Code == "SQL_ERROR", err
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *acceptanceFixture) subscribe() {
	f.t.Helper()
	conn, err := net.DialTimeout("tcp", f.address, time.Second)
	if err != nil {
		f.t.Fatal(err)
	}
	f.peer, f.events = conn, make(chan brokerEvent, 128)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s := bufio.NewScanner(conn)
		s.Buffer(make([]byte, 4096), 1024*1024)
		for s.Scan() {
			select {
			case f.events <- brokerEvent{line: s.Text(), at: time.Now()}:
			case <-f.ctx.Done():
				return
			}
		}
		select {
		case f.events <- brokerEvent{err: fmt.Errorf("Broker stream closed: %v", s.Err())}:
		case <-f.ctx.Done():
		}
	}()
	f.t.Cleanup(func() {
		_ = conn.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			f.t.Error("Broker reader did not stop")
		}
	})
	// SUB and the echo are ordered on one connection. This is a wire-level
	// readiness barrier for the test subscriber, not a fabricated Target.
	f.write("SUB targets.updated\nSUB e2e.ready\nPUB e2e.ready {\"ready\":true}\n")
	select {
	case event := <-f.events:
		if event.err != nil || event.line != `MSG e2e.ready {"ready":true}` {
			f.t.Fatalf("subscriber barrier: %+v", event)
		}
	case <-f.ctx.Done():
		f.t.Fatal("subscriber readiness timeout")
	}
}

func (f *acceptanceFixture) write(line string) {
	f.t.Helper()
	for _, p := range f.s.processes {
		if err := p.alive(); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := f.peer.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		f.t.Fatal(err)
	}
	if _, err := io.WriteString(f.peer, line); err != nil {
		f.t.Fatal(err)
	}
}

func (f *acceptanceFixture) publish(payload string) { f.write("PUB observations " + payload + "\n") }

func observation(target, observer string, confidence float64, timestamp int64) string {
	return fmt.Sprintf(`{"targetId":%q,"observerId":%q,"latitude":59.95,"longitude":30.311,"confidence":%g,"timestamp":%d}`, target, observer, confidence, timestamp)
}

func (f *acceptanceFixture) decode(event brokerEvent) wireTarget {
	f.t.Helper()
	if event.err != nil {
		f.t.Fatal(event.err)
	}
	const prefix = "MSG targets.updated "
	if !strings.HasPrefix(event.line, prefix) {
		f.t.Fatalf("unexpected Broker event: %s", event.line)
	}
	payload := []byte(strings.TrimPrefix(event.line, prefix))
	var fields map[string]json.RawMessage
	var v wireTarget
	if err := json.Unmarshal(payload, &fields); err != nil || len(fields) != 5 {
		f.t.Fatalf("Target must have exactly five fields: %s", payload)
	}
	for _, key := range []string{"targetId", "latitude", "longitude", "probability", "lastSeenAt"} {
		if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) || len(fields[key]) == 0 {
			f.t.Fatalf("missing/null Target field %s: %s", key, payload)
		}
	}
	if err := json.Unmarshal(payload, &v); err != nil || v.TargetID == "" || v.LastSeenAt <= 0 || math.IsNaN(v.Probability) || v.Probability < 0 || v.Probability > 1 {
		f.t.Fatalf("invalid Target: %s (%v)", payload, err)
	}
	f.rejectInvalidTarget(v)
	f.t.Logf("received %s", payload)
	return v
}

func (f *acceptanceFixture) rejectInvalidTarget(v wireTarget) {
	f.t.Helper()
	if v.TargetID >= "40" && v.TargetID <= "44" {
		f.t.Fatalf("invalid Observation produced Target: %+v", v)
	}
}

func (f *acceptanceFixture) awaitTarget(id string, publish func()) (wireTarget, time.Time) {
	f.t.Helper()
	deadline := time.NewTimer(acceptanceWait)
	defer deadline.Stop()
	retry := time.NewTicker(200 * time.Millisecond)
	defer retry.Stop()
	publish()
	for {
		select {
		case event := <-f.events:
			v := f.decode(event)
			if v.TargetID == id {
				return v, event.at
			}
		case <-retry.C:
			// Same observer/timestamp: retries cover subscription races and
			// must not become independent evidence (spec/target-aggregation.md).
			publish()
		case <-deadline.C:
			f.t.Fatalf("ACCEPTANCE: no server-produced targets.updated for %s within %s; sent only Observation, no Target injection", id, acceptanceWait)
		case <-f.ctx.Done():
			f.t.Fatal(f.ctx.Err())
		}
	}
}

func (f *acceptanceFixture) nextTarget(id string, timeout time.Duration, match func(wireTarget) bool) (wireTarget, time.Time) {
	f.t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case event := <-f.events:
			v := f.decode(event)
			if v.TargetID == id && (match == nil || match(v)) {
				return v, event.at
			}
		case <-timer.C:
			f.t.Fatalf("ACCEPTANCE: no matching Target for %s within %s", id, timeout)
		case <-f.ctx.Done():
			f.t.Fatal(f.ctx.Err())
		}
	}
}

func (f *acceptanceFixture) observe(duration time.Duration, check func(wireTarget, time.Time)) {
	f.t.Helper()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	for {
		select {
		case event := <-f.events:
			check(f.decode(event), event.at)
		case <-timer.C:
			return
		case <-f.ctx.Done():
			f.t.Fatal(f.ctx.Err())
		}
	}
}

func checkTarget(t *testing.T, v wireTarget, received time.Time, stamp int64, confidences ...float64) {
	t.Helper()
	if math.Abs(v.Latitude-59.95) > 1e-9 || math.Abs(v.Longitude-30.311) > 1e-9 || v.LastSeenAt != stamp {
		t.Fatalf("Target location/time does not match input: %+v, timestamp=%d", v, stamp)
	}
	// A service computes before network receipt. Allow at most one second of
	// transport/scheduling lag; do not demand deterministic t0 values at wall time.
	probability := func(at time.Time) float64 {
		age := math.Max(0, float64(at.UnixMilli()-stamp))
		product := 1.0
		for _, c := range confidences {
			product *= 1 - c*math.Exp2(-age/30000)
		}
		return 1 - product
	}
	low, high := probability(received)-1e-6, probability(received.Add(-time.Second))+1e-6
	if v.Probability < low || v.Probability > high {
		t.Fatalf("probability=%g outside wall-clock contract interval [%g,%g]: %+v", v.Probability, low, high, v)
	}
}

func (f *acceptanceFixture) startMap(root string, b binaries) *process {
	f.t.Helper()
	command, events := filepath.Join(f.dir, "wm.sock"), filepath.Join(f.dir, "events.sock")
	f.s.sockets = append(f.s.sockets, command, events)
	if _, err := f.s.start("WM", b.wm, "-socket", command, "-events", events); err != nil {
		f.t.Fatal(err)
	}
	if err := waitFor(f.ctx, "WM readiness", f.s.processes, func() (bool, error) {
		_, a := os.Stat(command)
		_, b := os.Stat(events)
		return a == nil && b == nil, nil
	}); err != nil {
		f.t.Fatal(err)
	}
	m, err := f.s.start("Map", b.tacticalMap, "-broker", f.address, "-config", filepath.Join(root, "app/map/assets/test-map.json"), "-socket", command, "-events", events)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := waitFor(f.ctx, "empty live Map and subscription", f.s.processes, func() (bool, error) {
		logs := m.logs.String()
		return strings.Contains(logs, "Map shown.") && strings.Contains(logs, "0 visible, 0 outside map") && strings.Contains(logs, "SUB targets.updated sent"), nil
	}); err != nil {
		f.t.Fatal(err)
	}
	return m
}

type sqlReply struct {
	OK      bool                `json:"ok"`
	Columns []json.RawMessage   `json:"columns"`
	Rows    [][]json.RawMessage `json:"rows"`
	Error   struct {
		Code string `json:"code"`
	} `json:"error"`
}

func queryDBMS(socket, query string) (sqlReply, error) {
	var reply sqlReply
	conn, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		return reply, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		return reply, err
	}
	var request bytes.Buffer
	if err := binary.Write(&request, binary.LittleEndian, uint32(len(query))); err != nil {
		return reply, err
	}
	request.WriteString(query)
	if _, err := io.Copy(conn, &request); err != nil {
		return reply, err
	}
	var size uint32
	if err := binary.Read(conn, binary.LittleEndian, &size); err != nil {
		return reply, err
	}
	if size == 0 || size > 1024*1024 {
		return reply, fmt.Errorf("invalid DBMS frame length %d", size)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(conn, body); err != nil {
		return reply, err
	}
	err = json.Unmarshal(body, &reply)
	return reply, err
}
