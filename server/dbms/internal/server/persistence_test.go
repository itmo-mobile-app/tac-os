package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/itmo-mobile-app/tac-os/server/dbms"
)

func startPersistentProcess(t *testing.T, binary, socket, data string) (net.Conn, func(os.Signal)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, binary, "-socket", socket, "-data", data)
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	wait := func() error { waited = true; return cmd.Wait() }
	t.Cleanup(func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = wait()
		}
	})
	ready := make(chan bool, 1)
	go func() { s := bufio.NewScanner(out); ready <- s.Scan() && strings.HasPrefix(s.Text(), "Listening on ") }()
	select {
	case ok := <-ready:
		if !ok {
			_ = cmd.Process.Kill()
			err = wait()
			t.Fatalf("startup: %v: %s", err, diagnostics.String())
		}
	case <-ctx.Done():
		t.Fatal("server startup timed out")
	}
	c := dial(t, socket)
	return c, func(sig os.Signal) {
		t.Helper()
		if err := cmd.Process.Signal(sig); err != nil {
			t.Fatal(err)
		}
		err := wait()
		if sig != os.Kill && err != nil {
			t.Fatalf("shutdown: %v: %s", err, diagnostics.String())
		}
	}
}

func TestProcessPersistence(t *testing.T) {
	binary := os.Getenv("TACOS_DBMS_SERVER")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "tacdb-server")
		cmd := exec.Command("go", "build", "-race", "-buildvcs=false", "-o", binary, "./cmd/tacdb-server")
		cmd.Dir = "../.."
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
	}
	dir, err := os.MkdirTemp("", "db131-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock, path := filepath.Join(dir, "db.sock"), filepath.Join(dir, "state.json")
	c, stop := startPersistentProcess(t, binary, sock, path)
	code(t, query(t, c, "SELECT * FROM t;"), "SQL_ERROR")
	if s, err := ListenPersistent(filepath.Join(dir, "second.sock"), time.Second, path); err == nil {
		s.Close()
		t.Fatal("second writer opened the same snapshot")
	}
	ok(t, c, "CREATE TABLE empty (id INTEGER);")
	ok(t, c, "CREATE TABLE t (id INTEGER, score REAL, label TEXT);")
	ok(t, c, "INSERT INTO t (id, score, label) VALUES (9223372036854775807, 0.5, 'О''кей');")
	ok(t, c, "UPDATE t SET score = 0.75 WHERE id = 9223372036854775807;")
	code(t, query(t, c, "UPDATE t SET label = 'bad', score = 1 WHERE id = 9223372036854775807;"), "SQL_ERROR")
	// Kill after the acknowledged write: saving only on shutdown would fail here.
	stop(os.Kill)
	c, stop = startPersistentProcess(t, binary, sock, path)
	r := query(t, c, "SELECT * FROM t;")
	row := r["rows"].([]any)[0].([]any)
	if row[0] != json.Number("9223372036854775807") || row[1] != json.Number("0.75") || row[2] != "О'кей" {
		t.Fatalf("restored data: %+v", r)
	}
	r = query(t, c, "SELECT * FROM empty;")
	if len(r["rows"].([]any)) != 0 || r["columns"].([]any)[0].(map[string]any)["type"] != "INTEGER" {
		t.Fatalf("empty table lost: %+v", r)
	}
	stop(syscall.SIGTERM)
	if _, err := os.Lstat(sock); !os.IsNotExist(err) {
		t.Fatal("socket left behind")
	}
	broken := []byte(`{"version":1,"tables":[`)
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-socket", sock, "-data", path)
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "load snapshot") {
		t.Fatalf("corrupt startup: %v %s", err, out)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, broken) {
		t.Fatal("corrupt snapshot was overwritten")
	}
	if _, err := os.Lstat(sock); !os.IsNotExist(err) {
		t.Fatal("socket created for corrupt database")
	}
}

func TestStorageErrorResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	db, err := dbms.OpenSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	e := executor{db: *db}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	request := func(sql string) map[string]any {
		t.Helper()
		var r map[string]any
		if err := json.Unmarshal(e.execute([]byte(sql)), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	code(t, request("CREATE TABLE t (id INTEGER);"), "INTERNAL_ERROR")
	code(t, request("SELECT * FROM t;"), "SQL_ERROR")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := request("CREATE TABLE t (id INTEGER);"); got["ok"] != true {
		t.Fatal(got)
	}
}
