package dbclient

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fake(t *testing.T, serve func(net.Conn)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "db.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); serve(c) }()
		}
	}()
	return path
}
func readSQL(c net.Conn) (string, error) {
	var h [4]byte
	if _, err := io.ReadFull(c, h[:]); err != nil {
		return "", err
	}
	n := binary.LittleEndian.Uint32(h[:])
	if n == 0 || n > MaxFrame {
		return "", ErrFrameSize
	}
	b := make([]byte, n)
	_, err := io.ReadFull(c, b)
	return string(b), err
}
func send(c net.Conn, s string) error {
	var h [4]byte
	binary.LittleEndian.PutUint32(h[:], uint32(len(s)))
	if _, err := c.Write(h[:2]); err != nil {
		return err
	}
	if _, err := c.Write(h[2:]); err != nil {
		return err
	}
	for len(s) > 0 {
		n := 3
		if len(s) < n {
			n = len(s)
		}
		if _, err := io.WriteString(c, s[:n]); err != nil {
			return err
		}
		s = s[n:]
	}
	return nil
}
func TestCRUDAndInt64(t *testing.T) {
	var queries atomic.Int32
	socket := fake(t, func(c net.Conn) {
		for {
			sql, err := readSQL(c)
			if err != nil {
				return
			}
			queries.Add(1)
			var response string
			switch {
			case strings.HasPrefix(sql, "CREATE TABLE notes"):
				response = `{"ok":true}`
			case strings.HasPrefix(sql, "INSERT INTO notes"):
				if !strings.Contains(sql, "'O''Reilly'") {
					t.Errorf("missing SQL escaping: %s", sql)
				}
				response = `{"ok":true}`
			case strings.HasPrefix(sql, "UPDATE notes"):
				response = `{"ok":true}`
			case strings.HasPrefix(sql, "SELECT"):
				response = `{"ok":true,"columns":[{"name":"id","type":"INTEGER"},{"name":"body","type":"TEXT"}],"rows":[[9223372036854775807,"O'Reilly"]]}`
			default:
				response = `{"ok":false,"error":{"code":"SQL_ERROR","message":"invalid","position":0}}`
			}
			if send(c, response) != nil {
				return
			}
		}
	})
	c := New(socket, time.Second)
	defer c.Close()
	ctx := context.Background()
	for _, sql := range []string{"CREATE TABLE notes (id INTEGER, body TEXT);", fmt.Sprintf("INSERT INTO notes (id, body) VALUES (9223372036854775807, %s);", SQLString("O'Reilly")), "UPDATE notes SET body = 'O''Reilly' WHERE id = 9223372036854775807;"} {
		if _, err := c.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	r, err := c.Exec(ctx, "SELECT id, body FROM notes;")
	if err != nil {
		t.Fatal(err)
	}
	v, ok := r.Rows[0][0].(json.Number)
	if !ok || v.String() != "9223372036854775807" {
		t.Fatalf("int64 lost: %#v", r.Rows)
	}
	if queries.Load() != 4 {
		t.Fatalf("got %d requests", queries.Load())
	}
}
func TestSQLError(t *testing.T) {
	socket := fake(t, func(c net.Conn) {
		_, _ = readSQL(c)
		_ = send(c, `{"ok":false,"error":{"code":"SQL_ERROR","message":"Unknown table","position":14}}`)
	})
	c := New(socket, time.Second)
	defer c.Close()
	_, err := c.Exec(context.Background(), "SELECT * FROM missing;")
	var q *QueryError
	if !errors.As(err, &q) || q.Code != "SQL_ERROR" || *q.Position != 14 {
		t.Fatalf("got %v", err)
	}
}
func TestDisconnectNoReplay(t *testing.T) {
	var count atomic.Int32
	socket := fake(t, func(c net.Conn) {
		_, err := readSQL(c)
		if err != nil {
			return
		}
		count.Add(1) /* close without reply */
	})
	c := New(socket, time.Second)
	defer c.Close()
	_, err := c.Exec(context.Background(), "INSERT INTO notes (id, body) VALUES (1, 'x');")
	if err == nil {
		t.Fatal("expected EOF")
	}
	time.Sleep(30 * time.Millisecond)
	if count.Load() != 1 {
		t.Fatalf("unexpected replay %d", count.Load())
	}
}
func TestTimeout(t *testing.T) {
	socket := fake(t, func(c net.Conn) { _, _ = readSQL(c); time.Sleep(250 * time.Millisecond) })
	c := New(socket, 40*time.Millisecond)
	defer c.Close()
	start := time.Now()
	_, err := c.Exec(context.Background(), "SELECT * FROM notes;")
	if err == nil || time.Since(start) > 200*time.Millisecond {
		t.Fatalf("timeout failed: %v", err)
	}
}
func TestContextCancel(t *testing.T) {
	socket := fake(t, func(c net.Conn) { _, _ = readSQL(c); time.Sleep(time.Second) })
	c := New(socket, 3*time.Second)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Exec(ctx, "SELECT * FROM notes;")
	if err == nil || time.Since(start) > 300*time.Millisecond {
		t.Fatalf("cancel failed: %v", err)
	}
}
func TestInvalidFrame(t *testing.T) {
	for _, n := range []uint32{0, MaxFrame + 1} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			socket := fake(t, func(c net.Conn) {
				_, _ = readSQL(c)
				var h [4]byte
				binary.LittleEndian.PutUint32(h[:], n)
				_, _ = c.Write(h[:])
			})
			c := New(socket, time.Second)
			defer c.Close()
			_, err := c.Exec(context.Background(), "SELECT * FROM notes;")
			if !errors.Is(err, ErrFrameSize) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
func TestTruncatedBody(t *testing.T) {
	socket := fake(t, func(c net.Conn) {
		_, _ = readSQL(c)
		var h [4]byte
		binary.LittleEndian.PutUint32(h[:], 10)
		_, _ = c.Write(h[:])
		_, _ = c.Write([]byte("{}"))
	})
	c := New(socket, time.Second)
	defer c.Close()
	_, err := c.Exec(context.Background(), "SELECT * FROM notes;")
	if err == nil {
		t.Fatal("expected truncation error")
	}
}
func TestLargeRequestAndInvalidUTF8(t *testing.T) {
	c := New(filepath.Join(os.TempDir(), "nonexistent-dbms.sock"), time.Second)
	defer c.Close()
	for _, s := range []string{strings.Repeat("a", MaxFrame+1), string([]byte{0xff})} {
		if _, err := c.Exec(context.Background(), s); err == nil {
			t.Fatal("expected rejection")
		}
	}
}
func TestTypedMismatch(t *testing.T) {
	socket := fake(t, func(c net.Conn) {
		_, _ = readSQL(c)
		_ = send(c, `{"ok":true,"columns":[{"name":"x","type":"INTEGER"}],"rows":[[1.5]]}`)
	})
	c := New(socket, time.Second)
	defer c.Close()
	_, err := c.Exec(context.Background(), "SELECT x FROM notes;")
	if err == nil {
		t.Fatal("expected invalid INTEGER")
	}
}
