package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func start(t *testing.T, timeout time.Duration) (string, *Server) {
	t.Helper()
	dir, err := os.MkdirTemp("", "db96-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "db.sock")
	s, err := Listen(path, timeout)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background()) }()
	t.Cleanup(func() {
		s.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("shutdown blocked")
		}
		_ = os.RemoveAll(dir)
	})
	return path, s
}
func dial(t *testing.T, path string) net.Conn {
	t.Helper()
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	return c
}
func ask(c net.Conn, sql string) (map[string]any, error) {
	if err := writeFrame(c, []byte(sql)); err != nil {
		return nil, err
	}
	p, err := readFrame(c)
	if err != nil {
		return nil, err
	}
	var r map[string]any
	d := json.NewDecoder(bytes.NewReader(p))
	d.UseNumber()
	err = d.Decode(&r)
	return r, err
}
func query(t *testing.T, c net.Conn, sql string) map[string]any {
	t.Helper()
	r, err := ask(c, sql)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func ok(t *testing.T, c net.Conn, sql string) {
	t.Helper()
	r := query(t, c, sql)
	if r["ok"] != true || len(r) != 1 {
		t.Fatalf("%s: %+v", sql, r)
	}
}
func code(t *testing.T, r map[string]any, want string) {
	t.Helper()
	if r["ok"] != false || r["error"].(map[string]any)["code"] != want {
		t.Fatalf("expected %s: %+v", want, r)
	}
}
func TestSQL(t *testing.T) {
	path, _ := start(t, 5*time.Second)
	c := dial(t, path)
	ok(t, c, "CREATE TABLE targets (id TEXT, score REAL, stamp INTEGER);")
	r := query(t, c, "SELECT stamp, id, stamp FROM targets;")
	if len(r["rows"].([]any)) != 0 || r["columns"].([]any)[0].(map[string]any)["type"] != "INTEGER" {
		t.Fatal(r)
	}
	ok(t, c, "INSERT INTO targets (id, score, stamp) VALUES ('Привет; O''Reilly', 0.8, 9223372036854775807);")
	ok(t, c, "UPDATE targets SET score = 0.9 WHERE stamp = 9223372036854775807;")
	r = query(t, c, "SELECT * FROM targets;")
	row := r["rows"].([]any)[0].([]any)
	if row[0] != "Привет; O'Reilly" || row[1] != json.Number("0.9") || row[2] != json.Number("9223372036854775807") {
		t.Fatal(r)
	}
	ok(t, c, "UPDATE targets SET stamp = -9223372036854775808 WHERE score = 0.9;")
	r = query(t, c, "SELECT stamp FROM targets;")
	if r["rows"].([]any)[0].([]any)[0] != json.Number("-9223372036854775808") {
		t.Fatal(r)
	}
	r = query(t, c, "SELECT * FROM missing;")
	code(t, r, "SQL_ERROR")
	if r["error"].(map[string]any)["position"] != json.Number("14") {
		t.Fatal(r)
	}
	for _, sql := range []string{" ", string([]byte{255})} {
		r = query(t, c, sql)
		code(t, r, "BAD_REQUEST")
		if _, exists := r["error"].(map[string]any)["position"]; exists {
			t.Fatal("non SQL error position")
		}
	}
	code(t, query(t, c, "UPDATE targets SET id = 'bad', score = 30 WHERE score = 0.9;"), "SQL_ERROR")
	r = query(t, c, "SELECT id FROM targets;")
	if r["rows"].([]any)[0].([]any)[0] != "Привет; O'Reilly" {
		t.Fatal("partial update")
	}
	code(t, query(t, c, "SELECT * FROM targets; SELECT * FROM targets;"), "SQL_ERROR")
}
func TestFrames(t *testing.T) {
	path, _ := start(t, 5*time.Second)
	c := dial(t, path)
	var frames bytes.Buffer
	_ = writeFrame(&frames, []byte("CREATE TABLE t (x INTEGER);"))
	_ = writeFrame(&frames, []byte("INSERT INTO t (x) VALUES (7);"))
	for _, b := range frames.Bytes() {
		if _, err := c.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		p, err := readFrame(c)
		if err != nil || string(p) != "{\"ok\":true}" {
			t.Fatalf("%q %v", p, err)
		}
	}
	for _, input := range [][]byte{{0, 0, 0, 0}, {1, 0, 16, 0}, {1, 2}, {10, 0, 0, 0, 'x'}} {
		conn := dial(t, path)
		_, _ = conn.Write(input)
		_ = conn.(*net.UnixConn).CloseWrite()
		if n, err := conn.Read(make([]byte, 1)); n != 0 || err == nil {
			t.Fatalf("bad frame got response: %v %v", n, err)
		}
	}
	payload := []byte(strings.Repeat(" ", MaxFrame-len("SELECT * FROM t;")) + "SELECT * FROM t;")
	if err := writeFrame(c, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := readFrame(c); err != nil {
		t.Fatal(err)
	}
	// Coalesced complete frames also retain their individual boundaries.
	frames.Reset()
	_ = writeFrame(&frames, []byte("SELECT * FROM t;"))
	_ = writeFrame(&frames, []byte("SELECT * FROM t;"))
	_, _ = c.Write(frames.Bytes())
	for i := 0; i < 2; i++ {
		if _, err := readFrame(c); err != nil {
			t.Fatal(err)
		}
	}
}
func TestLargeResult(t *testing.T) {
	path, _ := start(t, 5*time.Second)
	c := dial(t, path)
	ok(t, c, "CREATE TABLE big (x TEXT);")
	ok(t, c, "INSERT INTO big (x) VALUES ('"+strings.Repeat("a", 600000)+"');")
	code(t, query(t, c, "SELECT x, x FROM big;"), "RESULT_TOO_LARGE")
	if query(t, c, "SELECT * FROM big WHERE x = 'missing';")["ok"] != true {
		t.Fatal("no recovery")
	}
}
func TestConcurrent(t *testing.T) {
	path, _ := start(t, 5*time.Second)
	c := dial(t, path)
	ok(t, c, "CREATE TABLE t (id INTEGER);")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn, err := net.Dial("unix", path)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			r, err := ask(conn, fmt.Sprintf("INSERT INTO t (id) VALUES (%d);", i))
			if err != nil || r["ok"] != true {
				t.Errorf("%+v %v", r, err)
			}
		}(i)
	}
	wg.Wait()
	r := query(t, c, "SELECT * FROM t;")
	if len(r["rows"].([]any)) != 20 {
		t.Fatal(r)
	}
}
func TestLifecycle(t *testing.T) {
	path, s := start(t, 80*time.Millisecond)
	c := dial(t, path)
	_, _ = c.Write([]byte{10, 0, 0, 0, 'x'})
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("no timeout")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v %v", info, err)
	}
	if other, err := Listen(path, time.Second); err == nil {
		other.Close()
		t.Fatal("active socket replaced")
	}
	s.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("socket left behind")
	}
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	_ = stale.Close()
	next, err := Listen(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := Listen(path, time.Second); err == nil {
		other.Close()
		t.Fatal("file replaced")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "keep" {
		t.Fatal("file removed")
	}
	_ = os.Remove(path)
	if err := os.Symlink(path+".lock", path); err != nil {
		t.Fatal(err)
	}
	if other, err := Listen(path, time.Second); err == nil {
		other.Close()
		t.Fatal("symlink replaced")
	}
}

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return w.Buffer.Write(p)
}
func TestShortWrites(t *testing.T) {
	w := &shortWriter{}
	if err := writeFrame(w, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	p, err := readFrame(&w.Buffer)
	if err != nil || string(p) != "hello" {
		t.Fatal(string(p), err)
	}
}
func TestProcess(t *testing.T) {
	binaryPath := os.Getenv("TACOS_DBMS_SERVER")
	if binaryPath == "" {
		binaryPath = filepath.Join(t.TempDir(), "tacdb-server")
		cmd := exec.Command("go", "build", "-race", "-buildvcs=false", "-o", binaryPath, "./cmd/tacdb-server")
		cmd.Dir = "../.."
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
	}
	dir, err := os.MkdirTemp("", "db96-process-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "db.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "-socket", path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	ready := make(chan error, 1)
	go func() {
		var b [1]byte
		for {
			_, err := io.ReadFull(out, b[:])
			if err != nil {
				ready <- err
				return
			}
			if b[0] == '\n' {
				ready <- nil
				return
			}
		}
	}()
	select {
	case err = <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("startup timed out")
	}
	c := dial(t, path)
	ok(t, c, "CREATE TABLE t (id INTEGER, score REAL);")
	ok(t, c, "INSERT INTO t (id, score) VALUES (9223372036854775807, 0.8);")
	ok(t, c, "UPDATE t SET score = 0.9 WHERE id = 9223372036854775807;")
	r := query(t, c, "SELECT * FROM t;")
	if r["rows"].([]any)[0].([]any)[1] != json.Number("0.9") {
		t.Fatal(r)
	}
	if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatalf("exit: %v %s", err, diagnostics.String())
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("socket left behind")
	}
	if diagnostics.Len() != 0 {
		t.Fatal(diagnostics.String())
	}
}
