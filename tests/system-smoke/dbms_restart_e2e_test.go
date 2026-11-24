//go:build server_e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestDBMSRestartE2E checks #147; recovery of Target Service itself is #148.
func TestDBMSRestartE2E(t *testing.T) {
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
	for _, signal := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		t.Run(signal.String(), func(t *testing.T) {
			f := newAcceptanceFixture(t)
			f.db = filepath.Join(f.dir, "db.sock")
			data := filepath.Join(f.dir, "state.json")
			f.s.sockets = append(f.s.sockets, f.db)
			launch := func() *process {
				t.Helper()
				p, err := f.s.start("persistent DBMS", b.dbms, "-socket", f.db, "-data", data)
				if err != nil {
					t.Fatal(err)
				}
				if err := waitFor(f.ctx, "persistent DBMS readiness", []*process{p}, func() (bool, error) {
					if !strings.Contains(p.logs.String(), "Listening on ") {
						return false, nil
					}
					r, err := queryDBMS(f.db, "SELECT * FROM missing_readiness_probe;")
					return err == nil && !r.OK && r.Error.Code == "SQL_ERROR", err
				}); err != nil {
					t.Fatal(err)
				}
				return p
			}
			db := launch()
			_, f.address, err = startBroker(f.ctx, &f.s, b.broker)
			if err != nil {
				t.Fatal(err)
			}
			f.subscribe()
			target, err := f.s.start("Target Service", b.target, "-broker", f.address, "-dbms", f.db, "-interval", "1h")
			if err != nil {
				t.Fatal(err)
			}
			if err := waitFor(f.ctx, "Target subscription", []*process{target}, func() (bool, error) { return strings.Contains(target.logs.String(), "subscribed to observations"), nil }); err != nil {
				t.Fatal(err)
			}
			stamp := time.Now().UnixMilli()
			// Only Observation is published. The test never writes domain tables.
			// Repeated identical Observation covers a Broker subscription race, not SQL retries.
			v, at := f.awaitTarget("17", func() { f.publish(observation("17", "restart-observer", 0.8, stamp)) })
			checkTarget(t, v, at, stamp, 0.8)
			if err := target.stop(); err != nil {
				t.Fatal(err)
			}
			queries := []string{
				"SELECT target_id, latitude, longitude, probability, last_seen_at FROM targets;",
				"SELECT target_id, observer_id, latitude, longitude, confidence, timestamp FROM observations;",
			}
			before := make([]sqlReply, len(queries))
			for i, q := range queries {
				before[i] = mustSQL147(t, f.db, q)
				if len(before[i].Rows) != 1 {
					t.Fatalf("expected exactly one real domain row: %+v", before[i])
				}
			}
			var saved wireTarget
			for i, dest := range []any{&saved.TargetID, &saved.Latitude, &saved.Longitude, &saved.Probability, &saved.LastSeenAt} {
				if err := json.Unmarshal(before[0].Rows[0][i], dest); err != nil {
					t.Fatal(err)
				}
			}
			if saved != v {
				t.Fatalf("stored Target differs from published Target: %+v vs %+v", saved, v)
			}
			assertTypes147(t, before[0], []string{"TEXT", "REAL", "REAL", "REAL", "INTEGER"})
			assertTypes147(t, before[1], []string{"TEXT", "TEXT", "REAL", "REAL", "REAL", "INTEGER"})
			// Full int64 limits use an independent non-domain probe: future domain timestamps are invalid.
			mustSQL147(t, f.db, "CREATE TABLE bounds (n INTEGER, r REAL);")
			mustSQL147(t, f.db, "INSERT INTO bounds (n,r) VALUES (9223372036854775807, 0.125);")
			mustSQL147(t, f.db, "INSERT INTO bounds (n,r) VALUES (-9223372036854775808, -30.5);")
			mustSQL147(t, f.db, "UPDATE bounds SET r = 0.75 WHERE n = 9223372036854775807;")
			bounds := mustSQL147(t, f.db, "SELECT n,r FROM bounds;")
			if string(bounds.Rows[0][0]) != "9223372036854775807" || string(bounds.Rows[1][0]) != "-9223372036854775808" || string(bounds.Rows[0][1]) != "0.75" || string(bounds.Rows[1][1]) != "-30.5" {
				t.Fatal("int64 lost before restart")
			}
			snapshot, err := os.ReadFile(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, q := range []string{"UPDATE targets SET latitude = 1.0, probability = 'bad' WHERE target_id = '17';", "INSERT INTO bounds (n,r) VALUES (1, 'bad');", "SELECT * FROM targets; SELECT * FROM targets;", "DELETE FROM targets;"} {
				r, err := queryDBMS(f.db, q)
				if err != nil || r.OK || r.Error.Code != "SQL_ERROR" {
					t.Fatalf("invalid SQL %q: %+v %v", q, r, err)
				}
			}
			after, err := os.ReadFile(data)
			if err != nil || !bytes.Equal(snapshot, after) {
				t.Fatal("failed SQL changed snapshot")
			}
			if err := db.signal(signal); err != nil {
				t.Fatal(err)
			}
			err = db.wait(f.ctx)
			if signal == syscall.SIGTERM && err != nil {
				t.Fatal(err)
			}
			if signal == syscall.SIGKILL && err == nil {
				t.Fatal("kill did not terminate DBMS")
			}
			if signal == syscall.SIGTERM {
				if err := checkAbsent(f.db); err != nil {
					t.Fatal(err)
				}
			}
			// Restore and read before any new Observation or Target Service startup.
			restarted := launch()
			for i, q := range queries {
				if got := mustSQL147(t, f.db, q); !reflect.DeepEqual(before[i], got) {
					t.Fatalf("restored row mismatch: before=%+v after=%+v", before[i], got)
				}
			}
			if got := mustSQL147(t, f.db, "SELECT n,r FROM bounds;"); !reflect.DeepEqual(bounds, got) {
				t.Fatal("int64/REAL changed after restart")
			}
			if err := restarted.stop(); err != nil {
				t.Fatal(err)
			}
			if err := checkAbsent(f.db); err != nil {
				t.Fatal(err)
			}
			for name, broken := range map[string][]byte{"empty": {}, "truncated": snapshot[:len(snapshot)/2], "invalid": []byte("not JSON")} {
				brokenPath := filepath.Join(f.dir, name+".json")
				if err := os.WriteFile(brokenPath, broken, 0600); err != nil {
					t.Fatal(err)
				}
				p, err := f.s.start("corrupt snapshot "+name, b.dbms, "-socket", f.db, "-data", brokenPath)
				if err != nil {
					t.Fatal(err)
				}
				badCtx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
				err = p.wait(badCtx)
				cancel()
				if err == nil || !strings.Contains(p.logs.String(), "load snapshot") {
					t.Fatalf("corrupt startup: %v %s", err, p.logs.String())
				}
				if err := p.alive(); err == nil {
					t.Fatal("corrupt server still alive")
				}
				after, err := os.ReadFile(brokenPath)
				if err != nil || !bytes.Equal(broken, after) {
					t.Fatal("corrupt snapshot modified")
				}
				if err := checkAbsent(f.db); err != nil {
					t.Fatal(err)
				}
			}
			launch()
			if got := mustSQL147(t, f.db, queries[0]); !reflect.DeepEqual(before[0], got) {
				t.Fatal("valid snapshot damaged")
			}
			t.Log("PASS real domain rows restored; SQL errors, int64/REAL, corrupt files and cleanup checked")
		})
	}
}
func mustSQL147(t *testing.T, socket, sql string) sqlReply {
	t.Helper()
	r, err := queryDBMS(socket, sql)
	if err != nil || !r.OK {
		t.Fatalf("%s: %+v %v", sql, r, err)
	}
	return r
}
func assertTypes147(t *testing.T, r sqlReply, want []string) {
	t.Helper()
	if len(r.Columns) != len(want) {
		t.Fatal("wrong columns")
	}
	for i, raw := range r.Columns {
		var c struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &c); err != nil || c.Type != want[i] {
			t.Fatalf("column %d: %s want %s", i, raw, want[i])
		}
	}
}
