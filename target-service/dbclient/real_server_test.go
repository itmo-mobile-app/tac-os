package dbclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestRealDBMSIntegration runs against the actual #96 server executable.
// Build it first and set TACOS_DBMS_SERVER to its absolute path.
func TestRealDBMSIntegration(t *testing.T) {
	binary := os.Getenv("TACOS_DBMS_SERVER")
	if binary == "" {
		t.Skip("set TACOS_DBMS_SERVER to run real DBMS integration")
	}
	abs, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "db.sock")
	cmd := exec.Command(abs, "-socket", socket, "-io-timeout", "2s")
	logFile, err := os.CreateTemp(t.TempDir(), "dbms-log-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	c := New(socket, 2*time.Second)
	defer c.Close()
	deadline := time.Now().Add(4 * time.Second)
	for {
		_, err := os.Stat(socket)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("DBMS did not start: %v; see %s", err, logFile.Name())
		}
		time.Sleep(20 * time.Millisecond)
	}
	ctx := context.Background()
	queries := []string{
		"CREATE TABLE client_test (id INTEGER, label TEXT, score REAL);",
		"INSERT INTO client_test (id, label, score) VALUES (9223372036854775807, " + SQLString("O'Reilly") + ", 0.5);",
		"UPDATE client_test SET score = 0.75 WHERE id = 9223372036854775807;",
	}
	for _, sql := range queries {
		if _, err := c.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	r, err := c.Exec(ctx, "SELECT id, label, score FROM client_test WHERE id = 9223372036854775807;")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Columns) != 3 || r.Columns[0].Type != "INTEGER" || r.Columns[1].Type != "TEXT" || r.Columns[2].Type != "REAL" || len(r.Rows) != 1 {
		t.Fatalf("unexpected result: %+v", r)
	}
	n, ok := r.Rows[0][0].(json.Number)
	if !ok || n.String() != "9223372036854775807" || r.Rows[0][1] != "O'Reilly" {
		t.Fatalf("data mismatch: %+v", r.Rows)
	}
	score, ok := r.Rows[0][2].(json.Number)
	if !ok || score.String() != "0.75" {
		t.Fatalf("UPDATE mismatch: %+v", r.Rows)
	}
	_, err = c.Exec(ctx, "SELECT id FROM missing_table;")
	var sqlErr *QueryError
	if !errors.As(err, &sqlErr) || sqlErr.Code != "SQL_ERROR" || sqlErr.Position == nil {
		t.Fatalf("SQL error mismatch: %v", err)
	}
	empty, err := c.Exec(ctx, "SELECT id FROM client_test WHERE id = 0;")
	if err != nil || len(empty.Columns) != 1 || len(empty.Rows) != 0 {
		t.Fatalf("empty SELECT: %+v, %v", empty, err)
	}
}
