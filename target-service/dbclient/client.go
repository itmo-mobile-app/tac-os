// Package dbclient implements the version 1 Unix socket SQL framing protocol.
package dbclient

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const MaxFrame = 1 << 20
const DefaultSocket = "/tmp/tac-os-dbms.sock"
const DefaultTimeout = 5 * time.Second

var ErrFrameSize = errors.New("DBMS frame length must be between 1 and 1048576 bytes")
var ErrClosed = errors.New("DBMS client is closed")

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type QueryError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Position *int64 `json:"position,omitempty"`
}

func (e *QueryError) Error() string {
	if e.Position != nil {
		return fmt.Sprintf("DBMS %s at byte %d: %s", e.Code, *e.Position, e.Message)
	}
	return fmt.Sprintf("DBMS %s: %s", e.Code, e.Message)
}

type Result struct {
	OK      bool        `json:"ok"`
	Columns []Column    `json:"columns,omitempty"`
	Rows    [][]any     `json:"rows,omitempty"`
	Error   *QueryError `json:"error,omitempty"`
}

// Client serializes exchanges on a single persistent connection. A failed
// exchange invalidates the connection and is never replayed automatically.
type Client struct {
	mu      sync.Mutex
	conn    net.Conn
	socket  string
	timeout time.Duration
	closed  bool
}

func New(socket string, timeout time.Duration) *Client {
	if socket == "" {
		socket = DefaultSocket
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Client{socket: socket, timeout: timeout}
}
func (c *Client) Close() error { c.mu.Lock(); defer c.mu.Unlock(); c.closed = true; return c.drop() }
func (c *Client) drop() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) Exec(ctx context.Context, sql string) (Result, error) {
	var zero Result
	if !utf8.ValidString(sql) || strings.TrimSpace(sql) == "" {
		return zero, errors.New("SQL must be nonempty UTF-8")
	}
	if len(sql) > MaxFrame {
		return zero, ErrFrameSize
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return zero, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if c.conn == nil {
		dialer := net.Dialer{Timeout: c.timeout}
		conn, err := dialer.DialContext(ctx, "unix", c.socket)
		if err != nil {
			return zero, fmt.Errorf("connect DBMS: %w", err)
		}
		c.conn = conn
	}
	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		c.drop()
		return zero, err
	}
	// Close the socket on context cancellation, including contexts without deadlines.
	activeConn := c.conn
	stop := context.AfterFunc(ctx, func() { _ = activeConn.SetDeadline(time.Now()) })
	defer stop()
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(sql)))
	if err := writeAll(c.conn, header[:]); err != nil {
		c.drop()
		return zero, fmt.Errorf("write SQL header (outcome unknown; not retried): %w", err)
	}
	if err := writeAll(c.conn, []byte(sql)); err != nil {
		c.drop()
		return zero, fmt.Errorf("write SQL body (outcome unknown; not retried): %w", err)
	}
	if _, err := io.ReadFull(c.conn, header[:]); err != nil {
		c.drop()
		return zero, fmt.Errorf("read DBMS header (outcome unknown; not retried): %w", err)
	}
	n := binary.LittleEndian.Uint32(header[:])
	if n == 0 || n > MaxFrame {
		c.drop()
		return zero, ErrFrameSize
	}
	body := make([]byte, int(n))
	if _, err := io.ReadFull(c.conn, body); err != nil {
		c.drop()
		return zero, fmt.Errorf("read DBMS body (outcome unknown; not retried): %w", err)
	}
	if !utf8.Valid(body) {
		c.drop()
		return zero, errors.New("invalid UTF-8 response")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var r Result
	if err := dec.Decode(&r); err != nil {
		c.drop()
		return zero, fmt.Errorf("decode DBMS response: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		c.drop()
		return zero, errors.New("DBMS response contains trailing JSON")
	}
	if err := validate(&r); err != nil {
		c.drop()
		return zero, err
	}
	if !r.OK {
		return r, r.Error
	}
	return r, nil
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func validate(r *Result) error {
	if r.OK {
		if r.Error != nil {
			return errors.New("success response has error")
		}
		for _, col := range r.Columns {
			if col.Name == "" || (col.Type != "TEXT" && col.Type != "INTEGER" && col.Type != "REAL") {
				return errors.New("invalid column descriptor")
			}
		}
		for _, row := range r.Rows {
			if len(row) != len(r.Columns) {
				return errors.New("row width differs from columns")
			}
			for i, v := range row {
				switch r.Columns[i].Type {
				case "TEXT":
					if _, ok := v.(string); !ok {
						return errors.New("TEXT value is not string")
					}
				case "INTEGER":
					num, ok := v.(json.Number)
					if !ok {
						return errors.New("INTEGER is not JSON number")
					}
					if _, err := num.Int64(); err != nil {
						return fmt.Errorf("INTEGER outside int64: %w", err)
					}
				case "REAL":
					num, ok := v.(json.Number)
					if !ok {
						return errors.New("REAL is not JSON number")
					}
					if _, err := num.Float64(); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if r.Error == nil || r.Error.Code == "" || r.Error.Message == "" {
		return errors.New("error response missing code/message")
	}
	if r.Error.Code == "SQL_ERROR" {
		if r.Error.Position == nil || *r.Error.Position < 0 {
			return errors.New("SQL_ERROR missing valid position")
		}
	} else if r.Error.Position != nil {
		return errors.New("unexpected error position")
	}
	return nil
}

// SQLString escapes apostrophes using the SQL v1 TEXT literal convention.
func SQLString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
