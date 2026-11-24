package dbclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRealDBMSLostReply commits a real INSERT, drops only its response, and
// verifies that the existing client does not replay the ambiguous write.
func TestRealDBMSLostReply(t *testing.T) {
	binary := os.Getenv("TACOS_DBMS_SERVER")
	if binary == "" {
		t.Skip("set TACOS_DBMS_SERVER for real DBMS lost-response verification")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "db147-reply-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "db.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-socket", socket, "-data", filepath.Join(dir, "state.json"))
	logs, err := os.Create(filepath.Join(dir, "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	cmd.Stdout, cmd.Stderr = logs, logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	direct := New(socket, time.Second)
	defer direct.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("DBMS startup timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := direct.Exec(ctx, "CREATE TABLE notes (id INTEGER);"); err != nil {
		t.Fatal(err)
	}
	proxyPath := filepath.Join(dir, "proxy.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: proxyPath, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan error, 1)
	go func() {
		_ = listener.SetDeadline(time.Now().Add(3 * time.Second))
		peer, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer peer.Close()
		_ = peer.SetDeadline(time.Now().Add(3 * time.Second))
		sql, err := readSQL(peer)
		if err != nil {
			result <- err
			return
		}
		// Forward exactly once via the production client to the actual server.
		upstream := New(socket, time.Second)
		_, err = upstream.Exec(ctx, sql)
		_ = upstream.Close()
		if err != nil {
			result <- fmt.Errorf("upstream INSERT: %w", err)
			return
		}
		_ = peer.Close() // server acknowledged commit; caller never sees its reply
		_ = listener.SetDeadline(time.Now().Add(250 * time.Millisecond))
		extra, err := listener.Accept()
		if err == nil {
			_ = extra.Close()
			result <- fmt.Errorf("client reconnected after lost write reply")
			return
		}
		if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
			result <- err
			return
		}
		result <- nil
	}()
	client := New(proxyPath, time.Second)
	_, err = client.Exec(ctx, "INSERT INTO notes (id) VALUES (9223372036854775807);")
	_ = client.Close()
	if err == nil || !strings.Contains(err.Error(), "outcome unknown; not retried") {
		t.Fatalf("missing ambiguous-write error: %v", err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("proxy verification timed out")
	}
	r, err := direct.Exec(ctx, "SELECT id FROM notes;")
	if err != nil || len(r.Rows) != 1 || len(r.Rows[0]) != 1 || r.Rows[0][0] != json.Number("9223372036854775807") {
		t.Fatalf("write lost or duplicated: %+v %v", r, err)
	}
}
